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
	"reflect"
	"strconv"
	"strings"
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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	logger.Info("开始Fio调谐", "namespacedName", req.NamespacedName)

	// 记录执行时间
	start := time.Now()
	defer func() {
		logger.Info("完成Fio调谐", "namespacedName", req.NamespacedName, "duration", time.Since(start))
	}()

	// 获取Fio实例
	var fio testtoolsv1.Fio
	if err := r.Get(ctx, req.NamespacedName, &fio); err != nil {
		if apierrors.IsNotFound(err) {
			// Fio资源已被删除，忽略
			logger.Info("Fio资源已被删除，忽略", "namespacedName", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "获取Fio资源失败", "namespacedName", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// 检查是否需要执行测试
	shouldExecute := false

	// 检查是否设置了调度规则
	if fio.Spec.Schedule != "" {
		// 如果有上次执行时间，检查是否到了下次执行时间
		if fio.Status.LastExecutionTime != nil {
			// 计算下次执行时间
			intervalSeconds := 60 // 默认60秒
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
		// 没有设置调度规则，只有在初始创建或者状态为空时执行一次
		shouldExecute = fio.Status.LastExecutionTime == nil
	}

	if !shouldExecute {
		logger.Info("无需执行Fio测试",
			"fio", fio.Name,
			"lastExecutionTime", fio.Status.LastExecutionTime,
			"schedule", fio.Spec.Schedule)
		return ctrl.Result{}, nil
	}

	// 更新状态，记录开始时间
	fioCopy := fio.DeepCopy()
	now := metav1.NewTime(time.Now())
	fioCopy.Status.LastExecutionTime = &now
	// 在这里设置执行标记，但仅在首次执行时增加QueryCount
	if fio.Status.QueryCount == 0 {
		// 如果是首次执行，设置查询计数为1
		fioCopy.Status.QueryCount = 1
	}

	// 准备并执行FIO测试
	job, err := utils.PrepareFioJob(ctx, &fio, r.Client, r.Scheme)
	if err != nil {
		logger.Error(err, "准备FIO作业失败")
		fioCopy.Status.Status = "Failed"
		fioCopy.Status.FailureCount++
		fioCopy.Status.LastResult = fmt.Sprintf("Error preparing FIO job: %v", err)

		// 添加失败条件
		utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "JobPreparationFailed",
			Message:            fmt.Sprintf("FIO测试准备作业失败: %v", err),
		})

		// 更新状态
		if updateErr := r.Status().Update(ctx, fioCopy); updateErr != nil {
			logger.Error(updateErr, "更新FIO状态失败（作业准备失败后）")
		}

		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	// 构建并保存执行的命令
	fioArgs, _ := utils.BuildFioArgs(&fio)
	fioCopy.Status.ExecutedCommand = fmt.Sprintf("fio %s", strings.Join(fioArgs, " "))
	logger.Info("设置执行命令", "command", fioCopy.Status.ExecutedCommand)

	// 等待作业完成
	logger.Info("等待FIO作业完成", "jobName", job.Name)
	err = utils.WaitForJob(ctx, r.Client, fio.Namespace, job.Name, 30*time.Minute)
	if err != nil {
		logger.Error(err, "等待FIO作业完成时失败")
		fioCopy.Status.Status = "Failed"
		fioCopy.Status.FailureCount++
		fioCopy.Status.LastResult = fmt.Sprintf("Error waiting for FIO job: %v", err)

		// 添加失败条件
		utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "JobExecutionFailed",
			Message:            fmt.Sprintf("FIO测试执行失败: %v", err),
		})

		// 更新状态
		if updateErr := r.Status().Update(ctx, fioCopy); updateErr != nil {
			logger.Error(updateErr, "更新FIO状态失败（作业执行失败后）")
		}

		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	// 获取作业结果
	jobOutput, err := utils.GetJobResults(ctx, r.Client, fio.Namespace, job.Name)
	if err != nil {
		logger.Error(err, "获取FIO作业结果失败")
		fioCopy.Status.Status = "Failed"
		fioCopy.Status.FailureCount++
		fioCopy.Status.LastResult = fmt.Sprintf("Error getting FIO job results: %v", err)

		// 添加失败条件
		utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "ResultRetrievalFailed",
			Message:            fmt.Sprintf("无法获取FIO测试结果: %v", err),
		})
	} else {
		// 解析FIO输出并更新状态
		fioStats, err := utils.ParseFioOutput(jobOutput)
		if err != nil {
			logger.Error(err, "解析FIO输出失败")
			fioCopy.Status.Status = "Failed"
			fioCopy.Status.FailureCount++
			fioCopy.Status.LastResult = fmt.Sprintf("Error parsing FIO output: %v", err)

			// 添加失败条件
			utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
				Type:               "Failed",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "OutputParsingFailed",
				Message:            fmt.Sprintf("解析FIO测试输出失败: %v", err),
			})
		} else {
			// 更新状态信息
			fioCopy.Status.Status = "Succeeded"
			if fio.Spec.Schedule == "" {
				fioCopy.Status.SuccessCount = 1
			} else {
				fioCopy.Status.SuccessCount++
			}
			fioCopy.Status.LastResult = jobOutput
			fioCopy.Status.Stats = fioStats

			// 确保所有重要字段都被设置
			if fioCopy.Status.QueryCount == 0 {
				fioCopy.Status.QueryCount = 1
			}

			// 记录详细数据
			logger.Info("FIO测试成功完成",
				"filePath", fio.Spec.FilePath,
				"readIOPS", fioStats.ReadIOPS,
				"writeIOPS", fioStats.WriteIOPS,
				"readBW", fioStats.ReadBW,
				"writeBW", fioStats.WriteBW)

			// 标记测试已完成
			utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
				Type:               "Completed",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "TestCompleted",
				Message:            "FIO测试已成功完成",
			})
		}
	}

	// 更新状态
	maxUpdateRetries := 5
	updateRetryDelay := 200 * time.Millisecond

	for i := 0; i < maxUpdateRetries; i++ {
		// 如果不是首次尝试，重新获取对象和重新应用更改
		if i > 0 {
			// 重新获取最新的FIO对象
			if err := r.Get(ctx, req.NamespacedName, &fio); err != nil {
				logger.Error(err, "重新获取FIO进行状态更新重试失败", "attempt", i+1)
				if errors.IsNotFound(err) {
					return ctrl.Result{}, nil
				}
				return ctrl.Result{}, err
			}

			// 重新创建更新对象
			fioCopy = fio.DeepCopy()
			fioCopy.Status.LastExecutionTime = &now
			fioCopy.Status.ExecutedCommand = fmt.Sprintf("fio %s", strings.Join(fioArgs, " "))

			// 如果是首次执行（QueryCount为0），设置为1
			if fio.Status.QueryCount == 0 {
				fioCopy.Status.QueryCount = 1
			}

			// 重新应用所有状态更改
			if jobOutput != "" && err == nil {
				// 重新解析FIO输出
				fioStats, parseErr := utils.ParseFioOutput(jobOutput)
				if parseErr != nil {
					// 解析错误
					fioCopy.Status.Status = "Failed"
					fioCopy.Status.FailureCount = fio.Status.FailureCount + 1
					fioCopy.Status.LastResult = fmt.Sprintf("Error parsing FIO output: %v", parseErr)

					// 添加失败条件
					utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
						Type:               "Failed",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "OutputParsingFailed",
						Message:            fmt.Sprintf("解析FIO测试输出失败: %v", parseErr),
					})
				} else {
					// 更新状态信息
					fioCopy.Status.Status = "Succeeded"
					if fio.Spec.Schedule == "" {
						fioCopy.Status.SuccessCount = 1
					} else {
						fioCopy.Status.SuccessCount++
					}
					fioCopy.Status.LastResult = jobOutput
					fioCopy.Status.Stats = fioStats

					// 确保所有重要字段都被设置
					if fioCopy.Status.QueryCount == 0 {
						fioCopy.Status.QueryCount = 1
					}

					// 记录重试过程中的详细数据
					logger.Info("重试时更新FIO状态数据",
						"filePath", fio.Spec.FilePath,
						"readIOPS", fioStats.ReadIOPS,
						"writeIOPS", fioStats.WriteIOPS,
						"readBW", fioStats.ReadBW,
						"writeBW", fioStats.WriteBW)

					// 标记测试已完成
					utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
						Type:               "Completed",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "TestCompleted",
						Message:            "FIO测试已成功完成",
					})
				}
			} else {
				// 错误状态更新
				fioCopy.Status.Status = "Failed"
				fioCopy.Status.FailureCount = fio.Status.FailureCount + 1

				// 标记失败状态
				utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "ResultRetrievalFailed",
					Message:            "无法获取FIO测试结果",
				})
			}

			logger.Info("重试FIO状态更新", "attempt", i+1, "maxRetries", maxUpdateRetries)
		}

		if updateErr := r.Status().Update(ctx, fioCopy); updateErr != nil {
			if apierrors.IsConflict(updateErr) {
				logger.Info("更新FIO状态时发生冲突，将重试",
					"attempt", i+1,
					"maxRetries", maxUpdateRetries)
				if i < maxUpdateRetries-1 {
					time.Sleep(updateRetryDelay)
					updateRetryDelay *= 2 // 指数退避
					continue
				}
			}
			logger.Error(updateErr, "更新FIO状态失败")
			return ctrl.Result{RequeueAfter: time.Second * 5}, updateErr
		} else {
			// 成功更新，退出循环
			logger.Info("成功更新FIO状态")
			break
		}
	}

	// 只有在明确设置了Schedule字段时才安排下一次执行
	if fio.Spec.Schedule != "" {
		interval := 60 // 默认60秒
		if fio.Spec.Schedule != "" {
			if intervalValue, err := strconv.Atoi(fio.Spec.Schedule); err == nil && intervalValue > 0 {
				interval = intervalValue
			}
		}
		logger.Info("调度下一次FIO测试", "interval", interval)
		return ctrl.Result{RequeueAfter: time.Duration(interval) * time.Second}, nil
	}

	// 默认情况下，不再调度下一次执行
	logger.Info("FIO测试已完成，不再调度下一次执行",
		"fio", fio.Name,
		"namespace", fio.Namespace)
	return ctrl.Result{}, nil
}

