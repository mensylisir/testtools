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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/xiaoming/testtools/pkg/utils"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/controller/controllerutil"
)

// FioReconciler reconciles a Fio object
type FioReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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
			logger.Info("Fio资源不存在，可能已被删除")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "获取Fio资源失败")
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

	// 检查是否已存在任务
	var jobList batchv1.JobList
	if err := r.List(ctx, &jobList, client.InNamespace(fio.Namespace),
		client.MatchingLabels{
			"app.kubernetes.io/created-by": "testtools-controller",
			"app.kubernetes.io/managed-by": "fio-controller",
			"app.kubernetes.io/instance":   fio.Name,
		}); err != nil {
		logger.Error(err, "无法列出关联的Job")
		return ctrl.Result{}, err
	}

	// 如果已存在Job，则不需要再创建新Job
	if len(jobList.Items) > 0 {
		logger.Info("已存在关联的Job，检查状态", "jobCount", len(jobList.Items))
		return r.handleExistingJobs(ctx, &fio, jobList.Items)
	}

	// 资源未被调度，需要创建Job
	if fio.Status.Phase == "" || fio.Status.Phase == testtoolsv1.PhaseCreated {
		return r.scheduleFio(ctx, &fio)
	}

	// 根据当前状态处理
	switch fio.Status.Phase {
	case testtoolsv1.PhaseRunning:
		return r.checkFioStatus(ctx, &fio)
	case testtoolsv1.PhaseSucceeded, testtoolsv1.PhaseFailed:
		// 已完成的测试，无需处理
		return ctrl.Result{}, nil
	default:
		logger.Info("未知的Fio阶段", "phase", fio.Status.Phase)
		return ctrl.Result{}, nil
	}
}

// executeFio executes FIO tests and returns the results
func (r *FioReconciler) executeFio(ctx context.Context, fio *testtoolsv1.Fio) (testtoolsv1.FioStats, error) {
	logger := log.FromContext(ctx)
	logger.Info("开始执行Fio测试", "fio", fio.Name, "namespace", fio.Namespace)

	// 创建Job执行Fio测试
	job, err := utils.PrepareFioJob(ctx, fio, r.Client, r.Scheme)
	if err != nil {
		logger.Error(err, "创建Fio Job失败")
		return testtoolsv1.FioStats{}, err
	}

	// 等待Job完成
	err = utils.WaitForJob(ctx, r.Client, fio.Namespace, job.Name, 10*time.Minute)
	if err != nil {
		logger.Error(err, "等待Fio Job完成失败")
		return testtoolsv1.FioStats{}, err
	}

	// 获取Job输出
	output, err := utils.GetJobResults(ctx, r.Client, fio.Namespace, job.Name)
	if err != nil {
		logger.Error(err, "获取Fio Job结果失败")
		return testtoolsv1.FioStats{}, err
	}

	// 解析Fio输出
	stats, err := utils.ParseFioOutput(output)
	if err != nil {
		logger.Error(err, "解析Fio输出失败")
		return testtoolsv1.FioStats{}, err
	}

	// 更新Fio状态，包括设置TestReportName
	err = r.updateFioStatus(ctx, types.NamespacedName{Name: fio.Name, Namespace: fio.Namespace}, func(fioObj *testtoolsv1.Fio) {
		fioObj.Status.Status = "Succeeded"
		fioObj.Status.LastExecutionTime = &metav1.Time{Time: time.Now()}

		// 设置性能统计数据
		fioObj.Status.Stats = stats

		// 限制输出长度，避免太大
		if len(output) > 5000 {
			output = output[:5000] + "...(输出被截断)"
		}
		fioObj.Status.LastResult = output

		// 设置TestReportName，使TestReport控制器能够自动创建报告
		if fioObj.Status.TestReportName == "" {
			fioObj.Status.TestReportName = fmt.Sprintf("fio-%s-report", fio.Name)
		}

		// 更新条件
		utils.SetCondition(&fioObj.Status.Conditions, metav1.Condition{
			Type:               "Completed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.Time{Time: time.Now()},
			Reason:             "TestCompleted",
			Message:            "Fio测试已成功完成",
		})
	})

	if err != nil {
		logger.Error(err, "更新Fio状态失败")
		return stats, err
	}

	return stats, nil
}

