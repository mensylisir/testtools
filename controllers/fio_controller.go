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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/xiaoming/testtools/pkg/utils"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

	logger.Info("Waiting for Fio job to complete", "jobName", jobName)
	err = utils.WaitForJob(ctx, r.Client, fio.Namespace, jobName, 5*time.Minute)

	if err != nil {
		logger.Error(err, "Failed while waiting for Fio job to complete")
		fioCopy.Status.Status = "Failed"
		fioCopy.Status.FailureCount++
		fioCopy.Status.LastResult = fmt.Sprintf("Error waiting for Fio job: %v", err)

		utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "JobExecutionFailed",
			Message:            fmt.Sprintf("Error waiting for Fio job: %v", err),
		})

		if updateErr := r.Status().Update(ctx, fioCopy); updateErr != nil {
			logger.Error(updateErr, "Failed to update Fio status after job execution failure")
		}
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

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
		}
		fioCopy.Status.Status = "Succeeded"
		if fioCopy.Spec.Schedule == "" {
			fioCopy.Status.SuccessCount = 1
		} else {
			fioCopy.Status.SuccessCount++
		}
		fioCopy.Status.LastResult = jobOutput
		fioCopy.Status.Stats = fioOutput

		if fioCopy.Status.TestReportName == "" {
			fioCopy.Status.TestReportName = utils.GenerateTestReportName("Fio", fio.Name)
		}
		if fioCopy.Status.QueryCount == 0 {
			fioCopy.Status.QueryCount = 1
		}
		logger.Info("Fio test successfully", "result", jobOutput)
		utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
			Type:               "Completed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "TestCompleted",
			Message:            "Fio测试已成功完成",
		})
	}

	// 更新状态
	maxUpdateRetries := 5
	updateRetryDelay := 200 * time.Millisecond

	for i := 0; i < maxUpdateRetries; i++ {
		// 如果不是首次尝试，重新获取对象和重新应用更改
		if i > 0 {
			if err := r.Get(ctx, req.NamespacedName, &fio); err != nil {
				logger.Error(err, "Failed to re-fetch Fio for status update retry", "attempt", i+1)
				if errors.IsNotFound(err) {
					return ctrl.Result{}, nil
				}
				return ctrl.Result{}, err
			}

			fioCopy = fio.DeepCopy()
			fioCopy.Status.LastExecutionTime = &now
			fioCopy.Status.ExecutedCommand = fmt.Sprintf("fio %s", strings.Join(fioArgs, " "))

			if fio.Status.QueryCount == 0 {
				fioCopy.Status.QueryCount = 1
			}

			if jobOutput != "" {
				fioOutput, err := utils.ParseFioOutput(jobOutput)
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
				}
				fioCopy.Status.Status = "Succeeded"
				if fioCopy.Spec.Schedule == "" {
					fioCopy.Status.SuccessCount = 1
				} else {
					fioCopy.Status.SuccessCount++
				}
				fioCopy.Status.LastResult = jobOutput
				fioCopy.Status.Stats = fioOutput

				if fioCopy.Status.TestReportName == "" {
					fioCopy.Status.TestReportName = utils.GenerateTestReportName("Fio", fio.Name)
				}
				if fioCopy.Status.QueryCount == 0 {
					fioCopy.Status.QueryCount = 1
				}
				logger.Info("Fio test successfully", "result", jobOutput)
				utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
					Type:               "Completed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "TestCompleted",
					Message:            "Fio测试已成功完成",
				})
			} else {
				fioCopy.Status.Status = "Failed"
				fioCopy.Status.FailureCount++

				if fioCopy.Status.TestReportName == "" {
					fioCopy.Status.TestReportName = utils.GenerateTestReportName("Fio", fio.Name)
				}
				utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "ResultRetrievalFailed",
					Message:            "无法获取Fio测试结果",
				})
			}

			logger.Info("Retrying Fio status update", "attempt", i+1, "maxRetries", maxUpdateRetries)
		}

		if updateErr := r.Status().Update(ctx, fioCopy); updateErr != nil {
			if apierrors.IsConflict(updateErr) {
				logger.Info("Conflict when updating Fio status, will retry",
					"attempt", i+1,
					"maxRetries", maxUpdateRetries)
				if i < maxUpdateRetries-1 {
					time.Sleep(updateRetryDelay)
					updateRetryDelay *= 2 // 指数退避
					continue
				}
			}
			logger.Error(updateErr, "Failed to update Fio status")
			return ctrl.Result{RequeueAfter: time.Second * 5}, updateErr
		} else {
			logger.Info("Successfully updated Fio status")
			break
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

//// executeFio executes FIO tests and returns the results
//func (r *FioReconciler) executeFio(ctx context.Context, fio *testtoolsv1.Fio) (testtoolsv1.FioStats, error) {
//	logger := log.FromContext(ctx)
//	logger.Info("开始执行Fio测试", "fio", fio.Name, "namespace", fio.Namespace)
//
//	// 创建Job执行Fio测试
//	job, err := utils.PrepareFioJob(ctx, fio, r.Client, r.Scheme)
//	if err != nil {
//		logger.Error(err, "创建Fio Job失败")
//		return testtoolsv1.FioStats{}, err
//	}
//
//	// 等待Job完成
//	err = utils.WaitForJob(ctx, r.Client, fio.Namespace, job.Name, 10*time.Minute)
//	if err != nil {
//		logger.Error(err, "等待Fio Job完成失败")
//		return testtoolsv1.FioStats{}, err
//	}
//
//	// 获取Job输出
//	output, err := utils.GetJobResults(ctx, r.Client, fio.Namespace, job.Name)
//	if err != nil {
//		logger.Error(err, "获取Fio Job结果失败")
//		return testtoolsv1.FioStats{}, err
//	}
//
//	// 解析Fio输出
//	stats, err := utils.ParseFioOutput(output)
//	if err != nil {
//		logger.Error(err, "解析Fio输出失败")
//		return testtoolsv1.FioStats{}, err
//	}
//
//	// 更新Fio状态，包括设置TestReportName
//	err = r.updateFioStatus(ctx, types.NamespacedName{Name: fio.Name, Namespace: fio.Namespace}, func(fioObj *testtoolsv1.Fio) {
//		fioObj.Status.Status = "Succeeded"
//		fioObj.Status.LastExecutionTime = &metav1.Time{Time: time.Now()}
//
//		// 设置性能统计数据
//		fioObj.Status.Stats = stats
//
//		// 限制输出长度，避免太大
//		if len(output) > 5000 {
//			output = output[:5000] + "...(输出被截断)"
//		}
//		fioObj.Status.LastResult = output
//
//		// 设置TestReportName，使TestReport控制器能够自动创建报告
//		if fioObj.Status.TestReportName == "" {
//			fioObj.Status.TestReportName = fmt.Sprintf("fio-%s-report", fio.Name)
//		}
//
//		// 更新条件
//		utils.SetCondition(&fioObj.Status.Conditions, metav1.Condition{
//			Type:               "Completed",
//			Status:             metav1.ConditionTrue,
//			LastTransitionTime: metav1.Time{Time: time.Now()},
//			Reason:             "TestCompleted",
//			Message:            "Fio测试已成功完成",
//		})
//	})
//
//	if err != nil {
//		logger.Error(err, "更新Fio状态失败")
//		return stats, err
//	}
//
//	return stats, nil
//}

// updateFioStatus 使用乐观锁更新Fio状态
//func (r *FioReconciler) updateFioStatus(ctx context.Context, name types.NamespacedName, updateFn func(*testtoolsv1.Fio)) error {
//	logger := log.FromContext(ctx)
//	retries := 5
//	backoff := 200 * time.Millisecond
//
//	var lastErr error
//	for i := 0; i < retries; i++ {
//		// 获取Fio实例
//		var fio testtoolsv1.Fio
//		if err := r.Get(ctx, name, &fio); err != nil {
//			if errors.IsNotFound(err) {
//				logger.Info("无法找到要更新的Fio资源", "name", name.Name, "namespace", name.Namespace)
//				return err
//			}
//			lastErr = err
//			continue
//		}
//
//		// 应用更新
//		updateFn(&fio)
//
//		// 确保TestReportName被设置
//		if fio.Status.Status == "Succeeded" && fio.Status.TestReportName == "" {
//			fio.Status.TestReportName = fmt.Sprintf("fio-%s-report", fio.Name)
//		}
//
//		// 更新状态
//		if err := r.Status().Update(ctx, &fio); err != nil {
//			if errors.IsConflict(err) {
//				// 冲突错误，等待后重试
//				lastErr = err
//				time.Sleep(backoff)
//				backoff *= 2 // 指数退避
//				continue
//			}
//			return err
//		}
//
//		// 更新成功
//		return nil
//	}
//
//	return fmt.Errorf("failed to update Fio status after %d retries: %v", retries, lastErr)
//}

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

// scheduleFio 创建Job执行Fio测试
//func (r *FioReconciler) scheduleFio(ctx context.Context, fio *testtoolsv1.Fio) (ctrl.Result, error) {
//	logger := log.FromContext(ctx)
//	logger.Info("开始调度Fio测试", "fioName", fio.Name)
//
//	// 准备Fio Job
//	job, err := utils.PrepareFioJob(ctx, fio, r.Client, r.Scheme)
//	if err != nil {
//		logger.Error(err, "准备Fio Job失败")
//		if r.Recorder != nil {
//			r.Recorder.Event(fio, corev1.EventTypeWarning, "JobCreationFailed", "创建Fio Job失败: "+err.Error())
//		}
//
//		// 更新状态为失败
//		patch := client.MergeFrom(fio.DeepCopy())
//		fio.Status.Status = "Failed"
//		fio.Status.LastResult = "创建Fio Job失败: " + err.Error()
//
//		// 即使失败也设置TestReportName，确保能生成测试报告
//		if fio.Status.TestReportName == "" {
//			fio.Status.TestReportName = utils.GenerateTestReportName("Fio", fio.Name)
//		}
//
//		if err := r.Status().Patch(ctx, fio, patch); err != nil {
//			logger.Error(err, "更新Fio状态失败")
//			return ctrl.Result{}, err
//		}
//		return ctrl.Result{}, err
//	}
//
//	// 注意：不再需要SetControllerReference，因为已经在utils.PrepareFioJob中设置了OwnerReferences
//	// 避免重复设置导致控制器UID冲突
//
//	// 创建Job
//	logger.Info("创建Fio Job", "jobName", job.Name)
//	if err := r.Create(ctx, job); err != nil {
//		// 如果Job已存在错误，忽略它继续处理
//		if errors.IsAlreadyExists(err) {
//			logger.Info("Job已存在，跳过创建", "jobName", job.Name)
//		} else {
//			logger.Error(err, "创建Job失败")
//			if r.Recorder != nil {
//				r.Recorder.Event(fio, corev1.EventTypeWarning, "JobCreationFailed", "创建Fio Job失败: "+err.Error())
//			}
//
//			// 更新状态为失败
//			patch := client.MergeFrom(fio.DeepCopy())
//			fio.Status.Status = "Failed"
//			fio.Status.LastResult = "创建Fio Job失败: " + err.Error()
//
//			// 即使失败也设置TestReportName
//			if fio.Status.TestReportName == "" {
//				fio.Status.TestReportName = utils.GenerateTestReportName("Fio", fio.Name)
//			}
//
//			if err := r.Status().Patch(ctx, fio, patch); err != nil {
//				logger.Error(err, "更新Fio状态失败")
//				return ctrl.Result{}, err
//			}
//			return ctrl.Result{}, err
//		}
//	}
//
//	// 更新状态为运行中
//	if r.Recorder != nil {
//		r.Recorder.Event(fio, corev1.EventTypeNormal, "JobCreated", "成功创建Fio Job: "+job.Name)
//	}
//	patch := client.MergeFrom(fio.DeepCopy())
//	fio.Status.Status = "Running"
//	fio.Status.LastResult = "Fio测试正在运行"
//
//	// 设置TestReportName，确保能生成测试报告
//	if fio.Status.TestReportName == "" {
//		fio.Status.TestReportName = utils.GenerateTestReportName("Fio", fio.Name)
//	}
//
//	if err := r.Status().Patch(ctx, fio, patch); err != nil {
//		logger.Error(err, "更新Fio状态失败")
//		return ctrl.Result{}, err
//	}
//
//	// 短暂等待后重新检查状态
//	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
//}

// handleExistingJobs 处理已存在的Fio Job
//func (r *FioReconciler) handleExistingJobs(ctx context.Context, fio *testtoolsv1.Fio, jobs []batchv1.Job) (ctrl.Result, error) {
//	logger := log.FromContext(ctx)
//
//	// 如果没有Job，创建新的
//	if len(jobs) == 0 {
//		logger.Info("没有找到关联的Job，创建新Job")
//		return r.scheduleFio(ctx, fio)
//	}
//
//	// 检查是否有正在运行的作业
//	for _, job := range jobs {
//		// 如果作业正在运行
//		if job.Status.Active > 0 {
//			logger.Info("Fio Job正在运行中", "job", job.Name)
//
//			// 更新Fio状态为Running
//			if fio.Status.Status != "Running" {
//				fioCopy := fio.DeepCopy()
//				fioCopy.Status.Status = "Running"
//
//				// 确保设置TestReportName
//				if fioCopy.Status.TestReportName == "" {
//					fioCopy.Status.TestReportName = utils.GenerateTestReportName("Fio", fio.Name)
//				}
//
//				if err := r.Status().Update(ctx, fioCopy); err != nil {
//					logger.Error(err, "更新Fio状态失败")
//					return ctrl.Result{}, err
//				}
//			}
//
//			// 延迟重新检查
//			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
//		}
//
//		// 如果作业已完成
//		if job.Status.Succeeded > 0 {
//			logger.Info("Fio Job已成功完成", "job", job.Name)
//
//			// 获取作业结果
//			output, err := utils.GetJobResults(ctx, r.Client, fio.Namespace, job.Name)
//			if err != nil {
//				logger.Error(err, "获取Job结果失败")
//				// 即使错误，也继续更新Fio状态
//				output = fmt.Sprintf("获取任务结果失败: %v", err)
//			}
//
//			// 解析结果
//			stats := testtoolsv1.FioStats{}
//			if output != "" {
//				parsedStats, err := utils.ParseFioOutput(output)
//				if err != nil {
//					logger.Error(err, "解析Fio输出失败")
//					// 继续执行，使用空的stats
//				} else {
//					stats = parsedStats
//				}
//			}
//
//			// 更新Fio状态
//			fioCopy := fio.DeepCopy()
//			fioCopy.Status.Status = "Succeeded"
//			fioCopy.Status.LastResult = output
//			fioCopy.Status.Stats = stats
//			fioCopy.Status.SuccessCount++
//			if fioCopy.Status.QueryCount == 0 {
//				fioCopy.Status.QueryCount = 1
//			}
//
//			// 确保TestReportName已设置
//			if fioCopy.Status.TestReportName == "" {
//				fioCopy.Status.TestReportName = utils.GenerateTestReportName("Fio", fio.Name)
//			}
//
//			if err := r.Status().Update(ctx, fioCopy); err != nil {
//				logger.Error(err, "更新Fio状态失败")
//				return ctrl.Result{}, err
//			}
//
//			return ctrl.Result{}, nil
//		}
//
//		// 如果作业失败
//		if job.Status.Failed > 0 {
//			logger.Info("Fio Job执行失败", "job", job.Name)
//
//			// 更新Fio状态
//			fioCopy := fio.DeepCopy()
//			fioCopy.Status.Status = "Failed"
//			fioCopy.Status.FailureCount++
//			if fioCopy.Status.QueryCount == 0 {
//				fioCopy.Status.QueryCount = 1
//			}
//
//			// 确保TestReportName已设置，即使失败也设置，让TestReport控制器能处理
//			if fioCopy.Status.TestReportName == "" {
//				fioCopy.Status.TestReportName = utils.GenerateTestReportName("Fio", fio.Name)
//			}
//
//			if err := r.Status().Update(ctx, fioCopy); err != nil {
//				logger.Error(err, "更新Fio状态失败")
//				return ctrl.Result{}, err
//			}
//
//			return ctrl.Result{}, nil
//		}
//	}
//
//	// 如果所有作业都不在运行中，且没有成功或失败状态，则创建新作业
//	logger.Info("没有找到活跃的Job，重新创建")
//	return r.scheduleFio(ctx, fio)
//}

// checkFioStatus 检查Fio任务状态
//func (r *FioReconciler) checkFioStatus(ctx context.Context, fio *testtoolsv1.Fio) (ctrl.Result, error) {
//	logger := log.FromContext(ctx)
//
//	// 查找相关联的Job
//	var jobList batchv1.JobList
//	if err := r.List(ctx, &jobList, client.InNamespace(fio.Namespace),
//		client.MatchingLabels{
//			"app.kubernetes.io/created-by": "testtools-controller",
//			"app.kubernetes.io/managed-by": "fio-controller",
//			"app.kubernetes.io/instance":   fio.Name,
//		}); err != nil {
//		logger.Error(err, "无法列出关联的Job")
//		return ctrl.Result{}, err
//	}
//
//	// 如果没有找到Job，可能已经被删除
//	if len(jobList.Items) == 0 {
//		logger.Info("没有找到关联的Job，重新创建")
//		return r.scheduleFio(ctx, fio)
//	}
//
//	// 复用现有Job处理逻辑
//	return r.handleExistingJobs(ctx, fio, jobList.Items)
//}
