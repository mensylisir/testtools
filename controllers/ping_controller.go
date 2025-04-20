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
	"context"
	"fmt"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/types"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	"github.com/xiaoming/testtools/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
)

// PingFinalizerName 是用于Ping资源finalizer的名称
const PingFinalizerName = "ping.testtools.xiaoming.com/finalizer"

// PingReconciler reconciles a Ping object
type PingReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=pings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=pings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=pings/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *PingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("开始Ping调谐", "namespacedName", req.NamespacedName)

	// 记录执行时间
	start := time.Now()
	defer func() {
		logger.Info("完成Ping调谐", "namespacedName", req.NamespacedName, "duration", time.Since(start))
	}()

	// 获取Ping实例
	var ping testtoolsv1.Ping
	if err := r.Get(ctx, req.NamespacedName, &ping); err != nil {
		if apierrors.IsNotFound(err) {
			// Ping资源已被删除，忽略
			logger.Info("Ping资源已被删除，忽略", "namespacedName", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "获取Ping资源失败", "namespacedName", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// 处理资源删除
	if !ping.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&ping, PingFinalizerName) {
			logger.Info("处理Ping资源删除", "pingName", ping.Name)
			if err := r.cleanupResources(ctx, &ping); err != nil {
				logger.Error(err, "清理资源失败")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&ping, PingFinalizerName)
			if err := r.Update(ctx, &ping); err != nil {
				logger.Error(err, "移除finalizer失败")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// 确保finalizer存在
	if !controllerutil.ContainsFinalizer(&ping, PingFinalizerName) {
		controllerutil.AddFinalizer(&ping, PingFinalizerName)
		if err := r.Update(ctx, &ping); err != nil {
			logger.Error(err, "添加finalizer失败")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// 检查是否需要执行测试
	shouldExecute := false

	// 检查是否设置了调度规则
	if ping.Spec.Schedule != "" {
		// 如果有上次执行时间，检查是否到了下次执行时间
		if ping.Status.LastExecutionTime != nil {
			// 计算下次执行时间
			intervalSeconds := 60 // 默认60秒
			if scheduleValue, err := strconv.Atoi(ping.Spec.Schedule); err == nil && scheduleValue > 0 {
				intervalSeconds = scheduleValue
			}

			nextExecutionTime := ping.Status.LastExecutionTime.Time.Add(time.Duration(intervalSeconds) * time.Second)
			shouldExecute = time.Now().After(nextExecutionTime)

			if !shouldExecute {
				logger.Info("还未到调度时间，跳过执行",
					"ping", ping.Name,
					"上次执行", ping.Status.LastExecutionTime.Time,
					"下次执行", nextExecutionTime,
					"当前时间", time.Now())
				// 计算下次执行的延迟时间
				delay := time.Until(nextExecutionTime)
				if delay < 0 {
					delay = time.Second * 5 // 如果计算出的延迟为负，设置一个短暂的重试时间
				}
				return ctrl.Result{RequeueAfter: delay}, nil
			}
		} else {
			// 没有上次执行时间，应该立即执行
			shouldExecute = true
		}
	} else {
		// 没有设置调度规则，只有在初始创建或者状态为空时执行一次
		shouldExecute = ping.Status.LastExecutionTime == nil
	}

	if !shouldExecute {
		logger.Info("无需执行Ping测试",
			"ping", ping.Name,
			"lastExecutionTime", ping.Status.LastExecutionTime,
			"schedule", ping.Spec.Schedule)
		return ctrl.Result{}, nil
	}

	// 更新状态，记录开始时间
	pingCopy := ping.DeepCopy()
	now := metav1.NewTime(time.Now())
	pingCopy.Status.LastExecutionTime = &now
	// 在这里设置执行标记，但仅在首次执行时增加QueryCount
	if ping.Status.QueryCount == 0 {
		// 如果是首次执行，设置查询计数为1
		pingCopy.Status.QueryCount = 1
	}

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

	// 构建并保存执行的命令
	pingArgs := utils.BuildPingArgs(&ping)
	pingCopy.Status.ExecutedCommand = fmt.Sprintf("ping %s", strings.Join(pingArgs, " "))
	logger.Info("Setting executed command", "command", pingCopy.Status.ExecutedCommand)

	var job batchv1.Job
	if err = r.Client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: jobName}, &job); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Ping job not found, creating...", "ping", req.Name, "jobName", jobName, "namespace", req.Namespace)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Job", "ping", req.Name, "jobName", jobName)
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	} else {
		if job.Status.Succeeded > 0 {
			logger.Info("Job completed successfully", "ping", req.Name, "jobName", jobName, "namespace", req.Namespace)

			// 获取 Job 执行结果
			jobOutput, err := utils.GetJobResults(ctx, r.Client, ping.Namespace, jobName)
			if err != nil {
				logger.Error(err, "Failed to get job results")
				pingCopy.Status.Status = "Failed"
				pingCopy.Status.FailureCount++
				pingCopy.Status.LastResult = fmt.Sprintf("Error getting job results: %v", err)

				// 设置TestReportName，即使失败也设置
				if pingCopy.Status.TestReportName == "" {
					pingCopy.Status.TestReportName = utils.GenerateTestReportName("Ping", ping.Name)
				}

				if pingCopy.Status.QueryCount == 0 {
					pingCopy.Status.QueryCount = 1
				}

				// 添加失败条件
				utils.SetCondition(&pingCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "ResultRetrievalFailed",
					Message:            fmt.Sprintf("Ping测试获取结果失败: %v", err),
				})
			} else {
				// 成功执行
				pingCopy.Status.Status = "Succeeded"
				pingCopy.Status.SuccessCount++
				pingCopy.Status.LastResult = jobOutput

				if pingCopy.Status.QueryCount == 0 {
					pingCopy.Status.QueryCount = 1
				}

				// 使用utils.ParsePingOutput解析输出并设置统计信息
				pingOutput := utils.ParsePingOutput(jobOutput, ping.Spec.Host)

				// 设置状态中的性能指标
				pingCopy.Status.PacketLoss = fmt.Sprintf("%f", pingOutput.PacketLoss)
				pingCopy.Status.MinRtt = fmt.Sprintf("%f", pingOutput.MinRtt)
				pingCopy.Status.AvgRtt = fmt.Sprintf("%f", pingOutput.AvgRtt)
				pingCopy.Status.MaxRtt = fmt.Sprintf("%f", pingOutput.MaxRtt)

				// 添加成功条件
				utils.SetCondition(&pingCopy.Status.Conditions, metav1.Condition{
					Type:               "Completed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "TestCompleted",
					Message:            "Ping测试已成功完成",
				})

				// 设置TestReportName，以便TestReport控制器可以找到它
				if pingCopy.Status.TestReportName == "" {
					pingCopy.Status.TestReportName = utils.GenerateTestReportName("Ping", ping.Name)
				}
			}

			if updateErr := r.Status().Update(ctx, pingCopy); updateErr != nil {
				logger.Info(updateErr.Error(), "failed to update ping status", "ping", ping.Name, "namespace", ping.Namespace)
				return ctrl.Result{RequeueAfter: time.Second * 30}, nil
			}

		} else if job.Status.Failed > *job.Spec.BackoffLimit {
			logger.Error(nil, "Job failed", "ping", req.Name, "jobName", jobName, "namespace", req.Namespace,
				"failed", job.Status.Failed, "backoffLimit", *job.Spec.BackoffLimit)

			// 获取 Job 执行结果
			jobOutput, err := utils.GetJobResults(ctx, r.Client, ping.Namespace, jobName)
			if err != nil {
				logger.Error(err, "Failed to get job results")
				pingCopy.Status.Status = "Failed"
				pingCopy.Status.FailureCount++
				pingCopy.Status.LastResult = fmt.Sprintf("Error getting job results: %v", err)

				// 设置TestReportName，即使失败也设置
				if pingCopy.Status.TestReportName == "" {
					pingCopy.Status.TestReportName = utils.GenerateTestReportName("Ping", ping.Name)
				}

				if pingCopy.Status.QueryCount == 0 {
					pingCopy.Status.QueryCount = 1
				}

				// 添加失败条件
				utils.SetCondition(&pingCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "ResultRetrievalFailed",
					Message:            fmt.Sprintf("Ping测试获取结果失败: %v", err),
				})
			} else {
				// 成功执行
				pingCopy.Status.Status = "Failed"
				pingCopy.Status.FailureCount++
				pingCopy.Status.LastResult = jobOutput

				if pingCopy.Status.QueryCount == 0 {
					pingCopy.Status.QueryCount = 1
				}

				// 使用utils.ParsePingOutput解析输出并设置统计信息
				pingOutput := utils.ParsePingOutput(jobOutput, ping.Spec.Host)

				// 设置状态中的性能指标
				pingCopy.Status.PacketLoss = fmt.Sprintf("%f", pingOutput.PacketLoss)
				pingCopy.Status.MinRtt = fmt.Sprintf("%f", pingOutput.MinRtt)
				pingCopy.Status.AvgRtt = fmt.Sprintf("%f", pingOutput.AvgRtt)
				pingCopy.Status.MaxRtt = fmt.Sprintf("%f", pingOutput.MaxRtt)

				// 添加成功条件
				utils.SetCondition(&pingCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "TestCompleted",
					Message:            "Ping test complete failed",
				})

				// 设置TestReportName，以便TestReport控制器可以找到它
				if pingCopy.Status.TestReportName == "" {
					pingCopy.Status.TestReportName = utils.GenerateTestReportName("Ping", ping.Name)
				}
			}

			if updateErr := r.Status().Update(ctx, pingCopy); updateErr != nil {
				logger.Info(updateErr.Error(), "failed to update ping status", "ping", ping.Name, "namespace", ping.Namespace)
				return ctrl.Result{RequeueAfter: time.Second * 30}, nil
			}

		} else {
			logger.Info("Job is still running", "ping", req.Name, "jobName", jobName, "namespace", req.Namespace,
				"succeeded", job.Status.Succeeded, "failed", job.Status.Failed, "backoffLimit", *job.Spec.BackoffLimit)
			return ctrl.Result{}, nil
		}
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

