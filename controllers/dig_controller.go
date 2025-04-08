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

	"github.com/google/uuid"
	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
)

// DigReconciler reconciles a Dig object
type DigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=digs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=digs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=digs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	reconcileID := fmt.Sprintf("%s", uuid.New().String())
	ctx = context.WithValue(ctx, "reconcileID", reconcileID)

	logger.Info("Reconciling Dig", "namespace", req.Namespace, "name", req.Name, "reconcileID", reconcileID)

	// 第1步：获取最新的 Dig 资源
	var dig testtoolsv1.Dig
	if err := r.Get(ctx, req.NamespacedName, &dig); err != nil {
		// 忽略未找到错误，因为它们不能立即重新排队来修复
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 保存当前资源版本，用于后续更新时的乐观并发控制
	resourceVersion := dig.ResourceVersion

	// 第2步：确保测试报告名称存在
	reportName := fmt.Sprintf("dig-%s-report", dig.Name)

	// 更新TestReportName，但不需要立即重新排队
	if dig.Status.TestReportName == "" {
		digCopy := dig.DeepCopy()
		digCopy.Status.TestReportName = reportName

		// 使用 resourceVersion 确保我们更新的是我们刚刚获取的版本
		digCopy.ResourceVersion = resourceVersion

		if err := r.Status().Update(ctx, digCopy); err != nil {
			logger.Error(err, "Failed to update Dig status with TestReportName",
				"dig", dig.Name,
				"namespace", dig.Namespace,
				"reportName", reportName)
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}

		// 更新成功后，获取最新的资源版本并继续执行，不需要重新排队
		logger.Info("Updated Dig with TestReportName, continuing with execution",
			"dig", dig.Name,
			"namespace", dig.Namespace,
			"reportName", reportName)

		// 重新获取最新的Dig对象，以确保我们使用的是最新状态
		if err := r.Get(ctx, req.NamespacedName, &dig); err != nil {
			logger.Error(err, "Failed to refresh Dig resource after updating TestReportName")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	} else {
		reportName = dig.Status.TestReportName
	}

	// 添加严格的节流机制，如果上次执行时间太近，直接延迟处理
	// 检查是否有最后执行时间，避免频繁执行
	if dig.Status.LastExecutionTime != nil {
		// 假设Schedule表示的是执行间隔的秒数
		interval := 60 // 默认60秒
		if dig.Spec.Schedule != "" {
			if intervalValue, err := strconv.Atoi(dig.Spec.Schedule); err == nil && intervalValue > 0 {
				interval = intervalValue
			}
		}

		// 计算距离上次执行的时间
		elapsed := time.Since(dig.Status.LastExecutionTime.Time).Seconds()
		remainingTime := float64(interval) - elapsed

		// 如果距离上次执行的时间小于间隔的80%，则延迟处理
		if remainingTime > float64(interval)*0.2 {
			// 计算下一次执行的时间
			nextRunDelay := time.Duration(remainingTime) * time.Second
			logger.V(1).Info("Too soon to execute again, scheduling future reconcile",
				"dig", dig.Name,
				"namespace", dig.Namespace,
				"elapsed", elapsed,
				"interval", interval,
				"nextRunIn", nextRunDelay)
			return ctrl.Result{RequeueAfter: nextRunDelay}, nil
		}
	}

	// 第3步：确保 TestReport 资源存在
	var testReport testtoolsv1.TestReport
	err := r.Get(ctx, client.ObjectKey{Namespace: dig.Namespace, Name: reportName}, &testReport)
	if apierrors.IsNotFound(err) {
		// 如果报告不存在，创建它
		testReport = testtoolsv1.TestReport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      reportName,
				Namespace: dig.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: dig.APIVersion,
						Kind:       dig.Kind,
						Name:       dig.Name,
						UID:        dig.UID,
						Controller: func() *bool { b := true; return &b }(),
					},
				},
			},
			Spec: testtoolsv1.TestReportSpec{
				TestName:     fmt.Sprintf("%s DNS Test", dig.Name),
				TestType:     "DNS",
				Target:       fmt.Sprintf("%s DNS查询", dig.Spec.Domain),
				TestDuration: 1800, // 默认30分钟
				Interval:     60,   // 默认每60秒收集一次结果
				ResourceSelectors: []testtoolsv1.ResourceSelector{
					{
						APIVersion: "testtools.xiaoming.com/v1",
						Kind:       "Dig",
						Name:       dig.Name,
						Namespace:  dig.Namespace,
					},
				},
			},
		}

		if err := r.Create(ctx, &testReport); err != nil {
			logger.Error(err, "Failed to create TestReport")
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil // 5秒后重试
		}
		logger.Info("Created new TestReport",
			"dig", dig.Name,
			"namespace", dig.Namespace,
			"reportName", reportName)
	} else if err != nil {
		// 其他错误，记录并返回
		logger.Error(err, "Failed to get TestReport")
		return ctrl.Result{}, err
	}

	// 第4步：确定是否需要运行 dig 命令
	shouldRun := true

	// 检查是否有最后执行时间，并根据调度计划确定是否需要运行
	if dig.Status.LastExecutionTime != nil && dig.Spec.Schedule != "" {
		// 假设Schedule表示的是执行间隔的秒数
		interval := 60 // 默认60秒
		if dig.Spec.Schedule != "" {
			if intervalValue, err := strconv.Atoi(dig.Spec.Schedule); err == nil && intervalValue > 0 {
				interval = intervalValue
			}
		}

		// 检查最后执行时间是否小于间隔时间
		elapsed := time.Since(dig.Status.LastExecutionTime.Time)
		if elapsed.Seconds() < float64(interval) {
			shouldRun = false
			logger.Info("Skipping execution due to schedule",
				"dig", dig.Name,
				"namespace", dig.Namespace,
				"elapsed", elapsed.Seconds(),
				"interval", interval)
		}
	}

	// 第5步：如果需要，执行 dig 命令并更新状态
	if shouldRun {
		// 执行 dig 命令
		logger.Info("Executing dig command",
			"dig", dig.Name,
			"namespace", dig.Namespace,
			"domain", dig.Spec.Domain,
			"reconcileID", reconcileID)

		execResult, execErr := r.executeDig(ctx, &dig, logger)

		// 获取最新的资源版本，以避免更新冲突
		if err := r.Get(ctx, req.NamespacedName, &dig); err != nil {
			logger.Error(err, "Failed to refresh Dig resource before status update")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		// 创建 dig 的深度拷贝
		digCopy := dig.DeepCopy()
		oldStatus := *digCopy.Status.DeepCopy()

		// 更新基本状态字段
		now := metav1.NewTime(time.Now())
		digCopy.Status.LastExecutionTime = &now
		digCopy.Status.QueryCount++

		// 根据执行结果更新成功/失败信息
		if execErr != nil {
			digCopy.Status.Status = "Failed"
			digCopy.Status.FailureCount++
			digCopy.Status.LastResult = execResult

			// 更新条件
			setCondition(&digCopy.Status.Conditions, metav1.Condition{
				Type:               "Available",
				Status:             metav1.ConditionFalse,
				Reason:             "ExecutionFailed",
				Message:            execErr.Error(),
				LastTransitionTime: now,
			})
		} else {
			digCopy.Status.Status = "Succeeded"
			digCopy.Status.SuccessCount++
			digCopy.Status.LastResult = execResult

			// Parse response time if available in the result
			responseTimeMs := parseResponseTime(execResult)
			if responseTimeMs > 0 {
				// Update average response time
				oldAvgTime := digCopy.Status.AverageResponseTime
				if digCopy.Status.AverageResponseTime == 0 {
					digCopy.Status.AverageResponseTime = responseTimeMs
				} else {
					// Calculate running average
					totalQueries := float64(digCopy.Status.SuccessCount)
					digCopy.Status.AverageResponseTime = ((digCopy.Status.AverageResponseTime * (totalQueries - 1)) + responseTimeMs) / totalQueries
				}
				// 检查平均响应时间是否有明显变化
				if math.Abs(oldAvgTime-digCopy.Status.AverageResponseTime) > 0.01 {
					// 标记状态已变化
				}
			}

			// 更新条件
			setCondition(&digCopy.Status.Conditions, metav1.Condition{
				Type:               "Available",
				Status:             metav1.ConditionTrue,
				Reason:             "ExecutionSucceeded",
				Message:            "Dig command executed successfully",
				LastTransitionTime: now,
			})
		}

		// 检查状态是否有实质性变化
		statusChanged := false
		if digCopy.Status.Status != oldStatus.Status ||
			digCopy.Status.LastResult != oldStatus.LastResult ||
			digCopy.Status.SuccessCount != oldStatus.SuccessCount ||
			digCopy.Status.FailureCount != oldStatus.FailureCount ||
			math.Abs(digCopy.Status.AverageResponseTime-oldStatus.AverageResponseTime) > 0.01 {
			statusChanged = true
		}

		// 如果状态有变化，则更新
		if statusChanged {
			logger.Info("Updating Dig status",
				"dig", dig.Name,
				"namespace", dig.Namespace)

			// 更新状态
			if err := r.Status().Update(ctx, digCopy); err != nil {
				logger.Error(err, "Failed to update Dig status")
				// 不要立即重试，让控制器在下一个协调周期自动重试
				return ctrl.Result{RequeueAfter: time.Second * 5}, nil
			}

			// 第6步：更新 TestReport 的测试结果
			// 只有在 dig 状态有变化且执行成功时才更新 TestReport
			if execErr == nil {
				// 重新获取最新的 TestReport
				var testReport testtoolsv1.TestReport
				if err := r.Get(ctx, client.ObjectKey{Namespace: dig.Namespace, Name: reportName}, &testReport); err != nil {
					logger.Error(err, "Failed to get TestReport for updating result")
					// 不阻止控制循环继续
				} else {
					// 创建新的测试结果
					testResult := testtoolsv1.TestResult{
						ResourceName:      dig.Name,
						ResourceNamespace: dig.Namespace,
						ResourceKind:      dig.Kind,
						ExecutionTime:     now,
						Success:           true,
						ResponseTime:      digCopy.Status.AverageResponseTime,
						Output:            execResult,
						RawOutput:         execResult,
					}

					// 添加 DNS 相关指标
					responseTimeMs := parseResponseTime(execResult)
					if responseTimeMs > 0 {
						if testResult.MetricValues == nil {
							testResult.MetricValues = make(map[string]string)
						}
						testResult.MetricValues["responseTime"] = fmt.Sprintf("%.2f", responseTimeMs)
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
						if lastResult.ResourceName == dig.Name &&
							lastResult.ResourceNamespace == dig.Namespace &&
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
						updateDigTestReportSummary(&testReportCopy.Status)

						// 更新 TestReport
						if err := r.Status().Update(ctx, testReportCopy); err != nil {
							logger.Error(err, "Failed to update TestReport status")
							// 不阻止控制循环继续
						} else {
							logger.Info("Updated TestReport with new result",
								"dig", dig.Name,
								"namespace", dig.Namespace,
								"reportName", reportName)
						}
					} else {
						logger.Info("Skipping TestReport update as similar result already exists",
							"dig", dig.Name,
							"namespace", dig.Namespace,
							"reportName", reportName)
					}
				}
			}
		} else {
			logger.Info("No significant status changes, skipping updates",
				"dig", dig.Name,
				"namespace", dig.Namespace)
		}
	}

	// 第7步：安排下一次执行
	interval := 60 // 默认60秒
	if dig.Spec.Schedule != "" {
		if intervalValue, err := strconv.Atoi(dig.Spec.Schedule); err == nil && intervalValue > 0 {
			interval = intervalValue
		}
	}

	return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, nil
}

