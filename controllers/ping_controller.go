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
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
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

	// 保存当前资源版本，用于后续更新时的乐观并发控制
	resourceVersion := ping.ResourceVersion

	// 第2步：确保测试报告名称存在
	reportName := fmt.Sprintf("ping-%s-report", ping.Name)

	// 更新TestReportName，但不需要立即重新排队
	if ping.Status.TestReportName == "" {
		pingCopy := ping.DeepCopy()
		pingCopy.Status.TestReportName = reportName

		// 使用 resourceVersion 确保我们更新的是我们刚刚获取的版本
		pingCopy.ResourceVersion = resourceVersion

		if err := r.Status().Update(ctx, pingCopy); err != nil {
			logger.Error(err, "Failed to update Ping status with TestReportName")
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}

		// 更新成功后，获取最新的资源版本并继续执行，而不是重新排队
		logger.Info("Updated Ping with TestReportName, continuing with execution",
			"ping", ping.Name,
			"namespace", ping.Namespace,
			"reportName", reportName)

		// 重新获取最新的Ping对象，以确保我们使用的是最新状态
		if err := r.Get(ctx, req.NamespacedName, &ping); err != nil {
			logger.Error(err, "Failed to refresh Ping resource after updating TestReportName")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	} else {
		reportName = ping.Status.TestReportName
	}

	// 添加严格的节流机制，如果上次执行时间太近，直接延迟处理
	// 检查是否有最后执行时间，避免频繁执行
	if ping.Status.LastExecutionTime != nil {
		// 假设Schedule表示的是执行间隔的秒数
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

	// 第3步：确保 TestReport 资源存在
	var testReport testtoolsv1.TestReport
	err := r.Get(ctx, client.ObjectKey{Namespace: ping.Namespace, Name: reportName}, &testReport)
	if apierrors.IsNotFound(err) {
		// 如果报告不存在，创建它
		testReport = testtoolsv1.TestReport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      reportName,
				Namespace: ping.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: ping.APIVersion,
						Kind:       ping.Kind,
						Name:       ping.Name,
						UID:        ping.UID,
						Controller: func() *bool { b := true; return &b }(),
					},
				},
			},
			Spec: testtoolsv1.TestReportSpec{
				TestName:     fmt.Sprintf("%s Ping Test", ping.Name),
				TestType:     "Network",
				Target:       fmt.Sprintf("%s 网络可达性测试", ping.Spec.Host),
				TestDuration: 1800, // 默认30分钟
				Interval:     60,   // 默认每60秒收集一次结果
				ResourceSelectors: []testtoolsv1.ResourceSelector{
					{
						APIVersion: "testtools.xiaoming.com/v1",
						Kind:       "Ping",
						Name:       ping.Name,
						Namespace:  ping.Namespace,
					},
				},
			},
		}

		if err := r.Create(ctx, &testReport); err != nil {
			logger.Error(err, "Failed to create TestReport")
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil // 5秒后重试
		}
		logger.Info("Created new TestReport",
			"ping", ping.Name,
			"namespace", ping.Namespace,
			"reportName", reportName)
	} else if err != nil {
		// 其他错误，记录并返回
		logger.Error(err, "Failed to get TestReport")
		return ctrl.Result{}, err
	}

	// 第4步：执行 ping 命令（如果需要）
	// 确定是否需要运行 ping 命令
	shouldRun := true

	// 检查是否有最后执行时间，并根据调度计划确定是否需要运行
	if ping.Status.LastExecutionTime != nil && ping.Spec.Schedule != "" {
		// 假设Schedule表示的是执行间隔的秒数
		interval := 60 // 默认60秒
		if ping.Spec.Schedule != "" {
			if intervalValue, err := strconv.Atoi(ping.Spec.Schedule); err == nil && intervalValue > 0 {
				interval = intervalValue
			}
		}

		// 检查最后执行时间是否小于间隔时间
		elapsed := time.Since(ping.Status.LastExecutionTime.Time)
		if elapsed.Seconds() < float64(interval) {
			shouldRun = false
			// 降级为调试日志
			logger.V(1).Info("Skipping execution due to schedule",
				"ping", ping.Name,
				"namespace", ping.Namespace,
				"elapsed", elapsed.Seconds(),
				"interval", interval)
		}
	}

	// 第5步：如果需要，执行 ping 命令并更新状态
	if shouldRun {
		// 执行 ping 命令 - 降级为调试日志
		logger.V(1).Info("Executing ping command",
			"ping", ping.Name,
			"namespace", ping.Namespace,
			"host", ping.Spec.Host)

		execResult, execErr := r.executePing(ctx, &ping, logger)

		// 获取最新的资源版本，以避免更新冲突
		if err := r.Get(ctx, req.NamespacedName, &ping); err != nil {
			logger.Error(err, "Failed to refresh Ping resource before status update")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		// 创建 ping 的深度拷贝
		pingCopy := ping.DeepCopy()
		oldStatus := *pingCopy.Status.DeepCopy()

		// 更新基本状态字段
		now := metav1.NewTime(time.Now())
		pingCopy.Status.LastExecutionTime = &now
		pingCopy.Status.QueryCount++

		// 根据执行结果更新成功/失败信息
		if execErr != nil {
			pingCopy.Status.Status = "Failed"
			pingCopy.Status.FailureCount++
			pingCopy.Status.LastResult = fmt.Sprintf("Error: %v", execErr)

			// 更新条件
			setCondition(&pingCopy.Status.Conditions, metav1.Condition{
				Type:               "Available",
				Status:             metav1.ConditionFalse,
				Reason:             "ExecutionFailed",
				Message:            execErr.Error(),
				LastTransitionTime: now,
			})
		} else {
			pingCopy.Status.Status = "Succeeded"
			pingCopy.Status.SuccessCount++
			pingCopy.Status.LastResult = execResult

			// 解析 ping 统计数据
			parsePingStatistics(execResult, &pingCopy.Status)

			// 更新条件
			setCondition(&pingCopy.Status.Conditions, metav1.Condition{
				Type:               "Available",
				Status:             metav1.ConditionTrue,
				Reason:             "ExecutionSucceeded",
				Message:            "Ping command executed successfully",
				LastTransitionTime: now,
			})
		}

		// 检查状态是否有实质性变化
		statusChanged := !pingStatusEqual(oldStatus, pingCopy.Status)

		// 如果状态有变化，则更新
		if statusChanged {
			// 只有在状态确实发生变化时才记录INFO级日志
			logger.Info("Updating Ping status with significant changes",
				"ping", ping.Name,
				"namespace", ping.Namespace)

			// 更新状态
			if err := r.Status().Update(ctx, pingCopy); err != nil {
				logger.Error(err, "Failed to update Ping status")
				// 不要立即重试，让控制器在下一个协调周期自动重试
				return ctrl.Result{RequeueAfter: time.Second * 5}, nil
			}

			// 第6步：更新 TestReport 的测试结果
			// 只有在 ping 状态有变化且执行成功时才更新 TestReport
			if execErr == nil {
				// 重新获取最新的 TestReport
				var testReport testtoolsv1.TestReport
				if err := r.Get(ctx, client.ObjectKey{Namespace: ping.Namespace, Name: reportName}, &testReport); err != nil {
					logger.Error(err, "Failed to get TestReport for updating result")
					// 不阻止控制循环继续
				} else {
					// 创建新的测试结果
					testResult := testtoolsv1.TestResult{
						ResourceName:      ping.Name,
						ResourceNamespace: ping.Namespace,
						ResourceKind:      ping.Kind,
						ExecutionTime:     now,
						Success:           true,
						ResponseTime:      pingCopy.Status.AvgRtt,
						Output:            execResult,
						RawOutput:         execResult,
						MetricValues: map[string]string{
							"packetLoss": fmt.Sprintf("%.2f", pingCopy.Status.PacketLoss),
							"minRtt":     fmt.Sprintf("%.2f", pingCopy.Status.MinRtt),
							"avgRtt":     fmt.Sprintf("%.2f", pingCopy.Status.AvgRtt),
							"maxRtt":     fmt.Sprintf("%.2f", pingCopy.Status.MaxRtt),
						},
					}

					// 更新 TestReport
					testReportCopy := testReport.DeepCopy()

					// 添加新的测试结果
					if testReportCopy.Status.Results == nil {
						testReportCopy.Status.Results = []testtoolsv1.TestResult{}
					}

					// 检查是否已包含类似的结果
					duplicateFound := false
					if len(testReportCopy.Status.Results) > 0 {
						lastResult := testReportCopy.Status.Results[len(testReportCopy.Status.Results)-1]
						if lastResult.ResourceName == ping.Name &&
							lastResult.ResourceNamespace == ping.Namespace &&
							lastResult.ExecutionTime.Time.Sub(now.Time).Abs() < 30*time.Second {
							duplicateFound = true
						}
					}

					if !duplicateFound {
						testReportCopy.Status.Results = append(testReportCopy.Status.Results, testResult)

						// 限制保留的结果数量，保留最新的10个
						if len(testReportCopy.Status.Results) > 10 {
							testReportCopy.Status.Results = testReportCopy.Status.Results[len(testReportCopy.Status.Results)-10:]
						}

						// 更新摘要
						updateTestReportSummary(&testReportCopy.Status)

						// 更新 TestReport
						if err := r.Status().Update(ctx, testReportCopy); err != nil {
							logger.Error(err, "Failed to update TestReport status")
							// 不阻止控制循环继续
						} else {
							logger.Info("Updated TestReport with new result",
								"ping", ping.Name,
								"namespace", ping.Namespace,
								"reportName", reportName)
						}
					} else {
						logger.Info("Skipping TestReport update as similar result already exists",
							"ping", ping.Name,
							"namespace", ping.Namespace,
							"reportName", reportName)
					}
				}
			}
		} else {
			logger.Info("No significant status changes, skipping updates",
				"ping", ping.Name,
				"namespace", ping.Namespace)
		}
	}

	// 第7步：安排下一次执行
	interval := 60 // 默认60秒
	if ping.Spec.Schedule != "" {
		if intervalValue, err := strconv.Atoi(ping.Spec.Schedule); err == nil && intervalValue > 0 {
			interval = intervalValue
		}
	}

	return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, nil
}

