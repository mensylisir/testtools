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

	// 添加finalizer处理逻辑
	fioFinalizer := "testtools.xiaoming.com/fio-finalizer"

	// 检查对象是否正在被删除
	if fio.ObjectMeta.DeletionTimestamp.IsZero() {
		// 对象没有被标记为删除，确保它有我们的finalizer
		if !utils.ContainsString(fio.GetFinalizers(), fioFinalizer) {
			logger.Info("添加finalizer", "finalizer", fioFinalizer)
			fio.SetFinalizers(append(fio.GetFinalizers(), fioFinalizer))
			if err := r.Update(ctx, &fio); err != nil {
				return ctrl.Result{}, err
			}
			// 已更新finalizer，重新触发reconcile
			return ctrl.Result{}, nil
		}
	} else {
		// 对象正在被删除
		if utils.ContainsString(fio.GetFinalizers(), fioFinalizer) {
			// 执行清理操作
			if err := r.cleanupResources(ctx, &fio); err != nil {
				logger.Error(err, "清理资源失败")
				return ctrl.Result{}, err
			}

			// 清理完成后，移除finalizer
			fio.SetFinalizers(utils.RemoveString(fio.GetFinalizers(), fioFinalizer))
			if err := r.Update(ctx, &fio); err != nil {
				return ctrl.Result{}, err
			}
			logger.Info("已移除finalizer，允许资源被删除")
			return ctrl.Result{}, nil
		}
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

			// 确保所有数据字段都有值，即使为0也要明确设置
			if fioCopy.Status.QueryCount == 0 {
				fioCopy.Status.QueryCount = 1
			}

			// 确保Stats中的字段不为零
			if fioCopy.Status.Stats.ReadIOPS == 0 && fioStats.ReadIOPS > 0 {
				fioCopy.Status.Stats.ReadIOPS = fioStats.ReadIOPS
			}
			if fioCopy.Status.Stats.WriteIOPS == 0 && fioStats.WriteIOPS > 0 {
				fioCopy.Status.Stats.WriteIOPS = fioStats.WriteIOPS
			}
			if fioCopy.Status.Stats.ReadBW == 0 && fioStats.ReadBW > 0 {
				fioCopy.Status.Stats.ReadBW = fioStats.ReadBW
			}
			if fioCopy.Status.Stats.WriteBW == 0 && fioStats.WriteBW > 0 {
				fioCopy.Status.Stats.WriteBW = fioStats.WriteBW
			}

			// 记录详细数据
			logger.Info("FIO测试成功完成",
				"filePath", fio.Spec.FilePath,
				"readIOPS", fioCopy.Status.Stats.ReadIOPS,
				"writeIOPS", fioCopy.Status.Stats.WriteIOPS,
				"readBW", fioCopy.Status.Stats.ReadBW,
				"writeBW", fioCopy.Status.Stats.WriteBW)

			// 标记测试已完成
			utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
				Type:               "Completed",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "TestCompleted",
				Message:            "FIO测试已成功完成",
			})

			// 设置TestReportName，使TestReport控制器能够自动创建报告
			if fioCopy.Status.TestReportName == "" {
				fioCopy.Status.TestReportName = fmt.Sprintf("fio-%s-report", fio.Name)
			}
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

					// 确保所有数据字段都有值，即使为0也要明确设置
					if fioCopy.Status.QueryCount == 0 {
						fioCopy.Status.QueryCount = 1
					}

					// 确保Stats中的字段不为零
					if fioCopy.Status.Stats.ReadIOPS == 0 && fioStats.ReadIOPS > 0 {
						fioCopy.Status.Stats.ReadIOPS = fioStats.ReadIOPS
					}
					if fioCopy.Status.Stats.WriteIOPS == 0 && fioStats.WriteIOPS > 0 {
						fioCopy.Status.Stats.WriteIOPS = fioStats.WriteIOPS
					}
					if fioCopy.Status.Stats.ReadBW == 0 && fioStats.ReadBW > 0 {
						fioCopy.Status.Stats.ReadBW = fioStats.ReadBW
					}
					if fioCopy.Status.Stats.WriteBW == 0 && fioStats.WriteBW > 0 {
						fioCopy.Status.Stats.WriteBW = fioStats.WriteBW
					}

					// 记录重试过程中的详细数据
					logger.Info("重试时更新FIO状态数据",
						"filePath", fio.Spec.FilePath,
						"readIOPS", fioCopy.Status.Stats.ReadIOPS,
						"writeIOPS", fioCopy.Status.Stats.WriteIOPS,
						"readBW", fioCopy.Status.Stats.ReadBW,
						"writeBW", fioCopy.Status.Stats.WriteBW)

					// 标记测试已完成
					utils.SetCondition(&fioCopy.Status.Conditions, metav1.Condition{
						Type:               "Completed",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "TestCompleted",
						Message:            "FIO测试已成功完成",
					})

					// 设置TestReportName，使TestReport控制器能够自动创建报告
					if fioCopy.Status.TestReportName == "" {
						fioCopy.Status.TestReportName = fmt.Sprintf("fio-%s-report", fio.Name)
					}
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
			"app.kubernetes.io/name":       "fio-job",
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

	// 不再删除TestReport资源，这应该由TestReport控制器自己管理
	// TestReport通过其ResourceSelectors引用Fio，当Fio删除时，TestReport控制器会负责清理或更新TestReport

	logger.Info("资源清理完成", "fioName", fio.Name)
	return nil
}
