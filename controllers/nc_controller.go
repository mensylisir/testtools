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
	"k8s.io/apimachinery/pkg/types"

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

// NcFinalizerName 是用于Nc资源finalizer的名称
const NcFinalizerName = "nc.testtools.xiaoming.com/finalizer"

// NcReconciler reconciles a Nc object
type NcReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=ncs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=ncs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=ncs/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods/log,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *NcReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("开始Nc调谐", "namespacedName", req.NamespacedName)

	// 记录执行时间
	start := time.Now()
	defer func() {
		logger.Info("完成Nc调谐", "namespacedName", req.NamespacedName, "duration", time.Since(start))
	}()

	// 获取Nc实例
	var nc testtoolsv1.Nc
	if err := r.Get(ctx, req.NamespacedName, &nc); err != nil {
		if apierrors.IsNotFound(err) {
			// Nc资源已被删除，忽略
			logger.Info("Nc资源已被删除，忽略", "namespacedName", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "获取Nc资源失败", "namespacedName", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// 处理资源删除
	if !nc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&nc, NcFinalizerName) {
			logger.Info("处理Nc资源删除", "ncName", nc.Name)
			if err := r.cleanupResources(ctx, &nc); err != nil {
				logger.Error(err, "清理资源失败")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&nc, NcFinalizerName)
			if err := r.Update(ctx, &nc); err != nil {
				logger.Error(err, "移除finalizer失败")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// 确保finalizer存在
	if !controllerutil.ContainsFinalizer(&nc, NcFinalizerName) {
		controllerutil.AddFinalizer(&nc, NcFinalizerName)
		if err := r.Update(ctx, &nc); err != nil {
			logger.Error(err, "添加finalizer失败")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// 检查是否需要执行测试
	shouldExecute := false

	// 检查是否设置了调度规则
	if nc.Spec.Schedule != "" {
		// 如果有上次执行时间，检查是否到了下次执行时间
		if nc.Status.LastExecutionTime != nil {
			// 计算下次执行时间
			intervalSeconds := 60 // 默认60秒
			if scheduleValue, err := strconv.Atoi(nc.Spec.Schedule); err == nil && scheduleValue > 0 {
				intervalSeconds = scheduleValue
			}

			nextExecutionTime := nc.Status.LastExecutionTime.Time.Add(time.Duration(intervalSeconds) * time.Second)
			shouldExecute = time.Now().After(nextExecutionTime)

			if !shouldExecute {
				logger.Info("还未到调度时间，跳过执行",
					"nc", nc.Name,
					"上次执行", nc.Status.LastExecutionTime.Time,
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
		shouldExecute = nc.Status.LastExecutionTime == nil
	}

	if !shouldExecute {
		logger.Info("无需执行Nc测试",
			"nc", nc.Name,
			"lastExecutionTime", nc.Status.LastExecutionTime,
			"schedule", nc.Spec.Schedule)
		return ctrl.Result{}, nil
	}

	// 更新状态，记录开始时间
	ncCopy := nc.DeepCopy()
	now := metav1.NewTime(time.Now())
	ncCopy.Status.LastExecutionTime = &now

	// 准备 Job 执行 Nc 测试
	jobName, err := utils.PrepareNcJob(ctx, r.Client, &nc)
	if err != nil {
		logger.Error(err, "Failed to prepare Nc job")
		ncCopy.Status.Status = "Failed"
		ncCopy.Status.FailureCount++
		ncCopy.Status.LastResult = fmt.Sprintf("Error preparing Nc job: %v", err)

		// 添加失败条件
		utils.SetCondition(&ncCopy.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "JobPreparationFailed",
			Message:            fmt.Sprintf("Nc测试准备作业失败: %v", err),
		})

		// 更新状态
		if updateErr := r.Status().Update(ctx, ncCopy); updateErr != nil {
			logger.Error(updateErr, "Failed to update Nc status after job preparation failure")
		}

		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	// 构建并保存执行的命令
	ncArgs := utils.BuildNCArgs(&nc)
	ncCopy.Status.ExecutedCommand = fmt.Sprintf("nc %s", strings.Join(ncArgs, " "))
	logger.Info("Setting executed command", "command", ncCopy.Status.ExecutedCommand)

	var job batchv1.Job
	if err = r.Client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: jobName}, &job); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Nc job not found, creating...", "nc", req.Name, "jobName", jobName, "namespace", req.Namespace)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Job", "nc", req.Name, "jobName", jobName)
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	} else {
		if job.Status.Succeeded > 0 {
			logger.Info("Job completed successfully", "nc", req.Name, "jobName", jobName, "namespace", req.Namespace)

			// 获取 Job 执行结果
			jobOutput, err := utils.GetJobResults(ctx, r.Client, nc.Namespace, jobName)
			if err != nil {
				logger.Error(err, "Failed to get Nc job results", "nc", nc.Name, "namespace", req.Namespace, "jobName", jobName)
				ncCopy.Status.Status = "Failed"
				ncCopy.Status.FailureCount++
				ncCopy.Status.LastResult = fmt.Sprintf("Error getting Nc job results: %v", err)

				// 设置TestReportName，即使失败也设置
				if ncCopy.Status.TestReportName == "" {
					ncCopy.Status.TestReportName = utils.GenerateTestReportName("Nc", nc.Name)
				}

				if ncCopy.Status.QueryCount == 0 {
					ncCopy.Status.QueryCount = 1
				}

				// 添加失败条件
				utils.SetCondition(&ncCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "ResultRetrievalFailed",
					Message:            fmt.Sprintf("无法获取Nc测试结果: %v", err),
				})
			} else {
				// 解析 Nc 输出并更新状态
				ncOutput := utils.ParseNCOutput(jobOutput, ncCopy)

				// 更新状态信息
				ncCopy.Status.Status = "Succeeded"
				// 正确设置SuccessCount - 如果没有schedule则设置为1而不是递增
				if nc.Spec.Schedule == "" {
					ncCopy.Status.SuccessCount = 1
				} else {
					ncCopy.Status.SuccessCount++
				}
				ncCopy.Status.LastResult = jobOutput

				// 记录详细数据
				logger.Info("Nc测试成功完成",
					"host", ncOutput.Host,
					"port", ncOutput.Port)

				// 标记测试已完成
				utils.SetCondition(&ncCopy.Status.Conditions, metav1.Condition{
					Type:               "Completed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "TestCompleted",
					Message:            "Nc测试已成功完成",
				})

				// 设置TestReportName，使TestReport控制器能够自动创建报告
				// 如果TestReportName为空，则设置为默认格式
				if ncCopy.Status.TestReportName == "" {
					ncCopy.Status.TestReportName = utils.GenerateTestReportName("Nc", nc.Name)
				}

				if ncCopy.Status.QueryCount == 0 {
					ncCopy.Status.QueryCount = 1
				}
			}

			if updateErr := r.Status().Update(ctx, ncCopy); updateErr != nil {
				logger.Info(updateErr.Error(), "failed to update nc status", "nc", nc.Name, "namespace", nc.Namespace)
				return ctrl.Result{RequeueAfter: time.Second * 30}, nil
			}

		} else if job.Status.Failed > *job.Spec.BackoffLimit {
			logger.Error(nil, "Job failed", "nc", req.Name, "jobName", jobName, "namespace", req.Namespace,
				"failed", job.Status.Failed, "backoffLimit", *job.Spec.BackoffLimit)

			// 获取 Job 执行结果
			jobOutput, err := utils.GetJobResults(ctx, r.Client, nc.Namespace, jobName)
			if err != nil {
				logger.Error(err, "Failed to get Nc job results", "nc", nc.Name, "namespace", req.Namespace, "jobName", jobName)
				ncCopy.Status.Status = "Failed"
				ncCopy.Status.FailureCount++
				ncCopy.Status.LastResult = fmt.Sprintf("Error getting Nc job results: %v", err)

				// 设置TestReportName，即使失败也设置
				if ncCopy.Status.TestReportName == "" {
					ncCopy.Status.TestReportName = utils.GenerateTestReportName("Nc", nc.Name)
				}

				if ncCopy.Status.QueryCount == 0 {
					ncCopy.Status.QueryCount = 1
				}

				// 添加失败条件
				utils.SetCondition(&ncCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "ResultRetrievalFailed",
					Message:            fmt.Sprintf("无法获取Nc测试结果: %v", err),
				})
			} else {
				// 解析 Nc 输出并更新状态
				ncOutput := utils.ParseNCOutput(jobOutput, ncCopy)

				// 更新状态信息
				ncCopy.Status.Status = "Failed"
				// 正确设置SuccessCount - 如果没有schedule则设置为1而不是递增
				if nc.Spec.Schedule == "" {
					ncCopy.Status.FailureCount = 1
				} else {
					ncCopy.Status.FailureCount++
				}
				ncCopy.Status.LastResult = jobOutput

				// 记录详细数据
				logger.Info("Nc test completed failed",
					"host", ncOutput.Host,
					"port", ncOutput.Port)

				// 标记测试已完成
				utils.SetCondition(&ncCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "TestCompleted",
					Message:            "Nc test completed failed",
				})

				// 设置TestReportName，使TestReport控制器能够自动创建报告
				// 如果TestReportName为空，则设置为默认格式
				if ncCopy.Status.TestReportName == "" {
					ncCopy.Status.TestReportName = utils.GenerateTestReportName("Nc", nc.Name)
				}

				if ncCopy.Status.QueryCount == 0 {
					ncCopy.Status.QueryCount = 1
				}
			}

			if updateErr := r.Status().Update(ctx, ncCopy); updateErr != nil {
				logger.Info(updateErr.Error(), "failed to update nc status", "nc", nc.Name, "namespace", nc.Namespace)
				return ctrl.Result{RequeueAfter: time.Second * 30}, nil
			}
		} else {
			logger.Info("Job is still running", "nc", req.Name, "jobName", jobName, "namespace", req.Namespace,
				"succeeded", job.Status.Succeeded, "failed", job.Status.Failed, "backoffLimit", *job.Spec.BackoffLimit)
			return ctrl.Result{}, nil
		}
	}
	
	// 只有在明确设置了Schedule字段时才安排下一次执行
	if nc.Spec.Schedule != "" {
		interval := 60 // 默认60秒
		if nc.Spec.Schedule != "" {
			if intervalValue, err := strconv.Atoi(nc.Spec.Schedule); err == nil && intervalValue > 0 {
				interval = intervalValue
			}
		}
		logger.Info("Scheduling next Nc test", "interval", interval)
		return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, nil
	}

	// 默认情况下，不再调度下一次执行
	logger.Info("Nc测试已完成，不再调度下一次执行",
		"nc", nc.Name,
		"namespace", nc.Namespace)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NcReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testtoolsv1.Nc{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

// cleanupResources 清理与Nc资源关联的所有资源
func (r *NcReconciler) cleanupResources(ctx context.Context, nc *testtoolsv1.Nc) error {
	logger := log.FromContext(ctx)
	logger.Info("清理资源", "ncName", nc.Name, "说明", "使用OwnerReference自动级联删除资源")
	// 不需要手动删除资源，因为所有关联资源都通过OwnerReference设置了级联删除
	return nil
}