// executeFio executes FIO tests and returns the results
func (r *FioReconciler) executeFio(ctx context.Context, fio *testtoolsv1.Fio) (testtoolsv1.FioStats, error) {
	log := log.FromContext(ctx)
	log.Info("Executing FIO test", "name", fio.Name, "namespace", fio.Namespace)

	// Create job name
	jobName := fmt.Sprintf("fio-%s-%s", fio.Name, string(fio.UID)[:8])

	// Check if job exists
	var job batchv1.Job
	err := r.Get(ctx, types.NamespacedName{Namespace: fio.Namespace, Name: jobName}, &job)

	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to check for existing job")
			return testtoolsv1.FioStats{}, fmt.Errorf("failed to check for existing job: %w", err)
		}

		// Job doesn't exist, create it
		log.Info("Creating new FIO job", "name", jobName)
		jobObj, err := utils.PrepareFioJob(ctx, fio, r.Client, r.Scheme)
		if err != nil {
			log.Error(err, "Failed to prepare FIO job")
			return testtoolsv1.FioStats{}, fmt.Errorf("failed to prepare job: %w", err)
		}

		// Store job name for later use
		jobName = jobObj.Name
		log.Info("FIO job created", "name", jobName)
	} else {
		log.Info("Found existing FIO job", "name", jobName, "status", job.Status)
	}

	// Wait for job completion
	err = utils.WaitForJob(ctx, r.Client, fio.Namespace, jobName, 15*time.Minute)
	if err != nil {
		return testtoolsv1.FioStats{}, fmt.Errorf("error waiting for job: %w", err)
	}

	// Get job pods
	pods := &v1.PodList{}
	if err := r.List(ctx, pods,
		client.InNamespace(fio.Namespace),
		client.MatchingLabels{"job-name": jobName}); err != nil {
		return testtoolsv1.FioStats{}, fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return testtoolsv1.FioStats{}, fmt.Errorf("no pods found for job %s", jobName)
	}

	// Get results
	results, err := utils.GetJobResults(ctx, r.Client, fio.Namespace, pods.Items[0].Name)
	if err != nil {
		return testtoolsv1.FioStats{}, fmt.Errorf("failed to get results: %w", err)
	}

	// Parse FIO output
	fioStats, err := utils.ParseFioOutput(results)
	if err != nil {
		return testtoolsv1.FioStats{}, fmt.Errorf("failed to parse output: %w", err)
	}

	return fioStats, nil
}