//// executePing执行Ping测试并返回结果
//func (r *PingReconciler) executePing(ctx context.Context, ping *testtoolsv1.Ping) (bool, string, testtoolsv1.PingStatus, error) {
//	logger := log.FromContext(ctx)
//	start := time.Now()
//	stats := testtoolsv1.PingStatus{}
//
//	// 验证参数
//	if ping.Spec.Host == "" {
//		logger.Error(fmt.Errorf("host is required"), "Invalid Ping specification",
//			"ping", ping.Name,
//			"namespace", ping.Namespace)
//		return false, "Host is required", stats, fmt.Errorf("host is required")
//	}
//
//	// 构建 ping 命令
//	args := r.buildPingArgs(ping)
//	logger.V(1).Info("Executing ping command",
//		"args", args,
//		"host", ping.Spec.Host,
//		"count", ping.Spec.Count)
//
//	// 创建命令
//	cmd := exec.Command("ping", args...)
//	var stdout, stderr bytes.Buffer
//	cmd.Stdout = &stdout
//	cmd.Stderr = &stderr
//
//	// 执行命令
//	cmdStr := fmt.Sprintf("ping %s", strings.Join(args, " "))
//	logger.V(1).Info("Starting ping execution", "command", cmdStr)
//
//	err := cmd.Run()
//	executionTime := time.Since(start)
//	exitCode := -1
//	if cmd.ProcessState != nil {
//		exitCode = cmd.ProcessState.ExitCode()
//	}
//
//	// 处理执行结果
//	stdoutStr := stdout.String()
//	stderrStr := stderr.String()
//
//	if err != nil {
//		logger.Error(err, "Failed to execute ping command",
//			"stderr", stderrStr,
//			"stdout", stdoutStr,
//			"exitCode", exitCode,
//			"duration", executionTime.String())
//
//		result := stderrStr
//		if result == "" {
//			result = stdoutStr
//		}
//		return false, result, stats, fmt.Errorf("ping command failed: %w (exit code: %d)", err, exitCode)
//	}
//
//	// 成功执行
//	output := stdoutStr
//
//	// 使用utils中的ParsePingOutput，而不是本地的parsePingOutput
//	pingOutput := utils.ParsePingOutput(output, ping.Spec.Host)
//	stats = utils.ExtractPingStats(pingOutput)
//
//	logger.Info("Ping command executed successfully",
//		"duration", executionTime.String(),
//		"packetLoss", pingOutput.PacketLoss,
//		"avgRtt", pingOutput.AvgRtt)
//
//	return true, output, stats, nil
//}

