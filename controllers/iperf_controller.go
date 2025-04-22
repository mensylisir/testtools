package controllers

import (
	"context"
	"fmt"
	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	"github.com/xiaoming/testtools/pkg/utils"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strconv"
	"strings"
	"time"
)

const IperfFinalizerName = "iperf.testtools.xiaoming.com/finalizer"

type IperfReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=iperves,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=iperves/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=iperves/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *IperfReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Start to reconcile iperf resource", "namespace", req.Namespace, "name", req.Name)

	startTime := time.Now()
	defer func() {
		logger.Info("End to reconcile iperf resource", "namespace", req.Namespace, "name", req.Name, "duration", time.Since(startTime).String())
	}()

	// 获取Iperf对象
	var iperf testtoolsv1.Iperf
	if err := r.Get(ctx, req.NamespacedName, &iperf); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("The iperf resource is not exist", "namespace", req.Namespace, "name", req.Name)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to retrive the iperf resource", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, err
	}

	if !iperf.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&iperf, IperfFinalizerName) {
			logger.Info("The iperf resource is being deleted", "namespace", req.Namespace, "name", req.Name)
			if err := r.cleanupResources(ctx, &iperf); err != nil {
				logger.Error(err, "Failed to clean resource", "namespace", req.Namespace, "name", req.Name)
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&iperf, IperfFinalizerName)
			if err := r.Update(ctx, &iperf); err != nil {
				logger.Error(err, "Failed to remove finalizer", "namespace", req.Namespace, "name", req.Name)
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&iperf, IperfFinalizerName) {
		controllerutil.AddFinalizer(&iperf, IperfFinalizerName)
		if err := r.Update(ctx, &iperf); err != nil {
			logger.Error(err, "Failed to add finalizer", "namespace", req.Namespace, "name", req.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	shouldExecute := false

	if iperf.Spec.Schedule != "" {
		if iperf.Status.LastExecutionTime != nil {
			intervalSeconds := 60
			if scheduleValue, err := strconv.Atoi(iperf.Spec.Schedule); err == nil && scheduleValue > 0 {
				intervalSeconds = scheduleValue
			}

			nextExecutionTime := iperf.Status.LastExecutionTime.Add(time.Duration(intervalSeconds) * time.Second)
			shouldExecute = time.Now().After(nextExecutionTime)

			if !shouldExecute {
				logger.Info("Skipping execution due It is not yet the scheduled time.",
					"namespace", req.Namespace, "name", req.Name,
					"lastExecutionTime", iperf.Status.LastExecutionTime.Time,
					"nextExecutionTime", nextExecutionTime,
					"currentTime", time.Now())
				delay := time.Until(nextExecutionTime)
				if delay < 0 {
					delay = time.Second * 5
				}
				return ctrl.Result{RequeueAfter: delay}, nil
			}
		} else {
			shouldExecute = true
		}
	} else {
		shouldExecute = iperf.Status.LastExecutionTime == nil
	}

	if !shouldExecute {
		logger.Info("The iperf resource does not need to be executed.",
			"namespace", req.Namespace, "name", req.Name,
			"lastExecutionTime", iperf.Status.LastExecutionTime.Time,
			"currentTime", time.Now(),
			"schedule", iperf.Spec.Schedule)
		return ctrl.Result{}, nil
	}

	iperfCopy := iperf.DeepCopy()
	now := metav1.NewTime(time.Now())
	iperfCopy.Status.LastExecutionTime = &now

	if iperfCopy.Status.QueryCount == 0 {
		iperfCopy.Status.QueryCount = 1
	}

	var serverJobName, clientJobName, serverPodIP string

	serverJobName, err := utils.PrepareIperfServerJob(ctx, r.Client, &iperf)
	if err != nil {
		logger.Error(err, "failed to create Server Job: %v", "name", iperf.Name, "namespace", req.Namespace, "name", req.Name, "serverJobName", serverJobName)
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	serverPodIP, err = r.getServerPodIP(ctx, &iperf, serverJobName)
	if err != nil {
		logger.Error(err, "Failed to retrive server pod ip: %v", "name", iperf.Name, "namespace", req.Namespace, "name", req.Name, "serverJobName", serverJobName)
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}
	if len(serverPodIP) == 0 {
		logger.Error(err, "Failed to retrive server pod ip: %v", "name", iperf.Name, "namespace", req.Namespace, "name", req.Name, "serverJobName", serverJobName)
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	var serverJob batchv1.Job
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: serverJobName}, &serverJob); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("iperf server job not found", "namespace", req.Namespace, "name", req.Name, "job", serverJobName)
			return ctrl.Result{}, err
		}
		logger.Error(err, "Failed to get server job", "namespace", req.Namespace, "name", req.Name, "job", serverJobName)
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	if serverJob.Status.Active < 1 {
		logger.Error(err, "Waitting to server job active", "namespace", req.Namespace, "name", req.Name, "job", serverJobName)
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	clientJobName, err = utils.PrepareIperfClientJob(ctx, r.Client, &iperf)
	if err != nil {
		logger.Error(err, "failed to create client Job: %v", "name", iperf.Name, "namespace", req.Namespace, "name", req.Name, "serverJobName", serverJobName)
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	if len(serverPodIP) > 0 {
		iperfCopy.Spec.Host = serverPodIP
	}

	iperfServerArgs := utils.BuildIperfServerArgs(iperfCopy)
	iperfClientArgs := utils.BuildIperfClientArgs(iperfCopy)
	iperfCopy.Status.ExecutedCommand = fmt.Sprintf("iperf3 %s\n", strings.Join(iperfServerArgs, " "))
	iperfCopy.Status.ExecutedCommand += fmt.Sprintf("iperf3 %s", strings.Join(iperfClientArgs, " "))
	logger.Info("Setting executed command", "command", iperfCopy.Status.ExecutedCommand)

	var clientJob batchv1.Job
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: clientJobName}, &clientJob); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("iperf client job not found", "namespace", req.Namespace, "name", req.Name, "job", clientJobName)
			return ctrl.Result{}, err
		}
		logger.Error(err, "Failed to get job", "namespace", req.Namespace, "name", req.Name, "job", clientJobName)
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	} else {
		if clientJob.Status.Succeeded > 0 {
			logger.Info("Job completed successfully", "namespace", req.Namespace, "name", req.Name, "job", clientJobName)

			clientJobOutput, err := utils.GetJobResults(ctx, r.Client, iperf.Namespace, clientJobName)

			if err != nil {
				logger.Error(err, "Failed to get iperf job results", "namespace", req.Namespace, "name", req.Name)
				iperfCopy.Status.Status = "Failed"
				iperfCopy.Status.FailureCount++
				iperfCopy.Status.LastResult = fmt.Sprintf("Error getting iperf job results: %v", err)

				if iperf.Status.TestReportName == "" {
					iperf.Status.TestReportName = utils.GenerateTestReportName("iperf", iperf.Name)
				}

				utils.SetCondition(&iperfCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "ResultRetrievalFailed",
					Message:            fmt.Sprintf("Failed to get iperf job results: %v", err),
				})
			} else {
				clientOutput, err := utils.ParseIperfOutput(clientJobOutput, iperfCopy)
				if err != nil {
					logger.Error(err, "Failed to parse iperf client output", "namespace", req.Namespace, "name", req.Name)
					iperfCopy.Status.Status = "Failed"
					iperfCopy.Status.FailureCount++
					iperfCopy.Status.LastResult = fmt.Sprintf("Error getting iperf job results: %v", err)

					// 设置TestReportName，即使失败也设置
					if iperfCopy.Status.TestReportName == "" {
						iperfCopy.Status.TestReportName = utils.GenerateTestReportName("Iperf", iperf.Name)
					}

					// 添加失败条件
					utils.SetCondition(&iperfCopy.Status.Conditions, metav1.Condition{
						Type:               "Failed",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "ResultRetrievalFailed",
						Message:            fmt.Sprintf("无法获取Iperf测试结果: %v", err),
					})
				} else if clientOutput.Status == "SUCCESS" {
					iperfCopy.Status.Status = "Succeeded"
					if iperfCopy.Spec.Schedule == "" {
						iperfCopy.Status.SuccessCount = 1
					} else {
						iperfCopy.Status.SuccessCount++
					}
					iperfCopy.Status.LastResult = clientJobOutput

					iperfCopy.Status.Stats = testtoolsv1.IperfStats{
						StartTime:        iperfCopy.Status.LastExecutionTime,
						EndTime:          &metav1.Time{Time: time.Now()},
						SentBytes:        clientOutput.SentBytes,
						ReceivedBytes:    clientOutput.ReceivedBytes,
						SendBandwidth:    fmt.Sprintf("%f", clientOutput.SendBandwidth),
						ReceiveBandwidth: fmt.Sprintf("%f", clientOutput.ReceiveBandwidth),
						Jitter:           fmt.Sprintf("%f", clientOutput.Jitter),
						LostPackets:      clientOutput.LostPackets,
						LostPercent:      fmt.Sprintf("%f", clientOutput.LostPercent),
						Retransmits:      clientOutput.Retransmits,
						RttMs:            fmt.Sprintf("%f", clientOutput.RttMs),
						CpuUtilization:   fmt.Sprintf("%f", clientOutput.CpuUtilization),
					}

					logger.Info("Iperf test succeeded", "server.host", clientOutput.Host,
						"server.port", clientOutput.Port,
						"client.host", clientOutput.Host,
						"client.port", clientOutput.Port)

					utils.SetCondition(&iperfCopy.Status.Conditions, metav1.Condition{
						Type:               "Succeeded",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "TestCompleted",
						Message:            fmt.Sprintf("Iperf test completed successfully"),
					})
				} else {
					iperfCopy.Status.Status = "Failed"
					if iperfCopy.Spec.Schedule == "" {
						iperfCopy.Status.FailureCount = 1
					} else {
						iperfCopy.Status.FailureCount++
					}
					iperfCopy.Status.LastResult = clientJobOutput

					iperfCopy.Status.Stats = testtoolsv1.IperfStats{
						StartTime:        iperfCopy.Status.LastExecutionTime,
						EndTime:          &metav1.Time{Time: time.Now()},
						SentBytes:        clientOutput.SentBytes,
						ReceivedBytes:    clientOutput.ReceivedBytes,
						SendBandwidth:    fmt.Sprintf("%f", clientOutput.SendBandwidth),
						ReceiveBandwidth: fmt.Sprintf("%f", clientOutput.ReceiveBandwidth),
						Jitter:           fmt.Sprintf("%f", clientOutput.Jitter),
						LostPackets:      clientOutput.LostPackets,
						LostPercent:      fmt.Sprintf("%f", clientOutput.LostPercent),
						Retransmits:      clientOutput.Retransmits,
						RttMs:            fmt.Sprintf("%f", clientOutput.RttMs),
						CpuUtilization:   fmt.Sprintf("%f", clientOutput.CpuUtilization),
					}

					logger.Info("Iperf test completed failed", "server.host", clientOutput.Host,
						"server.port", clientOutput.Port,
						"client.host", clientOutput.Host,
						"client.port", clientOutput.Port)

					utils.SetCondition(&iperfCopy.Status.Conditions, metav1.Condition{
						Type:               "Failed",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "TestCompleted",
						Message:            fmt.Sprintf("Iperf test completed failed"),
					})
				}
				if iperfCopy.Status.TestReportName == "" {
					iperfCopy.Status.TestReportName = utils.GenerateTestReportName("iperf", iperf.Name)
				}
				if iperfCopy.Status.QueryCount == 0 {
					iperfCopy.Status.QueryCount = 1
				}
			}

			if updateErr := r.Status().Update(ctx, iperfCopy); updateErr != nil {
				logger.Info(updateErr.Error(), "failed to update iperf status", "iperf", iperf.Name, "namespace", iperf.Namespace)
				return ctrl.Result{RequeueAfter: time.Second * 30}, nil
			}

		} else if clientJob.Status.Failed > *clientJob.Spec.BackoffLimit {
			logger.Error(nil, "Job failed", "namespace", req.Namespace, "name", req.Name, "job", clientJobName,
				"failed", clientJob.Status.Failed, "backoffLimit", clientJob.Spec.BackoffLimit)
			clientJobOutput, err := utils.GetJobResults(ctx, r.Client, iperf.Namespace, clientJobName)

			if err != nil {
				logger.Error(err, "Failed to get iperf job results", "namespace", req.Namespace, "name", req.Name)
				iperfCopy.Status.Status = "Failed"
				iperfCopy.Status.FailureCount++
				iperfCopy.Status.LastResult = fmt.Sprintf("Error getting iperf job results: %v", err)

				if iperf.Status.TestReportName == "" {
					iperf.Status.TestReportName = utils.GenerateTestReportName("iperf", iperf.Name)
				}

				utils.SetCondition(&iperfCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "ResultRetrievalFailed",
					Message:            fmt.Sprintf("Failed to get iperf job results: %v", err),
				})
			} else {
				clientOutput, err := utils.ParseIperfOutput(clientJobOutput, iperfCopy)
				if err != nil {
					logger.Error(err, "Failed to parse iperf client output", "namespace", req.Namespace, "name", req.Name)
					iperfCopy.Status.Status = "Failed"
					iperfCopy.Status.FailureCount++
					iperfCopy.Status.LastResult = fmt.Sprintf("Error to parse iperf client output: %v", err)

					// 设置TestReportName，即使失败也设置
					if iperfCopy.Status.TestReportName == "" {
						iperfCopy.Status.TestReportName = utils.GenerateTestReportName("Iperf", iperf.Name)
					}

					// 添加失败条件
					utils.SetCondition(&iperfCopy.Status.Conditions, metav1.Condition{
						Type:               "Failed",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "ResultRetrievalFailed",
						Message:            fmt.Sprintf("无法获取Iperf测试结果: %v", err),
					})
				} else {
					iperfCopy.Status.Status = "Failed"
					if iperfCopy.Spec.Schedule == "" {
						iperfCopy.Status.FailureCount = 1
					} else {
						iperfCopy.Status.FailureCount++
					}
					iperfCopy.Status.LastResult = clientJobOutput

					iperfCopy.Status.Stats = testtoolsv1.IperfStats{
						StartTime:        iperfCopy.Status.LastExecutionTime,
						EndTime:          &metav1.Time{Time: time.Now()},
						SentBytes:        clientOutput.SentBytes,
						ReceivedBytes:    clientOutput.ReceivedBytes,
						SendBandwidth:    fmt.Sprintf("%f", clientOutput.SendBandwidth),
						ReceiveBandwidth: fmt.Sprintf("%f", clientOutput.ReceiveBandwidth),
						Jitter:           fmt.Sprintf("%f", clientOutput.Jitter),
						LostPackets:      clientOutput.LostPackets,
						LostPercent:      fmt.Sprintf("%f", clientOutput.LostPercent),
						Retransmits:      clientOutput.Retransmits,
						RttMs:            fmt.Sprintf("%f", clientOutput.RttMs),
						CpuUtilization:   fmt.Sprintf("%f", clientOutput.CpuUtilization),
					}

					logger.Info("Iperf test failed", "server.host", clientOutput.Host,
						"server.port", clientOutput.Port,
						"client.host", clientOutput.Host,
						"client.port", clientOutput.Port)

					utils.SetCondition(&iperfCopy.Status.Conditions, metav1.Condition{
						Type:               "Failed",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "TestCompleted",
						Message:            fmt.Sprintf("Iperf test completed failed"),
					})
				}
				if iperfCopy.Status.TestReportName == "" {
					iperfCopy.Status.TestReportName = utils.GenerateTestReportName("iperf", iperf.Name)
				}
				if iperfCopy.Status.QueryCount == 0 {
					iperfCopy.Status.QueryCount = 1
				}
			}

			if updateErr := r.Status().Update(ctx, iperfCopy); updateErr != nil {
				logger.Info(updateErr.Error(), "failed to update iperf status", "iperf", iperf.Name, "namespace", iperf.Namespace)
				return ctrl.Result{RequeueAfter: time.Second * 30}, nil
			}

		} else {
			logger.Info("Job is still running", "namespace", req.Namespace, "name", req.Name, "job", clientJobName,
				"successded", clientJob.Status.Succeeded, "failed", clientJob.Status.Failed, "backoffLimit", clientJob.Spec.BackoffLimit)
			return ctrl.Result{}, nil
		}
	}

	if iperf.Spec.Schedule != "" {
		interval := 60
		if iperf.Spec.Schedule != "" {
			if intervalValue, err := strconv.Atoi(iperf.Spec.Schedule); err == nil && intervalValue > 0 {
				interval = intervalValue
			}
		}
		logger.Info("Scheduling next iperf test", "interval", interval)
		return ctrl.Result{RequeueAfter: time.Second * time.Duration(interval)}, nil
	}

	logger.Info("Skoop test finished; no further execution will be scheduled.",
		"skoop", iperf.Name,
		"namespace", iperf.Namespace)
	return ctrl.Result{}, nil
}