// reconcileTestReport 确保 TestReport 资源存在，并返回是否需要重新排队
// 采用纯声明式方法，不进行多次更新操作
func (r *DigReconciler) reconcileTestReport(ctx context.Context, dig *testtoolsv1.Dig, logger logr.Logger) (string, bool, error) {
	reportName := fmt.Sprintf("dig-%s-report", dig.Name)
	logger.Info("Reconciling TestReport for Dig",
		"dig", dig.Name,
		"namespace", dig.Namespace,
		"reportName", reportName,
		"currentTestReportName", dig.Status.TestReportName)

	// 如果 Dig 资源尚未设置 TestReportName，则更新它
	if dig.Status.TestReportName == "" {
		digCopy := dig.DeepCopy()
		digCopy.Status.TestReportName = reportName
		if err := r.Status().Update(ctx, digCopy); err != nil {
			logger.Error(err, "Failed to update Dig status with TestReportName",
				"dig", dig.Name,
				"namespace", dig.Namespace,
				"reportName", reportName)
			// 即使更新失败，也返回 reportName，以便后续可以尝试更新 TestReport
			return reportName, false, err
		}
		// 状态已更新，需要重新进入 reconcile 循环
		logger.Info("Updated Dig status with TestReportName",
			"dig", dig.Name,
			"namespace", dig.Namespace,
			"reportName", reportName)
		return reportName, true, nil
	}

	// 检查 TestReport 是否存在
	var existingReport testtoolsv1.TestReport
	err := r.Get(ctx, client.ObjectKey{Namespace: dig.Namespace, Name: reportName}, &existingReport)
	if err == nil {
		// TestReport 已存在，无需操作
		logger.Info("TestReport already exists",
			"dig", dig.Name,
			"namespace", dig.Namespace,
			"reportName", reportName)
		return reportName, false, nil
	}

	// 如果是其他错误，返回错误，但仍返回 reportName
	if err := client.IgnoreNotFound(err); err != nil {
		logger.Error(err, "Failed to get TestReport",
			"dig", dig.Name,
			"namespace", dig.Namespace,
			"reportName", reportName)
		return reportName, false, err
	}

	// TestReport 不存在，创建它
	logger.Info("Creating new TestReport for Dig",
		"dig", dig.Name,
		"namespace", dig.Namespace,
		"reportName", reportName)
	newReport := testtoolsv1.TestReport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reportName,
			Namespace: dig.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: dig.APIVersion,
					Kind:       dig.Kind,
					Name:       dig.Name,
					UID:        dig.UID,
					Controller: func() *bool { b := true; return &b }(),
				},
			},
		},
		Spec: testtoolsv1.TestReportSpec{
			TestName:     fmt.Sprintf("%s DNS Test", dig.Name),
			TestType:     "DNS",
			Target:       fmt.Sprintf("%s DNS查询", dig.Spec.Domain),
			TestDuration: 1800, // 默认30分钟
			Interval:     60,   // 默认每60秒收集一次结果
			ResourceSelectors: []testtoolsv1.ResourceSelector{
				{
					APIVersion: "testtools.xiaoming.com/v1",
					Kind:       "Dig",
					Name:       dig.Name,
					Namespace:  dig.Namespace,
				},
			},
		},
	}

	// 创建 TestReport 资源
	if err := r.Create(ctx, &newReport); err != nil {
		logger.Error(err, "Failed to create TestReport",
			"dig", dig.Name,
			"namespace", dig.Namespace,
			"reportName", reportName)
		// 即使创建失败，也返回 reportName
		return reportName, false, err
	}

	logger.Info("Successfully created TestReport",
		"dig", dig.Name,
		"namespace", dig.Namespace,
		"reportName", reportName)
	return reportName, false, nil
}