// reconcileTestReport 确保 TestReport 资源存在，并返回是否需要重新排队
// 采用纯声明式方法，不进行多次更新操作
func (r *PingReconciler) reconcileTestReport(ctx context.Context, ping *testtoolsv1.Ping, logger logr.Logger) (string, bool, error) {
	reportName := fmt.Sprintf("ping-%s-report", ping.Name)

	// 如果 Ping 资源尚未设置 TestReportName，则更新它
	if ping.Status.TestReportName == "" {
		pingCopy := ping.DeepCopy()
		pingCopy.Status.TestReportName = reportName
		if err := r.Status().Update(ctx, pingCopy); err != nil {
			logger.Error(err, "Failed to update Ping status with TestReportName")
			return "", false, err
		}
		// 状态已更新，需要重新进入 reconcile 循环
		return "", true, nil
	}

	// 检查 TestReport 是否存在
	var existingReport testtoolsv1.TestReport
	err := r.Get(ctx, client.ObjectKey{Namespace: ping.Namespace, Name: reportName}, &existingReport)
	if err == nil {
		// TestReport 已存在，无需操作
		return reportName, false, nil
	}

	// 如果是其他错误，返回错误
	if err := client.IgnoreNotFound(err); err != nil {
		logger.Error(err, "Failed to get TestReport")
		return "", false, err
	}

	// TestReport 不存在，创建它
	logger.Info("Creating new TestReport for Ping", "ping", ping.Name, "report", reportName)
	newReport := testtoolsv1.TestReport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reportName,
			Namespace: ping.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: ping.APIVersion,
					Kind:       ping.Kind,
					Name:       ping.Name,
					UID:        ping.UID,
					Controller: func() *bool { b := true; return &b }(),
				},
			},
		},
		Spec: testtoolsv1.TestReportSpec{
			TestName:     fmt.Sprintf("%s Ping Test", ping.Name),
			TestType:     "Network",
			Target:       fmt.Sprintf("%s 网络可达性测试", ping.Spec.Host),
			TestDuration: 1800, // 默认30分钟
			Interval:     60,   // 默认每60秒收集一次结果
			ResourceSelectors: []testtoolsv1.ResourceSelector{
				{
					APIVersion: "testtools.xiaoming.com/v1",
					Kind:       "Ping",
					Name:       ping.Name,
					Namespace:  ping.Namespace,
				},
			},
		},
	}

	// 创建 TestReport 资源
	if err := r.Create(ctx, &newReport); err != nil {
		logger.Error(err, "Failed to create TestReport")
		return "", false, err
	}

	return reportName, false, nil
}

