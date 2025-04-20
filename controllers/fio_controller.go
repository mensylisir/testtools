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

	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/xiaoming/testtools/pkg/utils"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
)

// FioFinalizerName 是用于Fio资源finalizer的名称
const FioFinalizerName = "fio.testtools.xiaoming.com/finalizer"

// FioReconciler reconciles a Fio object
type FioReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	FioImage string
}

// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=fios,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=fios/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=fios/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods/log,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *FioReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("开始调和Fio资源", "fioName", req.NamespacedName)

	// 获取Fio资源
	var fio testtoolsv1.Fio
	if err := r.Get(ctx, req.NamespacedName, &fio); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Fio资源不存在，可能已被删除", "namespacedName", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "获取Fio资源失败", "namespacedName", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// 处理资源删除
	if !fio.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&fio, FioFinalizerName) {
			logger.Info("处理Fio资源删除", "fioName", fio.Name)
			if err := r.cleanupResources(ctx, &fio); err != nil {
				logger.Error(err, "清理资源失败")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&fio, FioFinalizerName)
			if err := r.Update(ctx, &fio); err != nil {
				logger.Error(err, "移除finalizer失败")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// 确保finalizer存在
	if !controllerutil.ContainsFinalizer(&fio, FioFinalizerName) {
		controllerutil.AddFinalizer(&fio, FioFinalizerName)
		if err := r.Update(ctx, &fio); err != nil {
			logger.Error(err, "添加finalizer失败")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// 检查是否需要执行测试
	shouldExecute := false

	if fio.Spec.Schedule != "" {
		if fio.Status.LastExecutionTime != nil {
			intervalSeconds := 60
			if scheduleValue, err := strconv.Atoi(fio.Spec.Schedule); err == nil && scheduleValue > 0 {
				intervalSeconds = scheduleValue
			}
			nextExecutionTime := fio.Status.LastExecutionTime.Time.Add(time.Duration(intervalSeconds) * time.Second)
			shouldExecute = time.Now().After(nextExecutionTime)

			if !shouldExecute {
				logger.Info("还未到调度时间，跳过执行",
					"fio", fio.Name,
					"上次执行", fio.Status.LastExecutionTime.Time,
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
		shouldExecute = fio.Status.LastExecutionTime == nil
	}

	if !shouldExecute {
		logger.Info("无需执行Fio测试",
			"fio", fio.Name,
			"lastExecutionTime", fio.Status.LastExecutionTime,
			"schedule", fio.Spec.Schedule)
		return ctrl.Result{}, nil
	}

	fioCopy := fio.DeepCopy()
	now := metav1.NewTime(time.Now())
	fioCopy.Status.LastExecutionTime = &now
	if fio.Status.QueryCount == 0 {
		// 如果是首次执行，设置查询计数为1
		fioCopy.Status.QueryCount = 1
	}

	jobName, err := utils.PrepareFioJob(ctx, r.Client, &fio)
	if err != nil {
		logger.Error(err, "Failed to prepare Fio job")
		fioCopy.Status.Status = "Failed"
		fioCopy.Status.FailureCount++
		fioCopy.Status.LastResult = fmt.Sprintf("Error preparing Fio job: %v", err)

		utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "JobPreparationFailed",
			Message:            fmt.Sprintf("Fio测试准备作业失败: %v", err),
		})

		if updateErr := r.Status().Update(ctx, fioCopy); updateErr != nil {
			logger.Error(updateErr, "Failed to update Fio status after job preparation failure")
		}

		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	fioArgs := utils.BuildFioArgs(&fio)
	fioCopy.Status.ExecutedCommand = fmt.Sprintf("fio %s", strings.Join(fioArgs, " "))
	logger.Info("Setting executed command", "command", fioCopy.Status.ExecutedCommand)

	var job batchv1.Job
	if err = r.Client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: jobName}, &job); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Fio job not found, creating...", "fio", req.Name, "jobName", jobName, "namespace", req.Namespace)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Job", "fio", req.Name, "jobName", jobName)
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	} else {
		if job.Status.Succeeded > 0 {
			logger.Info("Job completed successfully", "fio", req.Name, "jobName", jobName, "namespace", req.Namespace)
			jobOutput, err := utils.GetJobResults(ctx, r.Client, fio.Namespace, jobName)
			if err != nil {
				logger.Error(err, "Failed to get Fio job results")
				fioCopy.Status.Status = "Failed"
				fioCopy.Status.FailureCount++
				fioCopy.Status.LastResult = fmt.Sprintf("Error getting Fio job results: %v", err)

				if fioCopy.Status.TestReportName == "" {
					fioCopy.Status.TestReportName = utils.GenerateTestReportName("Fio", fio.Name)
				}

				utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "ResultRetrievalFailed",
					Message:            fmt.Sprintf("Error getting Fio job results: %v", err),
				})
			} else {
				fioOutput, err := utils.ParseFioOutput(jobOutput)
				if err != nil {
					logger.Error(err, "Failed to parse Fio job results")
					fioCopy.Status.Status = "Failed"
					fioCopy.Status.FailureCount++
					fioCopy.Status.LastResult = fmt.Sprintf("Error parsing Fio job results: %v", err)

					if fioCopy.Status.TestReportName == "" {
						fioCopy.Status.TestReportName = utils.GenerateTestReportName("Fio", fio.Name)
					}

					utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
						Type:               "Failed",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "ResultParsingFailed",
						Message:            fmt.Sprintf("Error parsing Fio job results: %v", err),
					})
				} else {
					fioCopy.Status.Status = "Succeeded"
					if fioCopy.Spec.Schedule == "" {
						fioCopy.Status.SuccessCount = 1
					} else {
						fioCopy.Status.SuccessCount++
					}
					fioCopy.Status.LastResult = jobOutput
					fioCopy.Status.Stats = fioOutput

					logger.Info("Fio test successfully", "result", jobOutput)
					utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
						Type:               "Completed",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "TestCompleted",
						Message:            "Fio测试已成功完成",
					})
				}

				if fioCopy.Status.TestReportName == "" {
					fioCopy.Status.TestReportName = utils.GenerateTestReportName("Fio", fio.Name)
				}
				if fioCopy.Status.QueryCount == 0 {
					fioCopy.Status.QueryCount = 1
				}
			}
			if updateErr := r.Status().Update(ctx, fioCopy); updateErr != nil {
				logger.Info(updateErr.Error(), "failed to update fio status", "fio", fio.Name, "namespace", fio.Namespace)
				return ctrl.Result{RequeueAfter: time.Second * 30}, nil
			}
		} else if job.Status.Failed > *job.Spec.BackoffLimit {
			logger.Error(nil, "Job failed", "fio", req.Name, "jobName", jobName, "namespace", req.Namespace,
				"failed", job.Status.Failed, "backoffLimit", *job.Spec.BackoffLimit)
			jobOutput, err := utils.GetJobResults(ctx, r.Client, fio.Namespace, jobName)
			if err != nil {
				logger.Error(err, "Failed to get Fio job results")
				fioCopy.Status.Status = "Failed"
				fioCopy.Status.FailureCount++
				fioCopy.Status.LastResult = fmt.Sprintf("Error getting Fio job results: %v", err)

				if fioCopy.Status.TestReportName == "" {
					fioCopy.Status.TestReportName = utils.GenerateTestReportName("Fio", fio.Name)
				}

				utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "ResultRetrievalFailed",
					Message:            fmt.Sprintf("Error getting Fio job results: %v", err),
				})
			} else {
				fioOutput, err := utils.ParseFioOutput(jobOutput)
				if err != nil {
					logger.Error(err, "Failed to parse Fio job results")
					fioCopy.Status.Status = "Failed"
					fioCopy.Status.FailureCount++
					fioCopy.Status.LastResult = fmt.Sprintf("Error parsing Fio job results: %v", err)

					if fioCopy.Status.TestReportName == "" {
						fioCopy.Status.TestReportName = utils.GenerateTestReportName("Fio", fio.Name)
					}

					utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
						Type:               "Failed",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "ResultParsingFailed",
						Message:            fmt.Sprintf("Error parsing Fio job results: %v", err),
					})
				} else {
					fioCopy.Status.Status = "Failed"
					if fioCopy.Spec.Schedule == "" {
						fioCopy.Status.FailureCount = 1
					} else {
						fioCopy.Status.FailureCount++
					}
					fioCopy.Status.LastResult = jobOutput
					fioCopy.Status.Stats = fioOutput

					logger.Info("Fio test failed", "result", jobOutput)
					utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
						Type:               "Failed",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "TestCompleted",
						Message:            "Fio test completed failed",
					})
				}

				if fioCopy.Status.TestReportName == "" {
					fioCopy.Status.TestReportName = utils.GenerateTestReportName("Fio", fio.Name)
				}
				if fioCopy.Status.QueryCount == 0 {
					fioCopy.Status.QueryCount = 1
				}
			}
			if updateErr := r.Status().Update(ctx, fioCopy); updateErr != nil {
				logger.Info(updateErr.Error(), "failed to update fio status", "fio", fio.Name, "namespace", fio.Namespace)
				return ctrl.Result{RequeueAfter: time.Second * 30}, nil
			}
		} else {
			logger.Info("Job is still running", "fio", req.Name, "jobName", jobName, "namespace", req.Namespace,
				"succeeded", job.Status.Succeeded, "failed", job.Status.Failed, "backoffLimit", *job.Spec.BackoffLimit)
			return ctrl.Result{}, nil
		}
	}
	if fio.Spec.Schedule != "" {
		interval := 60 // 默认60秒
		if intervalValue, err := strconv.Atoi(fio.Spec.Schedule); err == nil && intervalValue > 0 {
			interval = intervalValue
		}
		logger.Info("Scheduling next Fio test", "interval", interval)
		return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, nil
	}

	logger.Info("Fio测试已完成，不再调度下一次执行",
		"fio", fio.Name,
		"namespace", fio.Namespace)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FioReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testtoolsv1.Fio{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

// cleanupResources 清理与Fio资源关联的所有资源
func (r *FioReconciler) cleanupResources(ctx context.Context, fio *testtoolsv1.Fio) error {
	logger := log.FromContext(ctx)
	logger.Info("清理资源", "fioName", fio.Name, "说明", "使用OwnerReference自动级联删除资源")
	// 不需要手动删除资源，因为所有关联资源都通过OwnerReference设置了级联删除
	return nil
}
