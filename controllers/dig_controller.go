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
	"strconv"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/xiaoming/testtools/pkg/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// DigReconciler reconciles a Dig object
type DigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=digs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=digs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=digs/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("开始Dig调谐", "namespacedName", req.NamespacedName)

	// 记录执行时间
	start := time.Now()
	defer func() {
		logger.Info("完成Dig调谐", "namespacedName", req.NamespacedName, "duration", time.Since(start))
	}()

	// 获取Dig实例
	var dig testtoolsv1.Dig
	if err := r.Get(ctx, req.NamespacedName, &dig); err != nil {
		if apierrors.IsNotFound(err) {
			// Dig资源已被删除，忽略
			logger.Info("Dig资源已被删除，忽略", "namespacedName", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "获取Dig资源失败", "namespacedName", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// 添加finalizer处理逻辑
	digFinalizer := "testtools.xiaoming.com/dig-finalizer"

	// 检查对象是否正在被删除
	if dig.ObjectMeta.DeletionTimestamp.IsZero() {
		// 对象没有被标记为删除，确保它有我们的finalizer
		if !utils.ContainsString(dig.GetFinalizers(), digFinalizer) {
			logger.Info("添加finalizer", "finalizer", digFinalizer)
			dig.SetFinalizers(append(dig.GetFinalizers(), digFinalizer))
			if err := r.Update(ctx, &dig); err != nil {
				return ctrl.Result{}, err
			}
			// 已更新finalizer，重新触发reconcile
			return ctrl.Result{}, nil
		}
	} else {
		// 对象正在被删除
		if utils.ContainsString(dig.GetFinalizers(), digFinalizer) {
			// 执行清理操作
			if err := r.cleanupResources(ctx, &dig); err != nil {
				logger.Error(err, "清理资源失败")
				return ctrl.Result{}, err
			}

			// 清理完成后，移除finalizer
			dig.SetFinalizers(utils.RemoveString(dig.GetFinalizers(), digFinalizer))
			if err := r.Update(ctx, &dig); err != nil {
				return ctrl.Result{}, err
			}
			logger.Info("已移除finalizer，允许资源被删除")
			return ctrl.Result{}, nil
		}
	}

	// 检查是否需要执行测试
	shouldExecute := false

	// 检查是否设置了调度规则
	if dig.Spec.Schedule != "" {
		// 如果有上次执行时间，检查是否到了下次执行时间
		if dig.Status.LastExecutionTime != nil {
			// 计算下次执行时间
			intervalSeconds := 60 // 默认60秒
			if scheduleValue, err := strconv.Atoi(dig.Spec.Schedule); err == nil && scheduleValue > 0 {
				intervalSeconds = scheduleValue
			}

			nextExecutionTime := dig.Status.LastExecutionTime.Time.Add(time.Duration(intervalSeconds) * time.Second)
			shouldExecute = time.Now().After(nextExecutionTime)

			if !shouldExecute {
				logger.Info("还未到调度时间，跳过执行",
					"dig", dig.Name,
					"上次执行", dig.Status.LastExecutionTime.Time,
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
		shouldExecute = dig.Status.LastExecutionTime == nil
	}

	if !shouldExecute {
		logger.Info("无需执行Dig测试",
			"dig", dig.Name,
			"lastExecutionTime", dig.Status.LastExecutionTime,
			"schedule", dig.Spec.Schedule)
		return ctrl.Result{}, nil
	}

	// 更新状态，记录开始时间
	digCopy := dig.DeepCopy()
	now := metav1.NewTime(time.Now())
	digCopy.Status.LastExecutionTime = &now
	// 在这里设置执行标记，但仅在首次执行时增加QueryCount
	if dig.Status.QueryCount == 0 {
		// 如果是首次执行，设置查询计数为1
		digCopy.Status.QueryCount = 1
	}

	// 准备 Job 执行 Dig 测试
	jobName, err := utils.PrepareDigJob(ctx, r.Client, &dig)
	if err != nil {
		logger.Error(err, "Failed to prepare Dig job")
		digCopy.Status.Status = "Failed"
		digCopy.Status.FailureCount++
		digCopy.Status.LastResult = fmt.Sprintf("Error preparing Dig job: %v", err)

		// 添加失败条件
		utils.SetCondition(&digCopy.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "JobPreparationFailed",
			Message:            fmt.Sprintf("Dig测试准备作业失败: %v", err),
		})

		// 更新状态
		if updateErr := r.Status().Update(ctx, digCopy); updateErr != nil {
			logger.Error(updateErr, "Failed to update Dig status after job preparation failure")
		}

		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	// 构建并保存执行的命令
	digArgs := utils.BuildDigArgs(&dig)
	digCopy.Status.ExecutedCommand = fmt.Sprintf("dig %s", strings.Join(digArgs, " "))
	logger.Info("Setting executed command", "command", digCopy.Status.ExecutedCommand)

	// 等待 Job 完成
	logger.Info("Waiting for Dig job to complete", "jobName", jobName)
	err = utils.WaitForJob(ctx, r.Client, dig.Namespace, jobName, 5*time.Minute)
	if err != nil {
		logger.Error(err, "Failed while waiting for Dig job to complete")
		digCopy.Status.Status = "Failed"
		digCopy.Status.FailureCount++
		digCopy.Status.LastResult = fmt.Sprintf("Error waiting for Dig job: %v", err)

		// 添加失败条件
		utils.SetCondition(&digCopy.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "JobExecutionFailed",
			Message:            fmt.Sprintf("Dig测试执行失败: %v", err),
		})

		// 更新状态
		if updateErr := r.Status().Update(ctx, digCopy); updateErr != nil {
			logger.Error(updateErr, "Failed to update Dig status after job execution failure")
		}

		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	// 获取 Job 执行结果
	jobOutput, err := utils.GetJobResults(ctx, r.Client, dig.Namespace, jobName)
	if err != nil {
		logger.Error(err, "Failed to get Dig job results")
		digCopy.Status.Status = "Failed"
		digCopy.Status.FailureCount++
		digCopy.Status.LastResult = fmt.Sprintf("Error getting Dig job results: %v", err)

		// 添加失败条件
		utils.SetCondition(&digCopy.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "ResultRetrievalFailed",
			Message:            fmt.Sprintf("无法获取Dig测试结果: %v", err),
		})
	} else {
		// 解析 Dig 输出并更新状态
		digOutput := utils.ParseDigOutput(jobOutput)

		// 更新状态信息
		digCopy.Status.Status = "Succeeded"
		// 正确设置SuccessCount - 如果没有schedule则设置为1而不是递增
		if dig.Spec.Schedule == "" {
			digCopy.Status.SuccessCount = 1
		} else {
			digCopy.Status.SuccessCount++
		}
		digCopy.Status.LastResult = jobOutput
		digCopy.Status.AverageResponseTime = digOutput.QueryTime

		// 设置TestReportName，使TestReport控制器能够自动创建报告
		// 如果TestReportName为空，则设置为默认格式
		if digCopy.Status.TestReportName == "" {
			digCopy.Status.TestReportName = fmt.Sprintf("dig-%s-report", dig.Name)
		}

		// 确保所有重要字段都被设置
		if digCopy.Status.QueryCount == 0 {
			digCopy.Status.QueryCount = 1
		}

		// 记录详细数据
		logger.Info("Dig测试成功完成",
			"domain", dig.Spec.Domain,
			"server", digOutput.Server,
			"status", digOutput.Status,
			"queryTime", digOutput.QueryTime,
			"ipAddressCount", len(digOutput.IpAddresses))

		// 标记测试已完成
		utils.SetCondition(&digCopy.Status.Conditions, metav1.Condition{
			Type:               "Completed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "TestCompleted",
			Message:            "Dig测试已成功完成",
		})
	}

	// 更新状态
	maxUpdateRetries := 5
	updateRetryDelay := 200 * time.Millisecond

	for i := 0; i < maxUpdateRetries; i++ {
		// 如果不是首次尝试，重新获取对象和重新应用更改
		if i > 0 {
			// 重新获取最新的Dig对象
			if err := r.Get(ctx, req.NamespacedName, &dig); err != nil {
				logger.Error(err, "Failed to re-fetch Dig for status update retry", "attempt", i+1)
				if errors.IsNotFound(err) {
					return ctrl.Result{}, nil
				}
				return ctrl.Result{}, err
			}

			// 重新创建更新对象
			digCopy = dig.DeepCopy()
			digCopy.Status.LastExecutionTime = &now
			digCopy.Status.ExecutedCommand = fmt.Sprintf("dig %s", strings.Join(digArgs, " "))

			// 如果是首次执行（QueryCount为0），设置为1
			if dig.Status.QueryCount == 0 {
				digCopy.Status.QueryCount = 1
			}

			// 重新应用所有状态更改
			if jobOutput != "" {
				// 解析 Dig 输出并更新状态
				digOutput := utils.ParseDigOutput(jobOutput)

				// 更新状态信息
				digCopy.Status.Status = "Succeeded"
				// 正确设置SuccessCount - 如果没有schedule则设置为1而不是递增
				if dig.Spec.Schedule == "" {
					digCopy.Status.SuccessCount = 1
				} else {
					digCopy.Status.SuccessCount = dig.Status.SuccessCount + 1
				}
				digCopy.Status.LastResult = jobOutput
				digCopy.Status.AverageResponseTime = digOutput.QueryTime

				// 设置TestReportName，使TestReport控制器能够自动创建报告
				// 如果TestReportName为空，则设置为默认格式
				if digCopy.Status.TestReportName == "" {
					digCopy.Status.TestReportName = fmt.Sprintf("dig-%s-report", dig.Name)
				}

				// 确保所有重要字段都被设置
				if digCopy.Status.QueryCount == 0 {
					digCopy.Status.QueryCount = 1
				}

				// 记录详细数据
				logger.Info("Dig测试成功完成",
					"domain", dig.Spec.Domain,
					"server", digOutput.Server,
					"status", digOutput.Status,
					"queryTime", digOutput.QueryTime,
					"ipAddressCount", len(digOutput.IpAddresses))

				// 标记测试已完成
				utils.SetCondition(&digCopy.Status.Conditions, metav1.Condition{
					Type:               "Completed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "TestCompleted",
					Message:            "Dig测试已成功完成",
				})
			} else {
				// 错误状态更新
				digCopy.Status.Status = "Failed"
				digCopy.Status.FailureCount = dig.Status.FailureCount + 1

				// 标记失败状态
				utils.SetCondition(&digCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "ResultRetrievalFailed",
					Message:            "无法获取Dig测试结果",
				})
			}

			logger.Info("Retrying Dig status update", "attempt", i+1, "maxRetries", maxUpdateRetries)
		}

		if updateErr := r.Status().Update(ctx, digCopy); updateErr != nil {
			if apierrors.IsConflict(updateErr) {
				logger.Info("Conflict when updating Dig status, will retry",
					"attempt", i+1,
					"maxRetries", maxUpdateRetries)
				if i < maxUpdateRetries-1 {
					time.Sleep(updateRetryDelay)
					updateRetryDelay *= 2 // 指数退避
					continue
				}
			}
			logger.Error(updateErr, "Failed to update Dig status")
			return ctrl.Result{RequeueAfter: time.Second * 5}, updateErr
		} else {
			// 成功更新，退出循环
			logger.Info("Successfully updated Dig status")
			break
		}
	}

	// 只有在明确设置了Schedule字段时才安排下一次执行
	if dig.Spec.Schedule != "" {
		interval := 60 // 默认60秒
		if dig.Spec.Schedule != "" {
			if intervalValue, err := strconv.Atoi(dig.Spec.Schedule); err == nil && intervalValue > 0 {
				interval = intervalValue
			}
		}
		logger.Info("Scheduling next Dig test", "interval", interval)
		return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, nil
	}

	// 默认情况下，不再调度下一次执行
	logger.Info("Dig测试已完成，不再调度下一次执行",
		"dig", dig.Name,
		"namespace", dig.Namespace)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testtoolsv1.Dig{}).
		Complete(r)
}

// cleanupResources 清理与Dig资源关联的所有资源
func (r *DigReconciler) cleanupResources(ctx context.Context, dig *testtoolsv1.Dig) error {
	logger := log.FromContext(ctx)

	// 查找并清理关联的Job
	var jobList batchv1.JobList
	if err := r.List(ctx, &jobList, client.InNamespace(dig.Namespace),
		client.MatchingLabels{
			"app.kubernetes.io/created-by": "testtools-controller",
			"app.kubernetes.io/managed-by": "dig-controller",
			"app.kubernetes.io/name":       "dig-job",
			"app.kubernetes.io/instance":   dig.Name,
		}); err != nil {
		logger.Error(err, "无法列出关联的Job")
		return err
	}

	for _, job := range jobList.Items {
		logger.Info("删除关联的Job", "jobName", job.Name)
		// 设置删除宽限期为0，立即删除
		propagationPolicy := metav1.DeletePropagationBackground
		deleteOptions := client.DeleteOptions{
			PropagationPolicy: &propagationPolicy,
		}
		if err := r.Delete(ctx, &job, &deleteOptions); err != nil && !apierrors.IsNotFound(err) {
			logger.Error(err, "删除Job失败", "jobName", job.Name)
			return err
		}
	}

	// 不再删除TestReport资源，这应该由TestReport控制器自己管理
	// TestReport通过其ResourceSelectors引用Dig，当Dig删除时，TestReport控制器会负责清理或更新TestReport

	logger.Info("资源清理完成", "digName", dig.Name)
	return nil
}
