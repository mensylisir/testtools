/*
Copyright 2023 The Kubernetes authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	"github.com/xiaoming/testtools/pkg/utils"
)

// PingReconciler reconciles a Ping object
type PingReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=pings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=pings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=pings/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *PingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	// 降级为调试日志
	logger.V(1).Info("Reconciling Ping", "namespace", req.Namespace, "name", req.Name)

	// 第1步：获取最新的 Ping 资源
	var ping testtoolsv1.Ping
	if err := r.Get(ctx, req.NamespacedName, &ping); err != nil {
		// 忽略未找到的错误，因为它们不能通过即时重新排队来修复
		// (我们需要等待新的通知)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 检查是否已经执行完成，避免重复执行
	// 检查是否已存在完成状态的条件
	for _, condition := range ping.Status.Conditions {
		if condition.Type == "Completed" && condition.Status == metav1.ConditionTrue {
			logger.Info("Ping测试已经完成，不再重复执行",
				"ping", ping.Name,
				"namespace", ping.Namespace)
			return ctrl.Result{}, nil
		}
	}

	// 添加严格的节流机制，如果上次执行时间太近，直接延迟处理
	// 检查是否有最后执行时间，避免频繁执行
	if ping.Status.LastExecutionTime != nil && ping.Spec.Schedule != "" {
		// 只有在明确设置了Schedule字段时才应用定期执行逻辑
		interval := 60 // 默认60秒
		if ping.Spec.Schedule != "" {
			if intervalValue, err := strconv.Atoi(ping.Spec.Schedule); err == nil && intervalValue > 0 {
				interval = intervalValue
			}
		}

		// 计算距离上次执行的时间
		elapsed := time.Since(ping.Status.LastExecutionTime.Time).Seconds()
		remainingTime := float64(interval) - elapsed

		// 如果距离上次执行的时间小于间隔的80%，则延迟处理
		if remainingTime > float64(interval)*0.2 {
			// 计算下一次执行的时间
			nextRunDelay := time.Duration(remainingTime) * time.Second
			logger.V(1).Info("Too soon to execute again, scheduling future reconcile",
				"ping", ping.Name,
				"namespace", ping.Namespace,
				"elapsed", elapsed,
				"interval", interval,
				"nextRunIn", nextRunDelay)
			return ctrl.Result{RequeueAfter: nextRunDelay}, nil
		}
	}

	// 确保 TestReportName 字段存在，这样 TestReportController 可以找到关联的 Ping 资源
	// 注意：TestReport 的创建和更新已移至 TestReportController
	if ping.Status.TestReportName == "" {
		pingCopy := ping.DeepCopy()
		pingCopy.Status.TestReportName = fmt.Sprintf("ping-%s-report", ping.Name)

		if err := r.Status().Update(ctx, pingCopy); err != nil {
			logger.Error(err, "Failed to update Ping status with TestReportName")
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}

		// 重新获取最新的 Ping 对象，以确保使用最新状态
		if err := r.Get(ctx, req.NamespacedName, &ping); err != nil {
			logger.Error(err, "Failed to refresh Ping resource after updating TestReportName")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	// 更新状态，记录开始时间
	pingCopy := ping.DeepCopy()
	now := metav1.NewTime(time.Now())
	pingCopy.Status.LastExecutionTime = &now
	pingCopy.Status.QueryCount++

	// 准备 Job 执行 Ping 测试
	jobName, err := utils.PreparePingJob(ctx, r.Client, &ping)
	if err != nil {
		logger.Error(err, "Failed to prepare Ping job")
		pingCopy.Status.Status = "Failed"
		pingCopy.Status.FailureCount++
		pingCopy.Status.LastResult = fmt.Sprintf("Error preparing Ping job: %v", err)

		// 添加失败条件
		utils.SetCondition(&pingCopy.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "JobPreparationFailed",
			Message:            fmt.Sprintf("Ping测试准备作业失败: %v", err),
		})

		// 更新状态
		if updateErr := r.Status().Update(ctx, pingCopy); updateErr != nil {
			logger.Error(updateErr, "Failed to update Ping status after job preparation failure")
		}

		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	// 等待 Job 完成
	logger.Info("Waiting for Ping job to complete", "jobName", jobName)
	err = utils.WaitForJob(ctx, r.Client, ping.Namespace, jobName, 5*time.Minute)
	if err != nil {
		logger.Error(err, "Failed while waiting for Ping job to complete")
		pingCopy.Status.Status = "Failed"
		pingCopy.Status.FailureCount++
		pingCopy.Status.LastResult = fmt.Sprintf("Error waiting for Ping job: %v", err)

		// 添加失败条件
		utils.SetCondition(&pingCopy.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "JobExecutionFailed",
			Message:            fmt.Sprintf("Ping测试执行失败: %v", err),
		})

		// 更新状态
		if updateErr := r.Status().Update(ctx, pingCopy); updateErr != nil {
			logger.Error(updateErr, "Failed to update Ping status after job execution failure")
		}

		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	// 获取 Job 执行结果
	jobOutput, err := utils.GetJobResults(ctx, r.Client, ping.Namespace, jobName)
	if err != nil {
		logger.Error(err, "Failed to get Ping job results")
		pingCopy.Status.Status = "Failed"
		pingCopy.Status.FailureCount++
		pingCopy.Status.LastResult = fmt.Sprintf("Error getting Ping job results: %v", err)

		// 添加失败条件
		utils.SetCondition(&pingCopy.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "ResultRetrievalFailed",
			Message:            fmt.Sprintf("无法获取Ping测试结果: %v", err),
		})
	} else {
		// 解析 Ping 输出并更新状态
		pingOutput := utils.ParsePingOutput(jobOutput, ping.Spec.Host)

		// 提取统计数据
		pingStats := utils.ExtractPingStats(pingOutput)

		// 更新状态信息
		pingCopy.Status.Status = "Succeeded"
		pingCopy.Status.SuccessCount++
		pingCopy.Status.LastResult = jobOutput

		// 更新Ping统计数据
		pingCopy.Status.PacketLoss = pingStats.PacketLoss
		pingCopy.Status.MinRtt = pingStats.MinRtt
		pingCopy.Status.AvgRtt = pingStats.AvgRtt
		pingCopy.Status.MaxRtt = pingStats.MaxRtt

		// 标记测试已完成
		utils.SetCondition(&pingCopy.Status.Conditions, metav1.Condition{
			Type:               "Completed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "TestCompleted",
			Message:            "Ping测试已成功完成",
		})
	}

	// 更新状态
	if updateErr := r.Status().Update(ctx, pingCopy); updateErr != nil {
		logger.Error(updateErr, "Failed to update Ping status")
		return ctrl.Result{RequeueAfter: time.Second * 5}, updateErr
	}

	// 只有在明确设置了Schedule字段时才安排下一次执行
	if ping.Spec.Schedule != "" {
		interval := 60 // 默认60秒
		if ping.Spec.Schedule != "" {
			if intervalValue, err := strconv.Atoi(ping.Spec.Schedule); err == nil && intervalValue > 0 {
				interval = intervalValue
			}
		}
		logger.Info("Scheduling next Ping test", "interval", interval)
		return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, nil
	}

	// 默认情况下，不再调度下一次执行
	logger.Info("Ping测试已完成，不再调度下一次执行",
		"ping", ping.Name,
		"namespace", ping.Namespace)
	return ctrl.Result{}, nil
}

// executePing 执行 Ping 测试并返回结果
func (r *PingReconciler) executePing(ctx context.Context, ping *testtoolsv1.Ping) (bool, string, testtoolsv1.PingStatus, error) {
	logger := log.FromContext(ctx)
	start := time.Now()
	stats := testtoolsv1.PingStatus{}

	// 验证参数
	if ping.Spec.Host == "" {
		logger.Error(fmt.Errorf("host is required"), "Invalid Ping specification",
			"ping", ping.Name,
			"namespace", ping.Namespace)
		return false, "Host is required", stats, fmt.Errorf("host is required")
	}

	// 构建 ping 命令
	args := r.buildPingArgs(ping)
	logger.V(1).Info("Executing ping command",
		"args", args,
		"host", ping.Spec.Host,
		"count", ping.Spec.Count)

	// 创建命令
	cmd := exec.Command("ping", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// 执行命令
	cmdStr := fmt.Sprintf("ping %s", strings.Join(args, " "))
	logger.V(1).Info("Starting ping execution", "command", cmdStr)

	err := cmd.Run()
	executionTime := time.Since(start)
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	// 处理执行结果
	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	if err != nil {
		logger.Error(err, "Failed to execute ping command",
			"stderr", stderrStr,
			"stdout", stdoutStr,
			"exitCode", exitCode,
			"duration", executionTime.String())

		result := stderrStr
		if result == "" {
			result = stdoutStr
		}
		return false, result, stats, fmt.Errorf("ping command failed: %w (exit code: %d)", err, exitCode)
	}

	// 成功执行
	output := stdoutStr

	// 解析输出
	pingOutput := parsePingOutput(output, ping.Spec.Host)
	stats = extractPingStats(pingOutput)

	logger.Info("Ping command executed successfully",
		"duration", executionTime.String(),
		"packetLoss", pingOutput.PacketLoss,
		"avgRtt", pingOutput.AvgRtt)

	return true, output, stats, nil
}

// parsePingOutput 解析 ping 命令的输出，提取结构化数据
func parsePingOutput(output string, host string) *PingOutput {
	pingOutput := &PingOutput{
		Host:            host,
		SuccessfulPings: []float64{},
	}

	// 提取统计信息

	// 提取发送和接收的数据包数
	packetsRegex := regexp.MustCompile(`(\d+) packets transmitted, (\d+) received`)
	packetsMatches := packetsRegex.FindStringSubmatch(output)
	if len(packetsMatches) > 2 {
		if transmitted, err := strconv.Atoi(packetsMatches[1]); err == nil {
			pingOutput.Transmitted = transmitted
		}
		if received, err := strconv.Atoi(packetsMatches[2]); err == nil {
			pingOutput.Received = received
		}
	}

	// 提取丢包率
	packetLossRegex := regexp.MustCompile(`(\d+)% packet loss`)
	packetLossMatches := packetLossRegex.FindStringSubmatch(output)
	if len(packetLossMatches) > 1 {
		if packetLoss, err := strconv.ParseFloat(packetLossMatches[1], 64); err == nil {
			pingOutput.PacketLoss = packetLoss
		}
	}

	// 提取 RTT 统计信息
	// 不同平台的输出格式可能不同
	// Linux: min/avg/max/mdev
	// Windows: Minimum/Maximum/Average
	// macOS: min/avg/max/stddev

	// 尝试 Linux/macOS 格式
	rttRegex := regexp.MustCompile(`min/avg/max(?:/(?:mdev|stddev))? = ([\d.]+)/([\d.]+)/([\d.]+)(?:/([\d.]+))?`)
	rttMatches := rttRegex.FindStringSubmatch(output)
	if len(rttMatches) > 3 {
		if minRtt, err := strconv.ParseFloat(rttMatches[1], 64); err == nil {
			pingOutput.MinRtt = minRtt
		}
		if avgRtt, err := strconv.ParseFloat(rttMatches[2], 64); err == nil {
			pingOutput.AvgRtt = avgRtt
		}
		if maxRtt, err := strconv.ParseFloat(rttMatches[3], 64); err == nil {
			pingOutput.MaxRtt = maxRtt
		}
		if len(rttMatches) > 4 && rttMatches[4] != "" {
			if stddev, err := strconv.ParseFloat(rttMatches[4], 64); err == nil {
				pingOutput.StdDevRtt = stddev
			}
		}
	} else {
		// 尝试 Windows 格式
		minRegex := regexp.MustCompile(`Minimum = ([\d.]+)ms`)
		maxRegex := regexp.MustCompile(`Maximum = ([\d.]+)ms`)
		avgRegex := regexp.MustCompile(`Average = ([\d.]+)ms`)

		minMatches := minRegex.FindStringSubmatch(output)
		if len(minMatches) > 1 {
			if minRtt, err := strconv.ParseFloat(minMatches[1], 64); err == nil {
				pingOutput.MinRtt = minRtt
			}
		}

		maxMatches := maxRegex.FindStringSubmatch(output)
		if len(maxMatches) > 1 {
			if maxRtt, err := strconv.ParseFloat(maxMatches[1], 64); err == nil {
				pingOutput.MaxRtt = maxRtt
			}
		}

		avgMatches := avgRegex.FindStringSubmatch(output)
		if len(avgMatches) > 1 {
			if avgRtt, err := strconv.ParseFloat(avgMatches[1], 64); err == nil {
				pingOutput.AvgRtt = avgRtt
			}
		}
	}

	// 提取每个成功 ping 的 RTT 值 (可选）
	timeRegex := regexp.MustCompile(`time=([\d.]+) ms`)
	timeMatches := timeRegex.FindAllStringSubmatch(output, -1)
	for _, match := range timeMatches {
		if len(match) > 1 {
			if time, err := strconv.ParseFloat(match[1], 64); err == nil {
				pingOutput.SuccessfulPings = append(pingOutput.SuccessfulPings, time)
			}
		}
	}

	return pingOutput
}

// extractPingStats 从 PingOutput 提取 Ping 统计信息到 PingStatus
func extractPingStats(output *PingOutput) testtoolsv1.PingStatus {
	stats := testtoolsv1.PingStatus{}

	// 复制统计数据到 PingStatus
	stats.PacketLoss = output.PacketLoss
	stats.MinRtt = output.MinRtt
	stats.AvgRtt = output.AvgRtt
	stats.MaxRtt = output.MaxRtt

	return stats
}

// buildPingArgs builds the arguments for the ping command
func (r *PingReconciler) buildPingArgs(ping *testtoolsv1.Ping) []string {
	var args []string

	// Add the count parameter
	if ping.Spec.Count > 0 {
		args = append(args, "-c", strconv.Itoa(int(ping.Spec.Count)))
	}

	// Add the interval parameter (in seconds)
	if ping.Spec.Interval > 0 {
		args = append(args, "-i", strconv.Itoa(int(ping.Spec.Interval)))
	}

	// Add the timeout parameter (in seconds)
	if ping.Spec.Timeout > 0 {
		args = append(args, "-W", strconv.Itoa(int(ping.Spec.Timeout)))
	}

	// Add the packet size parameter
	if ping.Spec.PacketSize > 0 {
		args = append(args, "-s", strconv.Itoa(int(ping.Spec.PacketSize)))
	}

	// Add the TTL parameter
	if ping.Spec.TTL > 0 {
		args = append(args, "-t", strconv.Itoa(int(ping.Spec.TTL)))
	}

	// Add the IPv4 only parameter
	if ping.Spec.UseIPv4Only {
		args = append(args, "-4")
	}

	// Add the IPv6 only parameter
	if ping.Spec.UseIPv6Only {
		args = append(args, "-6")
	}

	// Add the do not fragment parameter
	if ping.Spec.DoNotFragment {
		args = append(args, "-M", "do")
	}

	// Finally, add the host
	args = append(args, ping.Spec.Host)

	return args
}

// parsePingStatistics parses the ping output to extract statistics
func parsePingStatistics(output string, status *testtoolsv1.PingStatus) {
	// Parse packet loss
	packetLossRegex := regexp.MustCompile(`(\d+)% packet loss`)
	packetLossMatches := packetLossRegex.FindStringSubmatch(output)
	if len(packetLossMatches) > 1 {
		if packetLoss, err := strconv.ParseFloat(packetLossMatches[1], 64); err == nil {
			status.PacketLoss = packetLoss
		}
	}

	// Parse RTT statistics
	rttRegex := regexp.MustCompile(`min/avg/max(?:/(?:mdev|stddev))? = ([\d.]+)/([\d.]+)/([\d.]+)`)
	rttMatches := rttRegex.FindStringSubmatch(output)
	if len(rttMatches) > 3 {
		if minRtt, err := strconv.ParseFloat(rttMatches[1], 64); err == nil {
			status.MinRtt = minRtt
		}
		if avgRtt, err := strconv.ParseFloat(rttMatches[2], 64); err == nil {
			status.AvgRtt = avgRtt
		}
		if maxRtt, err := strconv.ParseFloat(rttMatches[3], 64); err == nil {
			status.MaxRtt = maxRtt
		}
	}
}

// getExitCode extracts the exit code from an exec.ExitError
func getExitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}

// statusEqual checks if two PingStatus are equal
func pingStatusEqual(a, b testtoolsv1.PingStatus) bool {
	return a.Status == b.Status &&
		a.QueryCount == b.QueryCount &&
		a.SuccessCount == b.SuccessCount &&
		a.FailureCount == b.FailureCount &&
		a.PacketLoss == b.PacketLoss &&
		a.MinRtt == b.MinRtt &&
		a.AvgRtt == b.AvgRtt &&
		a.MaxRtt == b.MaxRtt &&
		a.TestReportName == b.TestReportName
}

// SetupWithManager sets up the controller with the Manager.
func (r *PingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testtoolsv1.Ping{}).
		Complete(r)
}

// PingOutput 表示 ping 命令输出的结构化表示
type PingOutput struct {
	Host            string    `json:"host"`            // 目标主机
	Transmitted     int       `json:"transmitted"`     // 已发送的数据包数
	Received        int       `json:"received"`        // 已接收的数据包数
	PacketLoss      float64   `json:"packetLoss"`      // 数据包丢失率 (%)
	MinRtt          float64   `json:"minRtt"`          // 最小往返时间 (ms)
	AvgRtt          float64   `json:"avgRtt"`          // 平均往返时间 (ms)
	MaxRtt          float64   `json:"maxRtt"`          // 最大往返时间 (ms)
	StdDevRtt       float64   `json:"stdDevRtt"`       // 往返时间标准差 (ms)
	SuccessfulPings []float64 `json:"successfulPings"` // 所有成功的ping往返时间
}

// conditionExists 检查条件是否存在并且内容相同
func conditionExists(conditions *[]metav1.Condition, newCond metav1.Condition) bool {
	for _, cond := range *conditions {
		if cond.Type == newCond.Type &&
			cond.Status == newCond.Status &&
			cond.Reason == newCond.Reason &&
			cond.Message == newCond.Message {
			return true
		}
	}
	return false
}