// updateFioStatus 使用乐观锁更新Fio状态
func (r *FioReconciler) updateFioStatus(ctx context.Context, name types.NamespacedName, updateFn func(*testtoolsv1.Fio)) error {
	logger := log.FromContext(ctx)
	retries := 3
	backoff := 100 * time.Millisecond

	for i := 0; i < retries; i++ {
		// 每次都获取最新版本
		var fio testtoolsv1.Fio
		if err := r.Get(ctx, name, &fio); err != nil {
			if errors.IsNotFound(err) {
				logger.Info("无法找到要更新的Fio资源", "name", name.Name, "namespace", name.Namespace)
				return err
			}
			logger.Error(err, "获取Fio资源失败", "name", name.Name, "namespace", name.Namespace)
			return err
		}

		// 深拷贝当前状态以便比较
		oldStatus := fio.Status.DeepCopy()

		// 应用更新函数
		updateFn(&fio)

		// 如果状态实际未变化，无需更新
		if reflect.DeepEqual(oldStatus, &fio.Status) {
			logger.Info("Fio状态未变化，跳过更新", "name", name.Name, "namespace", name.Namespace)
			return nil
		}

		// 尝试更新
		if err := r.Status().Update(ctx, &fio); err != nil {
			if errors.IsConflict(err) {
				// 冲突则重试
				logger.Info("更新Fio状态时发生冲突，准备重试",
					"attempt", i+1,
					"maxRetries", retries,
					"name", name.Name,
					"namespace", name.Namespace)

				// 使用指数退避策略
				if i < retries-1 {
					time.Sleep(backoff)
					backoff *= 2 // 指数增长
					continue
				}
			}

			logger.Error(err, "更新Fio状态失败",
				"name", name.Name,
				"namespace", name.Namespace,
				"attempt", i+1)
			return err
		}

		logger.Info("成功更新Fio状态", "name", name.Name, "namespace", name.Namespace)
		return nil
	}

	return fmt.Errorf("在%d次尝试后仍无法更新Fio状态", retries)
}

// SetupWithManager sets up the controller with the Manager.
func (r *FioReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testtoolsv1.Fio{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
