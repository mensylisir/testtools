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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/xiaoming/testtools/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
)

// DigFinalizerName 是用于Dig资源finalizer的名称
const DigFinalizerName = "dig.testtools.xiaoming.com/finalizer"

// DigReconciler reconciles a Dig object
type DigReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
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

	// 处理资源删除
	if !dig.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&dig, DigFinalizerName) {
			logger.Info("处理Dig资源删除", "digName", dig.Name)
			if err := r.cleanupResources(ctx, &dig); err != nil {
				logger.Error(err, "清理资源失败")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&dig, DigFinalizerName)
			if err := r.Update(ctx, &dig); err != nil {
				logger.Error(err, "移除finalizer失败")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// 确保finalizer存在
	if !controllerutil.ContainsFinalizer(&dig, DigFinalizerName) {
		controllerutil.AddFinalizer(&dig, DigFinalizerName)
		if err := r.Update(ctx, &dig); err != nil {
			logger.Error(err, "添加finalizer失败")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
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

	var job batchv1.Job
	if err = r.Client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: jobName}, &job); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Dig job not found, creating...", "dig", req.Name, "jobName", jobName, "namespace", req.Namespace)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Job", "dig", req.Name, "jobName", jobName)
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	} else {
		if job.Status.Succeeded > 0 {
			logger.Info("Job completed successfully", "dig", req.Name, "jobName", jobName, "namespace", req.Namespace)
			jobOutput, err := utils.GetJobResults(ctx, r.Client, dig.Namespace, jobName)
			if err != nil {
				logger.Error(err, "Failed to get Dig job results")
				digCopy.Status.Status = "Failed"
				digCopy.Status.FailureCount++
				digCopy.Status.LastResult = fmt.Sprintf("Error getting Dig job results: %v", err)

				// 设置TestReportName，即使失败也设置
				if digCopy.Status.TestReportName == "" {
					digCopy.Status.TestReportName = utils.GenerateTestReportName("Dig", dig.Name)
				}

				// 确保所有重要字段都被设置
				if digCopy.Status.QueryCount == 0 {
					digCopy.Status.QueryCount = 1
				}
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

				if digOutput.Status == "SUCCESS" {
					// 更新状态信息
					digCopy.Status.Status = "Succeeded"
					// 正确设置SuccessCount - 如果没有schedule则设置为1而不是递增
					if dig.Spec.Schedule == "" {
						digCopy.Status.SuccessCount = 1
					} else {
						digCopy.Status.SuccessCount++
					}
					digCopy.Status.LastResult = jobOutput
					digCopy.Status.AverageResponseTime = fmt.Sprintf("%f", digOutput.QueryTime)

					// 设置TestReportName，使TestReport控制器能够自动创建报告
					// 如果TestReportName为空，则设置为默认格式
					if digCopy.Status.TestReportName == "" {
						digCopy.Status.TestReportName = utils.GenerateTestReportName("Dig", dig.Name)
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
					// 更新状态信息
					digCopy.Status.Status = "Failed"
					// 正确设置SuccessCount - 如果没有schedule则设置为1而不是递增
					if dig.Spec.Schedule == "" {
						digCopy.Status.FailureCount = 1
					} else {
						digCopy.Status.FailureCount++
					}
					digCopy.Status.LastResult = jobOutput
					digCopy.Status.AverageResponseTime = fmt.Sprintf("%f", digOutput.QueryTime)

					// 设置TestReportName，使TestReport控制器能够自动创建报告
					// 如果TestReportName为空，则设置为默认格式
					if digCopy.Status.TestReportName == "" {
						digCopy.Status.TestReportName = utils.GenerateTestReportName("Dig", dig.Name)
					}

					// 确保所有重要字段都被设置
					if digCopy.Status.QueryCount == 0 {
						digCopy.Status.QueryCount = 1
					}

					// 记录详细数据
					logger.Info("Dig test completed failed",
						"domain", dig.Spec.Domain,
						"server", digOutput.Server,
						"status", digOutput.Status,
						"queryTime", digOutput.QueryTime,
						"ipAddressCount", len(digOutput.IpAddresses))

					// 标记测试已完成
					utils.SetCondition(&digCopy.Status.Conditions, metav1.Condition{
						Type:               "Failed",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "TestCompleted",
						Message:            "Dig test completed failed",
					})
				}

			}
			if updateErr := r.Status().Update(ctx, digCopy); updateErr != nil {
				logger.Info(updateErr.Error(), "failed to update Dig status", "dig", dig.Name, "namespace", dig.Namespace)
				return ctrl.Result{RequeueAfter: time.Second * 30}, nil
			}
		} else if job.Status.Failed > *job.Spec.BackoffLimit {
			logger.Error(nil, "Job failed", "dig", req.Name, "jobName", jobName, "namespace", req.Namespace,
				"failed", job.Status.Failed, "backoffLimit", *job.Spec.BackoffLimit)
			jobOutput, err := utils.GetJobResults(ctx, r.Client, dig.Namespace, jobName)
			if err != nil {
				logger.Error(err, "Failed to get Dig job results")
				digCopy.Status.Status = "Failed"
				digCopy.Status.FailureCount++
				digCopy.Status.LastResult = fmt.Sprintf("Error getting Dig job results: %v", err)

				// 设置TestReportName，即使失败也设置
				if digCopy.Status.TestReportName == "" {
					digCopy.Status.TestReportName = utils.GenerateTestReportName("Dig", dig.Name)
				}

				// 确保所有重要字段都被设置
				if digCopy.Status.QueryCount == 0 {
					digCopy.Status.QueryCount = 1
				}
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
				digCopy.Status.Status = "Failed"
				// 正确设置SuccessCount - 如果没有schedule则设置为1而不是递增
				if dig.Spec.Schedule == "" {
					digCopy.Status.FailureCount = 1
				} else {
					digCopy.Status.FailureCount++
				}
				digCopy.Status.LastResult = jobOutput
				digCopy.Status.AverageResponseTime = fmt.Sprintf("%f", digOutput.QueryTime)

				// 设置TestReportName，使TestReport控制器能够自动创建报告
				// 如果TestReportName为空，则设置为默认格式
				if digCopy.Status.TestReportName == "" {
					digCopy.Status.TestReportName = utils.GenerateTestReportName("Dig", dig.Name)
				}

				// 确保所有重要字段都被设置
				if digCopy.Status.QueryCount == 0 {
					digCopy.Status.QueryCount = 1
				}

				// 记录详细数据
				logger.Info("Dig test completed failed",
					"domain", dig.Spec.Domain,
					"server", digOutput.Server,
					"status", digOutput.Status,
					"queryTime", digOutput.QueryTime,
					"ipAddressCount", len(digOutput.IpAddresses))

				// 标记测试已完成
				utils.SetCondition(&digCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "TestCompleted",
					Message:            "Dig test completed failed",
				})
			}
			if updateErr := r.Status().Update(ctx, digCopy); updateErr != nil {
				logger.Info(updateErr.Error(), "failed to update Dig status", "dig", dig.Name, "namespace", dig.Namespace)
				return ctrl.Result{RequeueAfter: time.Second * 30}, nil
			}

		} else {
			logger.Info("Job is still running", "dig", req.Name, "jobName", jobName, "namespace", req.Namespace,
				"succeeded", job.Status.Succeeded, "failed", job.Status.Failed, "backoffLimit", *job.Spec.BackoffLimit)
			return ctrl.Result{}, nil
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
		Owns(&batchv1.Job{}).
		Complete(r)
}

// cleanupResources 清理与Dig资源关联的所有资源
func (r *DigReconciler) cleanupResources(ctx context.Context, dig *testtoolsv1.Dig) error {
	logger := log.FromContext(ctx)
	logger.Info("清理资源", "digName", dig.Name, "说明", "使用OwnerReference自动级联删除资源")
	// 不需要手动删除资源，因为所有关联资源都通过OwnerReference设置了级联删除
	return nil
}
