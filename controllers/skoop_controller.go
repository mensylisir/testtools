package controllers

import (
	"context"
	"fmt"
	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	"github.com/xiaoming/testtools/pkg/utils"
	batchv1 "k8s.io/api/batch/v1"
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

const SkoopFinalizerName = "skoop.testtools.xiaoming.com/finalizer"

type SkoopReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=skoops,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=skoops/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=skoops/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *SkoopReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Start to reconcile Skoop", "namespace", req.Namespace, "name", req.Name)

	startTime := time.Now()
	defer func() {
		logger.Info("End to reconcile Skoop", "namespace", req.Namespace, "name", req.Name, "duration", time.Since(startTime).String())
	}()

	var skoop testtoolsv1.Skoop
	if err := r.Get(ctx, req.NamespacedName, &skoop); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("The skoop resource is not exist", "namespace", req.Namespace, "name", req.Name)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to retrive the skoop resource", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, err
	}

	if !skoop.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&skoop, SkoopFinalizerName) {
			logger.Info("The skoop resource is being deleted", "namespace", req.Namespace, "name", req.Name)
			if err := r.cleanupResources(ctx, &skoop); err != nil {
				logger.Error(err, "Failed to clean resource", "namespace", req.Namespace, "name", req.Name)
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&skoop, SkoopFinalizerName)
			if err := r.Update(ctx, &skoop); err != nil {
				logger.Error(err, "Failed to remove finalizer", "namespace", req.Namespace, "name", req.Name)
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&skoop, SkoopFinalizerName) {
		controllerutil.AddFinalizer(&skoop, SkoopFinalizerName)
		if err := r.Update(ctx, &skoop); err != nil {
			logger.Error(err, "Failed to add finalizer", "namespace", req.Namespace, "name", req.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	shouldExecute := false

	if skoop.Spec.Schedule != "" {
		if skoop.Status.LastExecutionTime != nil {
			intervalSeconds := 60
			if scheduleValue, err := strconv.Atoi(skoop.Spec.Schedule); err == nil && scheduleValue > 0 {
				intervalSeconds = scheduleValue
			}

			nextExecutionTime := skoop.Status.LastExecutionTime.Add(time.Duration(intervalSeconds) * time.Second)
			shouldExecute = time.Now().After(nextExecutionTime)

			if !shouldExecute {
				logger.Info("Skipping execution due It is not yet the scheduled time.",
					"namespace", req.Namespace, "name", req.Name,
					"lastExecutionTime", skoop.Status.LastExecutionTime.Time,
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
		shouldExecute = skoop.Status.LastExecutionTime == nil
	}

	if !shouldExecute {
		logger.Info("The skoop resource does not need to be executed.",
			"namespace", req.Namespace, "name", req.Name,
			"lastExecutionTime", skoop.Status.LastExecutionTime.Time,
			"currentTime", time.Now(),
			"schedule", skoop.Spec.Schedule)
		return ctrl.Result{}, nil
	}

	skoopCopy := skoop.DeepCopy()
	now := metav1.NewTime(time.Now())
	skoopCopy.Status.LastExecutionTime = &now

	if skoopCopy.Status.QueryCount == 0 {
		skoopCopy.Status.QueryCount = 1
	}

	jobName, err := utils.PrepareSkoopJob(ctx, r.Client, &skoop)
	if err != nil {
		logger.Error(err, "Failed to prepare skoop job", "namespace", req.Namespace, "name", req.Name)
		skoopCopy.Status.Status = "Failed"
		skoopCopy.Status.FailureCount++
		skoopCopy.Status.LastResult = fmt.Sprintf("Error preparing skoop job: %v", err)

		utils.SetCondition(&skoopCopy.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "JobPreparationFailed",
			Message:            fmt.Sprintf("Failed to prepare skoop job: %v", err),
		})

		if updateErr := r.Status().Update(ctx, skoopCopy); updateErr != nil {
			logger.Error(updateErr, "Failed to update skoop status after job preparation failure")
		}
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	skoopArgs := utils.BuildSkoopArgs(skoopCopy)
	skoopCopy.Status.ExecutedCommand = fmt.Sprintf("skoop %s", strings.Join(skoopArgs, " "))
	logger.Info("Setting executed command", "command", skoopCopy.Status.ExecutedCommand)

	var job batchv1.Job
	if err = r.Client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: jobName}, &job); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Skoop job not found", "namespace", req.Namespace, "name", req.Name, "job", jobName)
			return ctrl.Result{}, err
		}
		logger.Error(err, "Failed to get job", "namespace", req.Namespace, "name", req.Name, "job", jobName)
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	} else {
		if job.Status.Succeeded > 0 {
			logger.Info("Job completed successfully", "namespace", req.Namespace, "name", req.Name, "job", jobName)

			jobOutput, err := utils.GetJobResults(ctx, r.Client, skoop.Namespace, jobName)
			if err != nil {
				logger.Error(err, "Failed to get skoop job results")
				skoopCopy.Status.Status = "Failed"
				skoopCopy.Status.FailureCount++
				skoopCopy.Status.LastResult = fmt.Sprintf("Error getting skoop job results: %v", err)

				if skoopCopy.Status.TestReportName == "" {
					skoopCopy.Status.TestReportName = utils.GenerateTestReportName("Skoop", skoop.Name)
				}

				utils.SetCondition(&skoopCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "ResultRetrievalFailed",
					Message:            fmt.Sprintf("Failed to get skoop job results: %v", err),
				})
			} else {
				skoopOutput, err := utils.ParseSkoopOutput(jobOutput, skoopCopy)
				if err != nil {
					logger.Error(err, "Failed to parse skoop output", "namespace", req.Namespace, "name", req.Name)
					skoopCopy.Status.Status = "Failed"
					skoopCopy.Status.FailureCount++
					skoopCopy.Status.LastResult = fmt.Sprintf("Error to parse skoop output: %v", err)

					// 设置TestReportName，即使失败也设置
					if skoopCopy.Status.TestReportName == "" {
						skoopCopy.Status.TestReportName = utils.GenerateTestReportName("Iperf", skoop.Name)
					}

					// 添加失败条件
					utils.SetCondition(&skoopCopy.Status.Conditions, metav1.Condition{
						Type:               "Failed",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "ResultRetrievalFailed",
						Message:            fmt.Sprintf("Error to parse skoop output: %v", err),
					})
				} else {
					skoopCopy.Status.Status = "Succeeded"
					if skoopCopy.Spec.Schedule == "" {
						skoopCopy.Status.SuccessCount = 1
					} else {
						skoopCopy.Status.SuccessCount++
					}
					skoopCopy.Status.LastResult = jobOutput

					if skoopCopy.Status.TestReportName == "" {
						skoopCopy.Status.TestReportName = utils.GenerateTestReportName("Skoop", skoop.Name)
					}

					if skoopCopy.Status.QueryCount == 0 {
						skoopCopy.Status.QueryCount = 1
					}

					logger.Info("Skoop test succeeded",
						"host", skoopOutput.DestinationAddress,
						"port", skoopOutput.DestinationPort)

					utils.SetCondition(&skoopCopy.Status.Conditions, metav1.Condition{
						Type:               "Succeeded",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "TestCompleted",
						Message:            fmt.Sprintf("Skoop test completed successfully"),
					})
				}
			}
			if updateErr := r.Status().Update(ctx, skoopCopy); updateErr != nil {
				logger.Info(updateErr.Error(), "failed to update skoop status", "skoop", skoop.Name, "namespace", skoop.Namespace)
				return ctrl.Result{RequeueAfter: time.Second * 30}, nil
			}

		} else if job.Status.Failed > *job.Spec.BackoffLimit {
			logger.Error(nil, "Job failed", "namespace", req.Namespace, "name", req.Name, "job", jobName,
				"failed", job.Status.Failed, "backoffLimit", job.Spec.BackoffLimit)

			jobOutput, err := utils.GetJobResults(ctx, r.Client, skoop.Namespace, jobName)
			if err != nil {
				logger.Error(err, "Failed to get skoop job results")
				skoopCopy.Status.Status = "Failed"
				skoopCopy.Status.FailureCount++
				skoopCopy.Status.LastResult = fmt.Sprintf("Error getting skoop job results: %v", err)

				if skoopCopy.Status.TestReportName == "" {
					skoopCopy.Status.TestReportName = utils.GenerateTestReportName("Skoop", skoop.Name)
				}

				utils.SetCondition(&skoopCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "ResultRetrievalFailed",
					Message:            fmt.Sprintf("Failed to get skoop job results: %v", err),
				})
			} else {
				skoopOutput, err := utils.ParseSkoopOutput(jobOutput, skoopCopy)
				if err != nil {
					logger.Error(err, "Failed to parse skoop output", "namespace", req.Namespace, "name", req.Name)
					skoopCopy.Status.Status = "Failed"
					skoopCopy.Status.FailureCount++
					skoopCopy.Status.LastResult = fmt.Sprintf("Error to parse skoop output: %v", err)

					// 设置TestReportName，即使失败也设置
					if skoopCopy.Status.TestReportName == "" {
						skoopCopy.Status.TestReportName = utils.GenerateTestReportName("Iperf", skoop.Name)
					}

					// 添加失败条件
					utils.SetCondition(&skoopCopy.Status.Conditions, metav1.Condition{
						Type:               "Failed",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "ResultRetrievalFailed",
						Message:            fmt.Sprintf("Error to parse skoop output: %v", err),
					})
				} else {
					skoopCopy.Status.Status = "Failed"
					if skoopCopy.Spec.Schedule == "" {
						skoopCopy.Status.FailureCount = 1
					} else {
						skoopCopy.Status.SuccessCount++
					}
					skoopCopy.Status.LastResult = jobOutput

					if skoopCopy.Status.TestReportName == "" {
						skoopCopy.Status.TestReportName = utils.GenerateTestReportName("Skoop", skoop.Name)
					}

					if skoopCopy.Status.QueryCount == 0 {
						skoopCopy.Status.QueryCount = 1
					}

					logger.Info("Skoop test failed",
						"host", skoopOutput.DestinationAddress,
						"port", skoopOutput.DestinationPort)

					utils.SetCondition(&skoopCopy.Status.Conditions, metav1.Condition{
						Type:               "Faled",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "TestCompleted",
						Message:            fmt.Sprintf("Skoop test completed failed"),
					})
				}
			}
			if updateErr := r.Status().Update(ctx, skoopCopy); updateErr != nil {
				logger.Info(updateErr.Error(), "failed to update skoop status", "skoop", skoop.Name, "namespace", skoop.Namespace)
				return ctrl.Result{RequeueAfter: time.Second * 30}, nil
			}

		} else {
			logger.Info("Job is still running", "namespace", req.Namespace, "name", req.Name, "job", jobName,
				"successded", job.Status.Succeeded, "failed", job.Status.Failed, "backoffLimit", job.Spec.BackoffLimit)
			return ctrl.Result{}, nil
		}
	}

	if skoop.Spec.Schedule != "" {
		interval := 60
		if skoop.Spec.Schedule != "" {
			if intervalValue, err := strconv.Atoi(skoop.Spec.Schedule); err == nil && intervalValue > 0 {
				interval = intervalValue
			}
		}
		logger.Info("Scheduling next Skoop test", "interval", interval)
		return ctrl.Result{RequeueAfter: time.Second * time.Duration(interval)}, nil
	}

	logger.Info("Skoop test finished; no further execution will be scheduled.",
		"skoop", skoop.Name,
		"namespace", skoop.Namespace)
	return ctrl.Result{}, nil

}

func (r *SkoopReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testtoolsv1.Skoop{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

func (r *SkoopReconciler) cleanupResources(ctx context.Context, skoop *testtoolsv1.Skoop) error {
	logger := log.FromContext(ctx)
	logger.Info("Start to cleanup resources", "namespace", skoop.Namespace, "name", skoop.Name)
	return nil
}