// buildPingArgs builds the arguments for the ping command
//func (r *PingReconciler) buildPingArgs(ping *testtoolsv1.Ping) []string {
//	var args []string
//
//	// Add the count parameter
//	if ping.Spec.Count > 0 {
//		args = append(args, "-c", strconv.Itoa(int(ping.Spec.Count)))
//	}
//
//	// Add the interval parameter (in seconds)
//	if ping.Spec.Interval > 0 {
//		args = append(args, "-i", strconv.Itoa(int(ping.Spec.Interval)))
//	}
//
//	// Add the timeout parameter (in seconds)
//	if ping.Spec.Timeout > 0 {
//		args = append(args, "-W", strconv.Itoa(int(ping.Spec.Timeout)))
//	}
//
//	// Add the packet size parameter
//	if ping.Spec.PacketSize > 0 {
//		args = append(args, "-s", strconv.Itoa(int(ping.Spec.PacketSize)))
//	}
//
//	// Add the TTL parameter
//	if ping.Spec.TTL > 0 {
//		args = append(args, "-t", strconv.Itoa(int(ping.Spec.TTL)))
//	}
//
//	// Add the IPv4 only parameter
//	if ping.Spec.UseIPv4Only {
//		args = append(args, "-4")
//	}
//
//	// Add the IPv6 only parameter
//	if ping.Spec.UseIPv6Only {
//		args = append(args, "-6")
//	}
//
//	// Add the do not fragment parameter
//	if ping.Spec.DoNotFragment {
//		args = append(args, "-M", "do")
//	}
//
//	// Finally, add the host
//	args = append(args, ping.Spec.Host)
//
//	return args
//}