// updatePingStatus 更新 Ping 资源的状态
func (r *PingReconciler) updatePingStatus(ctx context.Context, ping *testtoolsv1.Ping, result string, execErr error, logger logr.Logger) error {
	// 添加重试逻辑，最多重试3次
	var lastErr error
	for retries := 0; retries < 3; retries++ {
		// 添加退避延迟
		if retries > 0 {
			backoff := time.Duration(100*(1<<uint(retries))) * time.Millisecond // 指数退避: 100ms, 200ms, 400ms
			logger.Info("Retrying status update with backoff",
				"ping", ping.Name,
				"namespace", ping.Namespace,
				"retries", retries,
				"backoff", backoff)
			select {
			case <-time.After(backoff):
				// 继续重试
			case <-ctx.Done():
				// 上下文已取消，停止重试
				return fmt.Errorf("context canceled while waiting to retry: %v", ctx.Err())
			}
		}

		// 创建 Ping 的深度拷贝，以避免直接修改缓存中的对象
		pingCopy := ping.DeepCopy()

		// 准备状态更新
		now := metav1.NewTime(time.Now())
		pingCopy.Status.LastExecutionTime = &now
		pingCopy.Status.QueryCount++

		// 仅在状态有变化时才进行更新
		statusChanged := false

		// Update success/failure info
		if execErr != nil {
			statusChanged = pingCopy.Status.Status != "Failed" ||
				pingCopy.Status.LastResult != fmt.Sprintf("Error: %v", execErr)

			pingCopy.Status.Status = "Failed"
			pingCopy.Status.FailureCount++
			pingCopy.Status.LastResult = fmt.Sprintf("Error: %v", execErr)

			// Update conditions
			cond := metav1.Condition{
				Type:               "Available",
				Status:             metav1.ConditionFalse,
				Reason:             "ExecutionFailed",
				Message:            execErr.Error(),
				LastTransitionTime: now,
			}
			statusChanged = statusChanged || !conditionExists(&pingCopy.Status.Conditions, cond)
			setCondition(&pingCopy.Status.Conditions, cond)
		} else {
			// 解析ping统计数据，检查是否有变化
			oldStatus := pingCopy.Status.DeepCopy()
			parsePingStatistics(result, &pingCopy.Status)

			statusChanged = pingCopy.Status.Status != "Succeeded" ||
				pingCopy.Status.LastResult != result ||
				pingCopy.Status.PacketLoss != oldStatus.PacketLoss ||
				pingCopy.Status.MinRtt != oldStatus.MinRtt ||
				pingCopy.Status.AvgRtt != oldStatus.AvgRtt ||
				pingCopy.Status.MaxRtt != oldStatus.MaxRtt

			pingCopy.Status.Status = "Succeeded"
			pingCopy.Status.SuccessCount++
			pingCopy.Status.LastResult = result

			// Update conditions
			cond := metav1.Condition{
				Type:               "Available",
				Status:             metav1.ConditionTrue,
				Reason:             "ExecutionSucceeded",
				Message:            "Ping command executed successfully",
				LastTransitionTime: now,
			}
			statusChanged = statusChanged || !conditionExists(&pingCopy.Status.Conditions, cond)
			setCondition(&pingCopy.Status.Conditions, cond)
		}

		// 如果状态没有实质性变化，则跳过更新
		if !statusChanged && retries == 0 {
			logger.Info("Skipping status update as no significant changes detected",
				"ping", ping.Name,
				"namespace", ping.Namespace)
			return nil
		}

		// 尝试更新状态
		err := r.Status().Update(ctx, pingCopy)
		if err == nil {
			// 更新成功
			return nil
		}

		// 记录错误并准备重试
		lastErr = err
		logger.Error(err, "Failed to update Ping status, will retry",
			"ping", ping.Name,
			"namespace", ping.Namespace,
			"retries", retries)

		// 处理不同类型的错误
		if apierrors.IsConflict(err) {
			// 发生冲突，下一次循环将使用最新版本重试
			// 重新获取最新的 Ping 资源
			if getErr := r.Get(ctx, client.ObjectKey{Namespace: ping.Namespace, Name: ping.Name}, ping); getErr != nil {
				logger.Error(getErr, "Failed to get fresh Ping resource after conflict",
					"ping", ping.Name,
					"namespace", ping.Namespace)
				// 如果无法获取最新资源，我们将在下一次循环使用当前资源重试
			}
			continue
		}

		// 处理速率限制错误
		if apierrors.IsTooManyRequests(err) || strings.Contains(err.Error(), "rate limit") {
			logger.Info("Rate limiting detected, backing off before retry",
				"ping", ping.Name,
				"namespace", ping.Namespace,
				"retries", retries)
			continue
		}

		// 处理服务器超时或不可用
		if apierrors.IsServerTimeout(err) || apierrors.IsTimeout(err) || apierrors.IsServiceUnavailable(err) {
			logger.Info("Server timeout or unavailable, retrying",
				"ping", ping.Name,
				"namespace", ping.Namespace,
				"retries", retries)
			continue
		}

		// 其他类型的错误，直接返回
		return err
	}

	// 所有重试都失败了
	return fmt.Errorf("failed to update Ping status after retries: %w", lastErr)
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

// updateTestReport 更新 TestReport 资源，添加新的测试结果
func (r *PingReconciler) updateTestReport(ctx context.Context, ping *testtoolsv1.Ping, result string, execErr error, logger logr.Logger) error {
	reportName := ping.Status.TestReportName
	if reportName == "" {
		return nil // 无需更新
	}

	// 添加重试逻辑，最多重试3次
	var lastErr error
	var testReport *testtoolsv1.TestReport
	var testReportCopy *testtoolsv1.TestReport

	for retries := 0; retries < 3; retries++ {
		// 添加退避延迟
		if retries > 0 {
			backoff := time.Duration(100*(1<<uint(retries))) * time.Millisecond // 指数退避: 100ms, 200ms, 400ms
			// 降级为调试日志
			logger.V(1).Info("Retrying TestReport update with backoff",
				"ping", ping.Name,
				"namespace", ping.Namespace,
				"reportName", reportName,
				"retries", retries,
				"backoff", backoff)
			select {
			case <-time.After(backoff):
				// 继续重试
			case <-ctx.Done():
				// 上下文已取消，停止重试
				return fmt.Errorf("context canceled while waiting to retry TestReport update: %v", ctx.Err())
			}
		}

		// 获取最新的 TestReport 资源
		if testReport == nil || retries > 0 {
			testReport = &testtoolsv1.TestReport{}
			if err := r.Get(ctx, client.ObjectKey{Namespace: ping.Namespace, Name: reportName}, testReport); err != nil {
				lastErr = err
				logger.Error(err, "Failed to get TestReport for update",
					"ping", ping.Name,
					"namespace", ping.Namespace,
					"reportName", reportName,
					"retries", retries)

				// 如果不是暂时性错误，直接返回
				if !apierrors.IsNotFound(err) && !apierrors.IsServerTimeout(err) && !apierrors.IsTimeout(err) {
					return err
				}
				continue // 重试
			}

			// 获取成功，创建一个深度拷贝用于更新
			testReportCopy = testReport.DeepCopy()
		}

		// 检查是否已经包含相同的测试结果
		// 如果最后一个结果与当前要添加的结果相似（相同的资源和时间戳接近），则跳过更新
		skipUpdate := false
		if len(testReportCopy.Status.Results) > 0 {
			lastResult := testReportCopy.Status.Results[len(testReportCopy.Status.Results)-1]
			if lastResult.ResourceName == ping.Name &&
				lastResult.ResourceNamespace == ping.Namespace &&
				lastResult.ResourceKind == ping.Kind {
				// 如果最后的执行时间在30秒内，且成功/失败状态相同，则认为是重复的测试结果
				if ping.Status.LastExecutionTime != nil && lastResult.ExecutionTime.Time.Sub(ping.Status.LastExecutionTime.Time) < 30*time.Second {
					if (execErr == nil && lastResult.Success) || (execErr != nil && !lastResult.Success) {
						// 降级为调试日志
						logger.V(1).Info("Skipping TestReport update as similar result already exists",
							"ping", ping.Name,
							"namespace", ping.Namespace,
							"reportName", reportName)
						skipUpdate = true
					}
				}
			}
		}

		if skipUpdate && retries == 0 {
			return nil
		}

		// 创建一个新的测试结果
		testResult := testtoolsv1.TestResult{
			ResourceName:      ping.Name,
			ResourceNamespace: ping.Namespace,
			ResourceKind:      ping.Kind,
			Success:           execErr == nil,
		}

		// 确保LastExecutionTime不为空
		if ping.Status.LastExecutionTime != nil {
			testResult.ExecutionTime = *ping.Status.LastExecutionTime
		} else {
			now := metav1.NewTime(time.Now())
			testResult.ExecutionTime = now
		}

		if execErr != nil {
			testResult.Error = execErr.Error()
		} else {
			testResult.ResponseTime = ping.Status.AvgRtt
			testResult.Output = result
			testResult.RawOutput = result
			testResult.MetricValues = map[string]string{
				"packetLoss": fmt.Sprintf("%.2f", ping.Status.PacketLoss),
				"minRtt":     fmt.Sprintf("%.2f", ping.Status.MinRtt),
				"avgRtt":     fmt.Sprintf("%.2f", ping.Status.AvgRtt),
				"maxRtt":     fmt.Sprintf("%.2f", ping.Status.MaxRtt),
			}
		}

		// 添加新的测试结果
		if testReportCopy.Status.Results == nil {
			testReportCopy.Status.Results = []testtoolsv1.TestResult{}
		}
		testReportCopy.Status.Results = append(testReportCopy.Status.Results, testResult)

		// 限制保留的结果数量，保留最新的10个
		if len(testReportCopy.Status.Results) > 10 {
			testReportCopy.Status.Results = testReportCopy.Status.Results[len(testReportCopy.Status.Results)-10:]
		}

		// 更新摘要
		updateTestReportSummary(&testReportCopy.Status)

		// 更新 TestReport 状态
		err := r.Status().Update(ctx, testReportCopy)
		if err == nil {
			// 更新成功
			return nil
		}

		// 记录错误并准备重试
		lastErr = err
		logger.Error(err, "Failed to update TestReport status, will retry",
			"ping", ping.Name,
			"namespace", ping.Namespace,
			"reportName", reportName,
			"retries", retries)

		// 特定类型的错误处理
		if apierrors.IsConflict(err) || apierrors.IsTooManyRequests(err) ||
			strings.Contains(err.Error(), "rate limit") ||
			apierrors.IsServerTimeout(err) || apierrors.IsTimeout(err) ||
			apierrors.IsServiceUnavailable(err) {
			// 这些错误类型可以重试
			// 设置为nil强制在下一次重试时重新获取
			testReport = nil
			testReportCopy = nil
			continue
		}

		// 其他类型的错误，直接返回
		return err
	}

	// 所有重试都失败
	return fmt.Errorf("failed to update TestReport after retries: %w", lastErr)
}

// updateTestReportSummary 更新 TestReport 的摘要信息
func updateTestReportSummary(status *testtoolsv1.TestReportStatus) {
	status.Summary.Total = len(status.Results)

	succeeded := 0
	failed := 0
	var totalResponseTime float64
	minResponseTime := math.MaxFloat64
	maxResponseTime := 0.0

	for _, result := range status.Results {
		if result.Success {
			succeeded++
			totalResponseTime += result.ResponseTime

			if result.ResponseTime < minResponseTime {
				minResponseTime = result.ResponseTime
			}
			if result.ResponseTime > maxResponseTime {
				maxResponseTime = result.ResponseTime
			}
		} else {
			failed++
		}
	}

	status.Summary.Succeeded = succeeded
	status.Summary.Failed = failed

	if succeeded > 0 {
		status.Summary.AverageResponseTime = totalResponseTime / float64(succeeded)
		status.Summary.MinResponseTime = minResponseTime
		status.Summary.MaxResponseTime = maxResponseTime
	}
}

// executePing executes the ping command with the given parameters
func (r *PingReconciler) executePing(ctx context.Context, ping *testtoolsv1.Ping, logger logr.Logger) (string, error) {
	start := time.Now()

	// Validate ping command parameters
	if ping.Spec.Host == "" {
		logger.Error(fmt.Errorf("host is required"), "Invalid Ping specification",
			"ping", ping.Name,
			"namespace", ping.Namespace)
		return "", fmt.Errorf("host is required")
	}

	// Construct the ping command
	args := r.buildPingArgs(ping)
	// 调试日志
	logger.V(1).Info("Executing ping command",
		"args", args,
		"host", ping.Spec.Host,
		"count", ping.Spec.Count,
		"interval", ping.Spec.Interval,
		"timeout", ping.Spec.Timeout,
		"packetSize", ping.Spec.PacketSize)

	// Create the command
	cmd := exec.Command("ping", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute the command
	// 调试日志
	logger.V(1).Info("Starting ping execution",
		"command", fmt.Sprintf("ping %s", strings.Join(args, " ")))
	err := cmd.Run()
	execDuration := time.Since(start)
	// 调试日志
	logger.V(1).Info("Ping command execution completed",
		"duration", execDuration.String(),
		"exitCode", getExitCode(err))

	// Get the command output
	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	// Check for errors
	if err != nil {
		logger.Error(err, "Failed to execute ping command",
			"stderr", stderrStr,
			"stdout", stdoutStr,
			"duration", execDuration.String(),
			"args", args)
		return stdoutStr, err
	}

	// 成功执行记录为INFO日志，但简化输出信息
	logger.Info("Ping command executed successfully",
		"duration", execDuration.String(),
		"outputLength", len(stdoutStr))

	return stdoutStr, nil
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