// updateFioStatus 使用乐观锁更新Fio状态
func (r *FioReconciler) updateFioStatus(ctx context.Context, name types.NamespacedName, updateFn func(*testtoolsv1.Fio)) error {
	logger := log.FromContext(ctx)
	retries := 5
	backoff := 200 * time.Millisecond

	var lastErr error
	for i := 0; i < retries; i++ {
		// 获取Fio实例
		var fio testtoolsv1.Fio
		if err := r.Get(ctx, name, &fio); err != nil {
			if errors.IsNotFound(err) {
				logger.Info("无法找到要更新的Fio资源", "name", name.Name, "namespace", name.Namespace)
				return err
			}
			lastErr = err
			continue
		}

		// 应用更新
		updateFn(&fio)

		// 确保TestReportName被设置
		if fio.Status.Status == "Succeeded" && fio.Status.TestReportName == "" {
			fio.Status.TestReportName = fmt.Sprintf("fio-%s-report", fio.Name)
		}

		// 更新状态
		if err := r.Status().Update(ctx, &fio); err != nil {
			if errors.IsConflict(err) {
				// 冲突错误，等待后重试
				lastErr = err
				time.Sleep(backoff)
				backoff *= 2 // 指数退避
				continue
			}
			return err
		}

		// 更新成功
		return nil
	}

	return fmt.Errorf("failed to update Fio status after %d retries: %v", retries, lastErr)
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

	// 查找并清理关联的Job
	var jobList batchv1.JobList
	if err := r.List(ctx, &jobList, client.InNamespace(fio.Namespace),
		client.MatchingLabels{
			"app.kubernetes.io/created-by": "testtools-controller",
			"app.kubernetes.io/managed-by": "fio-controller",
			"app.kubernetes.io/instance":   fio.Name,
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

	// 不再需要删除TestReport，因为它们现在有OwnerReference，会随着Fio资源自动删除
	logger.Info("资源清理完成", "fioName", fio.Name)
	return nil
}

// scheduleFio 创建Job执行Fio测试
func (r *FioReconciler) scheduleFio(ctx context.Context, fio *testtoolsv1.Fio) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("开始调度Fio测试", "fioName", fio.Name)

	// 准备Fio Job
	job, err := utils.PrepareFioJob(fio, r.FioImage)
	if err != nil {
		logger.Error(err, "准备Fio Job失败")
		r.Recorder.Event(fio, corev1.EventTypeWarning, "JobCreationFailed", "创建Fio Job失败: "+err.Error())

		// 更新状态为失败
		patch := client.MergeFrom(fio.DeepCopy())
		fio.Status.Phase = testtoolsv1.PhaseFailed
		fio.Status.Message = "创建Fio Job失败: " + err.Error()
		if err := r.Status().Patch(ctx, fio, patch); err != nil {
			logger.Error(err, "更新Fio状态失败")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	// 设置Owner Reference
	if err := ctrl.SetControllerReference(fio, job, r.Scheme); err != nil {
		logger.Error(err, "设置Job Controller Reference失败")
		r.Recorder.Event(fio, corev1.EventTypeWarning, "OwnerReferenceFailed", "设置Job所有者引用失败: "+err.Error())
		return ctrl.Result{}, err
	}

	// 创建Job
	logger.Info("创建Fio Job", "jobName", job.Name)
	if err := r.Create(ctx, job); err != nil {
		logger.Error(err, "创建Job失败")
		r.Recorder.Event(fio, corev1.EventTypeWarning, "JobCreationFailed", "创建Fio Job失败: "+err.Error())

		// 更新状态为失败
		patch := client.MergeFrom(fio.DeepCopy())
		fio.Status.Phase = testtoolsv1.PhaseFailed
		fio.Status.Message = "创建Fio Job失败: " + err.Error()
		if err := r.Status().Patch(ctx, fio, patch); err != nil {
			logger.Error(err, "更新Fio状态失败")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	// 更新状态为运行中
	r.Recorder.Event(fio, corev1.EventTypeNormal, "JobCreated", "成功创建Fio Job: "+job.Name)
	patch := client.MergeFrom(fio.DeepCopy())
	fio.Status.Phase = testtoolsv1.PhaseRunning
	fio.Status.Message = "Fio测试正在运行"
	fio.Status.JobName = job.Name
	if err := r.Status().Patch(ctx, fio, patch); err != nil {
		logger.Error(err, "更新Fio状态失败")
		return ctrl.Result{}, err
	}

	// 短暂等待后重新检查状态
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}