// getExitCode extracts the exit code from an exec.ExitError
//func getExitCode(err error) int {
//	if err == nil {
//		return 0
//	}
//	if exitErr, ok := err.(*exec.ExitError); ok {
//		return exitErr.ExitCode()
//	}
//	return -1
//}

// statusEqual checks if two PingStatus are equal
//func pingStatusEqual(a, b testtoolsv1.PingStatus) bool {
//	return a.Status == b.Status &&
//		a.QueryCount == b.QueryCount &&
//		a.SuccessCount == b.SuccessCount &&
//		a.FailureCount == b.FailureCount &&
//		a.PacketLoss == b.PacketLoss &&
//		a.MinRtt == b.MinRtt &&
//		a.AvgRtt == b.AvgRtt &&
//		a.MaxRtt == b.MaxRtt &&
//		a.TestReportName == b.TestReportName
//}

// SetupWithManager sets up the controller with the Manager.
func (r *PingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testtoolsv1.Ping{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

// PingOutput 表示 ping 命令输出的结构化表示
//type PingOutput struct {
//	Host            string    `json:"host"`            // 目标主机
//	Transmitted     int       `json:"transmitted"`     // 已发送的数据包数
//	Received        int       `json:"received"`        // 已接收的数据包数
//	PacketLoss      float64   `json:"packetLoss"`      // 数据包丢失率 (%)
//	MinRtt          float64   `json:"minRtt"`          // 最小往返时间 (ms)
//	AvgRtt          float64   `json:"avgRtt"`          // 平均往返时间 (ms)
//	MaxRtt          float64   `json:"maxRtt"`          // 最大往返时间 (ms)
//	StdDevRtt       float64   `json:"stdDevRtt"`       // 往返时间标准差 (ms)
//	SuccessfulPings []float64 `json:"successfulPings"` // 所有成功的ping往返时间
//}

// conditionExists 检查条件是否存在并且内容相同
//func conditionExists(conditions *[]metav1.Condition, newCond metav1.Condition) bool {
//	for _, cond := range *conditions {
//		if cond.Type == newCond.Type &&
//			cond.Status == newCond.Status &&
//			cond.Reason == newCond.Reason &&
//			cond.Message == newCond.Message {
//			return true
//		}
//	}
//	return false
//}

// cleanupResources 清理与Ping资源关联的所有资源
func (r *PingReconciler) cleanupResources(ctx context.Context, ping *testtoolsv1.Ping) error {
	logger := log.FromContext(ctx)
	logger.Info("清理资源", "pingName", ping.Name, "说明", "使用OwnerReference自动级联删除资源")
	// 不需要手动删除资源，因为所有关联资源都通过OwnerReference设置了级联删除
	return nil
}