// updateDigStatus 更新 Dig 资源的状态
func (r *DigReconciler) updateDigStatus(ctx context.Context, dig *testtoolsv1.Dig, result string, execErr error, logger logr.Logger) error {
	// 添加重试逻辑，最多重试3次
	var lastErr error
	for retries := 0; retries < 3; retries++ {
		// 添加退避延迟
		if retries > 0 {
			backoff := time.Duration(100*(1<<uint(retries))) * time.Millisecond // 指数退避: 100ms, 200ms, 400ms
			logger.Info("Retrying status update with backoff",
				"dig", dig.Name,
				"namespace", dig.Namespace,
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

		// 创建 Dig 的深度拷贝，以避免直接修改缓存中的对象
		digCopy := dig.DeepCopy()

		// 准备状态更新
		now := metav1.NewTime(time.Now())
		digCopy.Status.LastExecutionTime = &now
		digCopy.Status.QueryCount++

		// 仅在状态有变化时才进行更新
		statusChanged := false

		// Update success/failure info
		if execErr != nil {
			statusChanged = digCopy.Status.Status != "Failed" ||
				digCopy.Status.LastResult != result

			digCopy.Status.Status = "Failed"
			digCopy.Status.FailureCount++
			digCopy.Status.LastResult = result

			// Update conditions
			cond := metav1.Condition{
				Type:               "Available",
				Status:             metav1.ConditionFalse,
				Reason:             "ExecutionFailed",
				Message:            execErr.Error(),
				LastTransitionTime: now,
			}

			// 检查条件是否存在且内容相同
			statusChanged = statusChanged || !hasMatchingCondition(&digCopy.Status.Conditions, &cond)
			setCondition(&digCopy.Status.Conditions, cond)
		} else {
			statusChanged = digCopy.Status.Status != "Succeeded" ||
				digCopy.Status.LastResult != result

			digCopy.Status.Status = "Succeeded"
			digCopy.Status.SuccessCount++
			digCopy.Status.LastResult = result

			// Parse response time if available in the result
			responseTimeMs := parseResponseTime(result)
			if responseTimeMs > 0 {
				// Update average response time
				oldAvgTime := digCopy.Status.AverageResponseTime
				if digCopy.Status.AverageResponseTime == 0 {
					digCopy.Status.AverageResponseTime = responseTimeMs
				} else {
					// Calculate running average
					totalQueries := float64(digCopy.Status.SuccessCount)
					digCopy.Status.AverageResponseTime = ((digCopy.Status.AverageResponseTime * (totalQueries - 1)) + responseTimeMs) / totalQueries
				}
				statusChanged = statusChanged || (math.Abs(oldAvgTime-digCopy.Status.AverageResponseTime) > 0.01)
			}

			// Update conditions
			cond := metav1.Condition{
				Type:               "Available",
				Status:             metav1.ConditionTrue,
				Reason:             "ExecutionSucceeded",
				Message:            "Dig command executed successfully",
				LastTransitionTime: now,
			}

			// 检查条件是否存在且内容相同
			statusChanged = statusChanged || !hasMatchingCondition(&digCopy.Status.Conditions, &cond)
			setCondition(&digCopy.Status.Conditions, cond)
		}

		// 如果状态没有实质性变化，则跳过更新
		if !statusChanged && retries == 0 {
			logger.Info("Skipping status update as no significant changes detected",
				"dig", dig.Name,
				"namespace", dig.Namespace)
			return nil
		}

		// 使用服务器端应用，解决乐观并发冲突
		err := r.Status().Update(ctx, digCopy)
		if err == nil {
			// 更新成功
			return nil
		}

		// 记录错误并准备重试
		lastErr = err
		logger.Error(err, "Failed to update Dig status, will retry",
			"dig", dig.Name,
			"namespace", dig.Namespace,
			"retries", retries)

		// 处理不同类型的错误
		if apierrors.IsConflict(err) {
			// 发生冲突，下一次循环将使用最新版本重试
			// 重新获取最新的 Dig 资源
			if getErr := r.Get(ctx, client.ObjectKey{Namespace: dig.Namespace, Name: dig.Name}, dig); getErr != nil {
				logger.Error(getErr, "Failed to get fresh Dig resource after conflict",
					"dig", dig.Name,
					"namespace", dig.Namespace)
				// 如果无法获取最新资源，我们将在下一次循环使用当前资源重试
			}
			continue
		}

		// 处理速率限制错误
		if apierrors.IsTooManyRequests(err) || strings.Contains(err.Error(), "rate limit") {
			logger.Info("Rate limiting detected, backing off before retry",
				"dig", dig.Name,
				"namespace", dig.Namespace,
				"retries", retries)
			continue
		}

		// 处理服务器超时或不可用
		if apierrors.IsServerTimeout(err) || apierrors.IsTimeout(err) || apierrors.IsServiceUnavailable(err) {
			logger.Info("Server timeout or unavailable, retrying",
				"dig", dig.Name,
				"namespace", dig.Namespace,
				"retries", retries)
			continue
		}

		// 其他类型的错误，直接返回
		return err
	}

	// 所有重试都失败
	return fmt.Errorf("failed to update Dig status after retries: %w", lastErr)
}

// hasMatchingCondition 检查条件列表中是否有匹配的条件
func hasMatchingCondition(conditions *[]metav1.Condition, newCond *metav1.Condition) bool {
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
func (r *DigReconciler) updateTestReport(ctx context.Context, dig *testtoolsv1.Dig, result string, execErr error, logger logr.Logger) error {
	reportName := dig.Status.TestReportName
	if reportName == "" {
		return nil // 无需更新
	}

	// 添加重试逻辑
	var lastErr error
	var testReport *testtoolsv1.TestReport
	var testReportCopy *testtoolsv1.TestReport

	for retries := 0; retries < 3; retries++ {
		// 添加退避延迟
		if retries > 0 {
			backoff := time.Duration(100*(1<<uint(retries))) * time.Millisecond // 指数退避: 100ms, 200ms, 400ms
			logger.Info("Retrying TestReport update with backoff",
				"dig", dig.Name,
				"namespace", dig.Namespace,
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

		// 获取 TestReport 资源
		if testReport == nil || retries > 0 {
			testReport = &testtoolsv1.TestReport{}
			if err := r.Get(ctx, client.ObjectKey{Namespace: dig.Namespace, Name: reportName}, testReport); err != nil {
				lastErr = err
				logger.Error(err, "Failed to get TestReport for update",
					"dig", dig.Name,
					"namespace", dig.Namespace,
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
			if lastResult.ResourceName == dig.Name &&
				lastResult.ResourceNamespace == dig.Namespace &&
				lastResult.ResourceKind == dig.Kind {
				// 如果最后的执行时间在30秒内，且成功/失败状态相同，则认为是重复的测试结果
				if dig.Status.LastExecutionTime != nil && lastResult.ExecutionTime.Time.Sub(dig.Status.LastExecutionTime.Time) < 30*time.Second {
					if (execErr == nil && lastResult.Success) || (execErr != nil && !lastResult.Success) {
						logger.Info("Skipping TestReport update as similar result already exists",
							"dig", dig.Name,
							"namespace", dig.Namespace,
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
			ResourceName:      dig.Name,
			ResourceNamespace: dig.Namespace,
			ResourceKind:      dig.Kind,
			Success:           execErr == nil,
		}

		// 确保 LastExecutionTime 不为空
		if dig.Status.LastExecutionTime != nil {
			testResult.ExecutionTime = *dig.Status.LastExecutionTime
		} else {
			now := metav1.NewTime(time.Now())
			testResult.ExecutionTime = now
		}

		if execErr != nil {
			testResult.Error = execErr.Error()
		} else {
			testResult.ResponseTime = dig.Status.AverageResponseTime
			testResult.Output = result
			testResult.RawOutput = result

			// Add any specific DNS metrics
			responseTimeMs := parseResponseTime(result)
			if responseTimeMs > 0 {
				if testResult.MetricValues == nil {
					testResult.MetricValues = make(map[string]string)
				}
				testResult.MetricValues["responseTime"] = fmt.Sprintf("%.2f", responseTimeMs)
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
		updateDigTestReportSummary(&testReportCopy.Status)

		// 更新 TestReport 状态
		err := r.Status().Update(ctx, testReportCopy)
		if err == nil {
			// 更新成功
			return nil
		}

		// 记录错误并准备重试
		lastErr = err
		logger.Error(err, "Failed to update TestReport status, will retry",
			"dig", dig.Name,
			"namespace", dig.Namespace,
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

// updateDigTestReportSummary updates the summary in the TestReport status
func updateDigTestReportSummary(status *testtoolsv1.TestReportStatus) {
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

// executeDig executes the dig command with the given parameters
func (r *DigReconciler) executeDig(ctx context.Context, dig *testtoolsv1.Dig, logger logr.Logger) (string, error) {
	start := time.Now()

	// Validate dig command parameters
	if dig.Spec.Domain == "" {
		logger.Error(fmt.Errorf("domain is required"), "Invalid Dig specification",
			"dig", dig.Name,
			"namespace", dig.Namespace)
		return "", fmt.Errorf("domain is required")
	}

	// Construct the dig command
	args := r.buildDigArgs(dig)
	// 增加详细日志
	logger.Info("Preparing to execute dig command",
		"reconcileID", fmt.Sprintf("%s", ctx.Value("reconcileID")),
		"dig", dig.Name,
		"namespace", dig.Namespace,
		"domain", dig.Spec.Domain,
		"args", strings.Join(args, " "))

	// Create the command
	cmd := exec.Command("dig", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute the command
	cmdStr := fmt.Sprintf("dig %s", strings.Join(args, " "))
	logger.Info("Starting dig execution",
		"command", cmdStr,
		"dig", dig.Name,
		"namespace", dig.Namespace)

	err := cmd.Run()
	executionTime := time.Since(start)
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	// 详细记录执行结果
	logger.Info("Dig execution completed",
		"dig", dig.Name,
		"namespace", dig.Namespace,
		"duration", executionTime,
		"exitCode", exitCode,
		"command", cmdStr,
		"testReportName", dig.Status.TestReportName)

	// Process the result
	var result string
	if err != nil {
		stdoutStr := stdout.String()
		stderrStr := stderr.String()
		logger.Error(err, "Failed to execute dig command",
			"dig", dig.Name,
			"namespace", dig.Namespace,
			"stderr", stderrStr,
			"stdout", stdoutStr,
			"exitCode", exitCode,
			"duration", executionTime,
			"command", cmdStr,
			"testReportName", dig.Status.TestReportName)

		result = stderrStr
		if result == "" {
			result = stdoutStr
		}
		return result, fmt.Errorf("dig command failed: %w (exit code: %d)", err, exitCode)
	}

	result = stdout.String()
	resultLines := strings.Split(result, "\n")
	firstLine := ""
	if len(resultLines) > 0 {
		firstLine = resultLines[0]
	}

	// 成功执行的详细日志
	logger.Info("Dig command executed successfully",
		"dig", dig.Name,
		"namespace", dig.Namespace,
		"duration", executionTime,
		"outputLength", len(result),
		"firstLine", firstLine,
		"queryCount", dig.Status.QueryCount,
		"successCount", dig.Status.SuccessCount,
		"testReportName", dig.Status.TestReportName)

	return result, nil
}

// buildDigArgs constructs the arguments for the dig command based on the CR spec
func (r *DigReconciler) buildDigArgs(dig *testtoolsv1.Dig) []string {
	var args []string

	// Add the server if specified
	if dig.Spec.Server != "" {
		args = append(args, "@"+dig.Spec.Server)
	}

	// Add source IP and port if specified
	if dig.Spec.SourceIP != "" {
		sourceArg := "-b " + dig.Spec.SourceIP
		if dig.Spec.SourcePort > 0 {
			sourceArg += "#" + strconv.Itoa(int(dig.Spec.SourcePort))
		}
		args = append(args, sourceArg)
	}

	// Add query class if specified
	if dig.Spec.QueryClass != "" {
		args = append(args, "-c", dig.Spec.QueryClass)
	}

	// Add query file if specified
	if dig.Spec.QueryFile != "" {
		args = append(args, "-f", dig.Spec.QueryFile)
	}

	// Add key file if specified
	if dig.Spec.KeyFile != "" {
		args = append(args, "-k", dig.Spec.KeyFile)
	}

	// Add port if specified
	if dig.Spec.Port > 0 {
		args = append(args, "-p", strconv.Itoa(int(dig.Spec.Port)))
	}

	// Add query name if specified
	if dig.Spec.QueryName != "" {
		args = append(args, "-q", dig.Spec.QueryName)
	}

	// Add query type if specified
	if dig.Spec.QueryType != "" {
		args = append(args, "-t", dig.Spec.QueryType)
	}

	// Add microseconds flag if specified
	if dig.Spec.UseMicroseconds {
		args = append(args, "-u")
	}

	// Add reverse query if specified
	if dig.Spec.ReverseQuery != "" {
		args = append(args, "-x", dig.Spec.ReverseQuery)
	}

	// Add TSIG key if specified
	if dig.Spec.TSIGKey != "" {
		args = append(args, "-y", dig.Spec.TSIGKey)
	}

	// Add IPv4/IPv6 flags if specified
	if dig.Spec.UseIPv4Only {
		args = append(args, "-4")
	}
	if dig.Spec.UseIPv6Only {
		args = append(args, "-6")
	}

	// Add TCP flag if specified
	if dig.Spec.UseTCP {
		args = append(args, "+tcp")
	}

	// Add timeout if specified
	if dig.Spec.Timeout > 0 {
		args = append(args, "+time="+strconv.Itoa(int(dig.Spec.Timeout)))
	}

	// Add the domain (required)
	args = append(args, dig.Spec.Domain)

	return args
}

// parseResponseTime parses the query time from dig output
func parseResponseTime(output string) float64 {
	// Sample dig output line: ";; Query time: 23 msec"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Query time") {
			parts := strings.Split(line, ":")
			if len(parts) < 2 {
				continue
			}
			timePart := strings.TrimSpace(parts[1])
			msecParts := strings.Split(timePart, " ")
			if len(msecParts) < 2 {
				continue
			}
			if val, err := strconv.ParseFloat(msecParts[0], 64); err == nil {
				return val
			}
		}
	}
	return 0
}

// setCondition sets the given condition in the condition list
func setCondition(conditions *[]metav1.Condition, newCondition metav1.Condition) {
	// If conditions is nil, create a new slice
	if *conditions == nil {
		*conditions = []metav1.Condition{}
	}

	// Find the condition
	var existingCondition *metav1.Condition
	for i := range *conditions {
		if (*conditions)[i].Type == newCondition.Type {
			existingCondition = &(*conditions)[i]
			break
		}
	}

	// If condition doesn't exist, add it
	if existingCondition == nil {
		*conditions = append(*conditions, newCondition)
		return
	}

	// Update existing condition
	if existingCondition.Status != newCondition.Status {
		existingCondition.LastTransitionTime = newCondition.LastTransitionTime
	}
	existingCondition.Status = newCondition.Status
	existingCondition.Reason = newCondition.Reason
	existingCondition.Message = newCondition.Message
}

// SetupWithManager sets up the controller with the Manager.
func (r *DigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testtoolsv1.Dig{}).
		Complete(r)
}
