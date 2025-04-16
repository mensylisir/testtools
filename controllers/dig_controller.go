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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/xiaoming/testtools/pkg/utils"
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
	logger.V(1).Info("Reconciling Dig", "namespace", req.Namespace, "name", req.Name)

	// 1. 获取最新的 Dig 资源
	var dig testtoolsv1.Dig
	if err := r.Get(ctx, req.NamespacedName, &dig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 检查是否已经执行完成，避免重复执行
	// 检查是否已存在完成状态的条件
	for _, condition := range dig.Status.Conditions {
		if condition.Type == "Completed" && condition.Status == metav1.ConditionTrue {
			logger.Info("Dig测试已经完成，不再重复执行",
				"dig", dig.Name,
				"namespace", dig.Namespace)
			return ctrl.Result{}, nil
		}
	}

	// 添加节流机制，如果上次执行时间太近，直接延迟处理
	// 检查是否有最后执行时间，避免频繁执行
	if dig.Status.LastExecutionTime != nil && dig.Spec.Schedule != "" {
		// 只有在明确设置了Schedule字段时才应用定期执行逻辑
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

	// 确保 TestReportName 字段存在，这样 TestReportController 可以找到关联的 Dig 资源
	// 注意：TestReport 的创建和更新已移至 TestReportController
	if dig.Status.TestReportName == "" {
		digCopy := dig.DeepCopy()
		digCopy.Status.TestReportName = fmt.Sprintf("dig-%s-report", dig.Name)

		if err := r.Status().Update(ctx, digCopy); err != nil {
			logger.Error(err, "Failed to update Dig status with TestReportName")
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}

		// 重新获取最新的 Dig 对象，以确保使用最新状态
		if err := r.Get(ctx, req.NamespacedName, &dig); err != nil {
			logger.Error(err, "Failed to refresh Dig resource after updating TestReportName")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	// 验证必填字段
	if dig.Spec.Domain == "" {
		logger.Error(nil, "Domain is required", "dig", dig.Name, "namespace", dig.Namespace)
		digCopy := dig.DeepCopy()
		now := metav1.NewTime(time.Now())
		digCopy.Status.LastExecutionTime = &now
		digCopy.Status.QueryCount++
		digCopy.Status.FailureCount++
		digCopy.Status.Status = "Failed"
		digCopy.Status.LastResult = "Domain is required"

		// 添加失败条件
		utils.SetCondition(&digCopy.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "ValidationFailed",
			Message:            "Dig域名参数验证失败: 域名为必填项",
		})

		// 更新状态
		if err := r.Status().Update(ctx, digCopy); err != nil {
			logger.Error(err, "Failed to update Dig status after validation failure")
		}

		return ctrl.Result{}, fmt.Errorf("domain is required for dig")
	}

	// 更新状态，记录开始时间
	digCopy := dig.DeepCopy()
	now := metav1.NewTime(time.Now())
	digCopy.Status.LastExecutionTime = &now
	digCopy.Status.QueryCount++

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
		digCopy.Status.SuccessCount++
		digCopy.Status.LastResult = jobOutput
		digCopy.Status.AverageResponseTime = digOutput.QueryTime

		// 注意：移除了之前不匹配的字段赋值
		// 如果需要保存DNS服务器和状态等数据，应当增加相应的API字段定义

		// 如果有IP地址列表，可以将其保存在LastResult或另一个自定义字段中
		// 这里不再尝试设置不存在的字段

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
	if updateErr := r.Status().Update(ctx, digCopy); updateErr != nil {
		logger.Error(updateErr, "Failed to update Dig status")
		return ctrl.Result{RequeueAfter: time.Second * 5}, updateErr
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
