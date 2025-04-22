package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

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
)

const TcpPingFinalizerName = "tcpping.testtools.xiaoming.com/finalizer"

type TcpPingReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=tcppings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=tcppings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=tcppings/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *TcpPingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Start to reconcile TcpPing", "namespace", req.Namespace, "name", req.Name)

	startTime := time.Now()
	defer func() {
		logger.Info("End to reconcile TcpPing", "namespace", req.Namespace, "name", req.Name, "duration", time.Since(startTime).String())
	}()

	var tcpping testtoolsv1.TcpPing
	if err := r.Get(ctx, req.NamespacedName, &tcpping); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("The tcpping resource is not exist", "namespace", req.Namespace, "name", req.Name)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to retrive the tcpping resource", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, err
	}

	if !tcpping.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&tcpping, TcpPingFinalizerName) {
			logger.Info("The tcpping resource is being deleted", "namespace", req.Namespace, "name", req.Name)
			if err := r.cleanupResources(ctx, &tcpping); err != nil {
				logger.Error(err, "Failed to clean resource", "namespace", req.Namespace, "name", req.Name)
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&tcpping, TcpPingFinalizerName)
			if err := r.Update(ctx, &tcpping); err != nil {
				logger.Error(err, "Failed to remove finalizer", "namespace", req.Namespace, "name", req.Name)
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&tcpping, TcpPingFinalizerName) {
		controllerutil.AddFinalizer(&tcpping, TcpPingFinalizerName)
		if err := r.Update(ctx, &tcpping); err != nil {
			logger.Error(err, "Failed to add finalizer", "namespace", req.Namespace, "name", req.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	shouldExecute := false

	if tcpping.Spec.Schedule != "" {
		if tcpping.Status.LastExecutionTime != nil {
			intervalSeconds := 60
			if scheduleValue, err := strconv.Atoi(tcpping.Spec.Schedule); err == nil && scheduleValue > 0 {
				intervalSeconds = scheduleValue
			}

			nextExecutionTime := tcpping.Status.LastExecutionTime.Add(time.Duration(intervalSeconds) * time.Second)
			shouldExecute = time.Now().After(nextExecutionTime)

			if !shouldExecute {
				logger.Info("Skipping execution due It is not yet the scheduled time.",
					"namespace", req.Namespace, "name", req.Name,
					"lastExecutionTime", tcpping.Status.LastExecutionTime.Time,
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
		shouldExecute = tcpping.Status.LastExecutionTime == nil
	}

	if !shouldExecute {
		logger.Info("The tcpping resource does not need to be executed.",
			"namespace", req.Namespace, "name", req.Name,
			"lastExecutionTime", tcpping.Status.LastExecutionTime.Time,
			"currentTime", time.Now(),
			"schedule", tcpping.Spec.Schedule)
		return ctrl.Result{}, nil
	}

	tcppingCopy := tcpping.DeepCopy()
	now := metav1.NewTime(time.Now())
	tcppingCopy.Status.LastExecutionTime = &now

	if tcppingCopy.Status.QueryCount == 0 {
		tcppingCopy.Status.QueryCount = 1
	}

	jobName, err := utils.PrepareTcpPingJob(ctx, r.Client, &tcpping)
	if err != nil {
		logger.Error(err, "Failed to prepare tcpping job", "namespace", req.Namespace, "name", req.Name)
		tcppingCopy.Status.Status = "Failed"
		tcppingCopy.Status.FailureCount++
		tcppingCopy.Status.LastResult = fmt.Sprintf("Error preparing tcpping job: %v", err)

		utils.SetCondition(&tcppingCopy.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "JobPreparationFailed",
			Message:            fmt.Sprintf("Failed to prepare tcpping job: %v", err),
		})

		if updateErr := r.Status().Update(ctx, tcppingCopy); updateErr != nil {
			logger.Error(updateErr, "Failed to update TcpPing status after job preparation failure")
		}
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	tcppingArgs := utils.BuildTcpPingArgs(&tcpping)
	tcppingCopy.Status.ExecutedCommand = fmt.Sprintf("tcpping %s", strings.Join(tcppingArgs, " "))
	logger.Info("Setting executed command", "command", tcppingCopy.Status.ExecutedCommand)

	var job batchv1.Job
	if err = r.Client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: jobName}, &job); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("TcpPing job not found", "namespace", req.Namespace, "name", req.Name, "job", jobName)
			return ctrl.Result{}, err
		}
		logger.Error(err, "Failed to get job", "namespace", req.Namespace, "name", req.Name, "job", jobName)
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	} else {
		if job.Status.Succeeded > 0 {
			logger.Info("Job completed successfully", "namespace", req.Namespace, "name", req.Name, "job", jobName)

			jobOutput, err := utils.GetJobResults(ctx, r.Client, tcpping.Namespace, jobName)
			if err != nil {
				logger.Error(err, "Failed to get tcpping job results")
				tcppingCopy.Status.Status = "Failed"
				tcppingCopy.Status.FailureCount++
				tcppingCopy.Status.LastResult = fmt.Sprintf("Error getting tcpping job results: %v", err)

				if tcppingCopy.Status.TestReportName == "" {
					tcppingCopy.Status.TestReportName = utils.GenerateTestReportName("Tcpping", tcpping.Name)
				}

				utils.SetCondition(&tcppingCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "ResultRetrievalFailed",
					Message:            fmt.Sprintf("Failed to get tcpping job results: %v", err),
				})
			} else {
				tcppingOutput := utils.ParseTcpPingOutput(jobOutput, tcppingCopy)
				if tcppingOutput.Status == "SUCCESS" {
					tcppingCopy.Status.Status = "Succeeded"
					if tcppingCopy.Spec.Schedule == "" {
						tcppingCopy.Status.SuccessCount = 1
					} else {
						tcppingCopy.Status.SuccessCount++
					}
					tcppingCopy.Status.LastResult = jobOutput

					if tcppingCopy.Status.TestReportName == "" {
						tcppingCopy.Status.TestReportName = utils.GenerateTestReportName("Tcpping", tcpping.Name)
					}

					if tcppingCopy.Status.QueryCount == 0 {
						tcppingCopy.Status.QueryCount = 1
					}

					logger.Info("TcpPing test succeeded",
						"host", tcppingOutput.Host,
						"port", tcppingOutput.Port)

					utils.SetCondition(&tcppingCopy.Status.Conditions, metav1.Condition{
						Type:               "Succeeded",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "TestCompleted",
						Message:            fmt.Sprintf("Tcpping test completed successfully"),
					})
				} else {
					tcppingCopy.Status.Status = "Failed"
					if tcppingCopy.Spec.Schedule == "" {
						tcppingCopy.Status.FailureCount = 1
					} else {
						tcppingCopy.Status.FailureCount++
					}
					tcppingCopy.Status.LastResult = jobOutput

					if tcppingCopy.Status.TestReportName == "" {
						tcppingCopy.Status.TestReportName = utils.GenerateTestReportName("Tcpping", tcpping.Name)
					}

					if tcppingCopy.Status.QueryCount == 0 {
						tcppingCopy.Status.QueryCount = 1
					}

					logger.Info("TcpPing test failed",
						"host", tcppingOutput.Host,
						"port", tcppingOutput.Port)

					utils.SetCondition(&tcppingCopy.Status.Conditions, metav1.Condition{
						Type:               "Failed",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "TestCompleted",
						Message:            fmt.Sprintf("Tcpping test completed failed"),
					})
				}

			}

			if updateErr := r.Status().Update(ctx, tcppingCopy); updateErr != nil {
				logger.Info(updateErr.Error(), "failed to update tcpping status", "tcpping", tcpping.Name, "namespace", tcpping.Namespace)
				return ctrl.Result{RequeueAfter: time.Second * 30}, nil
			}

		} else if job.Status.Failed > *job.Spec.BackoffLimit {
			logger.Error(nil, "Job failed", "namespace", req.Namespace, "name", req.Name, "job", jobName,
				"failed", job.Status.Failed, "backoffLimit", job.Spec.BackoffLimit)

			jobOutput, err := utils.GetJobResults(ctx, r.Client, tcpping.Namespace, jobName)
			if err != nil {
				logger.Error(err, "Failed to get tcpping job results")
				tcppingCopy.Status.Status = "Failed"
				tcppingCopy.Status.FailureCount++
				tcppingCopy.Status.LastResult = fmt.Sprintf("Error getting tcpping job results: %v", err)

				if tcppingCopy.Status.TestReportName == "" {
					tcppingCopy.Status.TestReportName = utils.GenerateTestReportName("Tcpping", tcpping.Name)
				}

				utils.SetCondition(&tcppingCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "ResultRetrievalFailed",
					Message:            fmt.Sprintf("Failed to get tcpping job results: %v", err),
				})
			} else {
				tcppingOutput := utils.ParseTcpPingOutput(jobOutput, tcppingCopy)
				tcppingCopy.Status.Status = "Failed"
				if tcppingCopy.Spec.Schedule == "" {
					tcppingCopy.Status.FailureCount = 1
				} else {
					tcppingCopy.Status.FailureCount++
				}
				tcppingCopy.Status.LastResult = jobOutput

				if tcppingCopy.Status.TestReportName == "" {
					tcppingCopy.Status.TestReportName = utils.GenerateTestReportName("Tcpping", tcpping.Name)
				}

				if tcppingCopy.Status.QueryCount == 0 {
					tcppingCopy.Status.QueryCount = 1
				}

				logger.Info("TcpPing test failed",
					"host", tcppingOutput.Host,
					"port", tcppingOutput.Port)

				utils.SetCondition(&tcppingCopy.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "TestCompleted",
					Message:            fmt.Sprintf("Tcpping test completed failed"),
				})
			}

			if updateErr := r.Status().Update(ctx, tcppingCopy); updateErr != nil {
				logger.Info(updateErr.Error(), "failed to update tcpping status", "tcpping", tcpping.Name, "namespace", tcpping.Namespace)
				return ctrl.Result{RequeueAfter: time.Second * 30}, nil
			}

		} else {
			logger.Info("Job is still running", "namespace", req.Namespace, "name", req.Name, "job", jobName,
				"successded", job.Status.Succeeded, "failed", job.Status.Failed, "backoffLimit", job.Spec.BackoffLimit)
			return ctrl.Result{}, nil
		}
	}

	if tcpping.Spec.Schedule != "" {
		interval := 60
		if tcpping.Spec.Schedule != "" {
			if intervalValue, err := strconv.Atoi(tcpping.Spec.Schedule); err == nil && intervalValue > 0 {
				interval = intervalValue
			}
		}
		logger.Info("Scheduling next TcpPing test", "interval", interval)
		return ctrl.Result{RequeueAfter: time.Second * time.Duration(interval)}, nil
	}

	logger.Info("TcpPing test finished; no further execution will be scheduled.",
		"tcpping", tcpping.Name,
		"namespace", tcpping.Namespace)
	return ctrl.Result{}, nil

}

func (r *TcpPingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testtoolsv1.TcpPing{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

func (r *TcpPingReconciler) cleanupResources(ctx context.Context, tcpping *testtoolsv1.TcpPing) error {
	logger := log.FromContext(ctx)
	logger.Info("Start to cleanup resources", "namespace", tcpping.Namespace, "name", tcpping.Name)
	return nil
}