func (r *IperfReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testtoolsv1.Iperf{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

func (r *IperfReconciler) cleanupResources(ctx context.Context, iperf *testtoolsv1.Iperf) error {
	logger := log.FromContext(ctx)
	logger.Info("Start to cleanup resources", "namespace", iperf.Namespace, "name", iperf.Name)
	return nil
}

func (r *IperfReconciler) getServerPodIP(ctx context.Context, iperf *testtoolsv1.Iperf, serverJobName string) (string, error) {
	// 获取服务器Job
	var job batchv1.Job
	jobKey := client.ObjectKey{Namespace: iperf.Namespace, Name: serverJobName}
	if err := r.Get(ctx, jobKey, &job); err != nil {
		return "", fmt.Errorf("获取服务器Job失败: %v", err)
	}

	// 获取该Job创建的Pod
	var podList corev1.PodList
	if err := r.List(ctx, &podList,
		client.InNamespace(iperf.Namespace),
		client.MatchingLabels(job.Spec.Template.Labels)); err != nil {
		return "", fmt.Errorf("获取服务器Pod失败: %v", err)
	}

	// 查找处于Running状态的Pod
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			// 确保Pod有IP地址
			if pod.Status.PodIP != "" {
				return pod.Status.PodIP, nil
			}
		}
	}

	return "", fmt.Errorf("没有找到运行中的服务器Pod")
}
