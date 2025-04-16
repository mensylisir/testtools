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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/robfig/cron/v3"
	"github.com/xiaoming/testtools/pkg/utils"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
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
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *FioReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling Fio")

	// Fetch the Fio instance
	var fio testtoolsv1.Fio
	if err := r.Get(ctx, req.NamespacedName, &fio); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Return and don't requeue
			log.Info("Fio resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Fio")
		return ctrl.Result{}, err
	}

	// Check if execution interval has passed since last execution
	var requeueTime time.Duration
	shouldExecute := false

	now := metav1.Now()
	if fio.Status.LastExecutionTime != nil {
		// Calculate time since last execution
		lastExecutionTime := fio.Status.LastExecutionTime.Time
		timeSinceLastExecution := now.Time.Sub(lastExecutionTime)
		log.Info("Time since last execution", "timeSinceLastExecution", timeSinceLastExecution.String())

		// If schedule is set, determine next execution time
		if fio.Spec.Schedule != "" {
			schedule, err := cron.ParseStandard(fio.Spec.Schedule)
			if err != nil {
				log.Error(err, "Failed to parse schedule", "schedule", fio.Spec.Schedule)
				// Update status with error
				utils.SetCondition(&fio.Status.Conditions, metav1.Condition{
					Type:    "Failed",
					Status:  metav1.ConditionTrue,
					Reason:  "InvalidSchedule",
					Message: fmt.Sprintf("Invalid schedule: %s", err),
				})
				if err := r.Status().Update(ctx, &fio); err != nil {
					log.Error(err, "Failed to update Fio status with schedule error")
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			}

			nextExecution := schedule.Next(lastExecutionTime)
			timeToNextExecution := nextExecution.Sub(now.Time)
			log.Info("Next execution scheduled", "nextExecution", nextExecution, "timeToNextExecution", timeToNextExecution.String())

			if timeToNextExecution > 0 {
				requeueTime = timeToNextExecution
			} else {
				shouldExecute = true
			}
		}
	} else {
		// No previous execution, run now
		shouldExecute = true
	}

	// Execute FIO if necessary
	if shouldExecute {
		log.Info("Executing FIO test")

		// Prepare and execute FIO job with retry logic
		var jobStats *testtoolsv1.FioStats
		var execErr error
		maxRetries := 3
		retryDelay := 2 * time.Second

		for attempt := 1; attempt <= maxRetries; attempt++ {
			// Get the latest version of the Fio object to avoid conflicts
			if attempt > 1 {
				if err := r.Get(ctx, req.NamespacedName, &fio); err != nil {
					log.Error(err, "Failed to re-fetch Fio object for retry", "attempt", attempt)
					if errors.IsNotFound(err) {
						return ctrl.Result{}, nil
					}
					return ctrl.Result{}, err
				}
				log.Info("Retrying FIO execution", "attempt", attempt)
			}

			jobStats, execErr = r.executeFio(ctx, &fio)
			if execErr == nil {
				break
			}

			log.Error(execErr, "FIO execution failed", "attempt", attempt)
			if attempt < maxRetries {
				time.Sleep(retryDelay)
				retryDelay *= 2 // Exponential backoff
			}
		}

		// If all retries failed, update status and requeue
		if execErr != nil {
			// Get fresh copy before updating status
			var freshFio testtoolsv1.Fio
			if err := r.Get(ctx, req.NamespacedName, &freshFio); err != nil {
				log.Error(err, "Failed to get fresh Fio copy for status update after execution errors")
				return ctrl.Result{}, err
			}

			// Update status with error
			utils.SetCondition(&freshFio.Status.Conditions, metav1.Condition{
				Type:    "Failed",
				Status:  metav1.ConditionTrue,
				Reason:  "ExecutionFailed",
				Message: fmt.Sprintf("Failed while executing FIO: %s", execErr),
			})
			// Update FailureCount
			freshFio.Status.FailureCount++

			if err := r.Status().Update(ctx, &freshFio); err != nil {
				log.Error(err, "Failed to update Fio status with execution error")
				return ctrl.Result{}, err
			}
			// Requeue after a short delay to try again
			return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
		}

		// Get fresh copy before updating status to avoid conflicts
		var freshFio testtoolsv1.Fio
		if err := r.Get(ctx, req.NamespacedName, &freshFio); err != nil {
			log.Error(err, "Failed to get fresh Fio copy for status update")
			return ctrl.Result{}, err
		}

		// Update FIO status with results
		freshFio.Status.LastExecutionTime = &now
		freshFio.Status.SuccessCount++
		freshFio.Status.QueryCount++

		// Update stats from job results
		if jobStats != nil {
			freshFio.Status.Stats = *jobStats
		}

		// 设置 TestReportName，用于关联 TestReport
		if freshFio.Status.TestReportName == "" {
			freshFio.Status.TestReportName = fmt.Sprintf("fio-%s-report", freshFio.Name)
			log.Info("Set TestReportName for FIO resource", "TestReportName", freshFio.Status.TestReportName)
		}

		// Set success condition
		utils.SetCondition(&freshFio.Status.Conditions, metav1.Condition{
			Type:    "Succeeded",
			Status:  metav1.ConditionTrue,
			Reason:  "ExecutionSucceeded",
			Message: "FIO test executed successfully",
		})
		utils.SetCondition(&freshFio.Status.Conditions, metav1.Condition{
			Type:    "Failed",
			Status:  metav1.ConditionFalse,
			Reason:  "ExecutionSucceeded",
			Message: "FIO test executed successfully",
		})

		// Update status with retry logic
		updateErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			return r.Status().Update(ctx, &freshFio)
		})

		if updateErr != nil {
			log.Error(updateErr, "Failed to update Fio status after multiple retries")
			return ctrl.Result{}, updateErr
		}

		// Determine when to run next
		if freshFio.Spec.Schedule != "" {
			schedule, _ := cron.ParseStandard(freshFio.Spec.Schedule)
			nextExecution := schedule.Next(now.Time)
			requeueTime = nextExecution.Sub(now.Time)
			log.Info("Next scheduled execution", "time", nextExecution, "delay", requeueTime.String())
		}
	}

	if requeueTime > 0 {
		return ctrl.Result{RequeueAfter: requeueTime}, nil
	}
	return ctrl.Result{}, nil
}

// executeFio executes FIO tests and returns the results
func (r *FioReconciler) executeFio(ctx context.Context, fio *testtoolsv1.Fio) (*testtoolsv1.FioStats, error) {
	log := log.FromContext(ctx)

	// Create Job for FIO command
	jobName, err := utils.PrepareFioJob(ctx, r.Client, fio)
	if err != nil {
		log.Error(err, "Failed to prepare FIO job")
		return nil, fmt.Errorf("failed to prepare FIO job: %w", err)
	}

	// Wait for the job to complete with timeout
	log.Info("Waiting for FIO job to complete", "jobName", jobName)
	err = utils.WaitForJob(ctx, r.Client, fio.Namespace, jobName, 15*time.Minute)
	if err != nil {
		log.Error(err, "Failed while waiting for FIO job")
		return nil, fmt.Errorf("failed while waiting for FIO job: %w", err)
	}

	// Get job to find its Pod
	var job batchv1.Job
	err = r.Get(ctx, types.NamespacedName{Namespace: fio.Namespace, Name: jobName}, &job)
	if err != nil {
		log.Error(err, "Failed to get job after completion")
		return nil, fmt.Errorf("failed to get job after completion: %w", err)
	}

	// Find the pod for this job
	pods := &v1.PodList{}
	if err := r.List(ctx, pods,
		client.InNamespace(fio.Namespace),
		client.MatchingLabels{"job-name": jobName}); err != nil {
		log.Error(err, "Failed to list pods for job")
		return nil, fmt.Errorf("failed to list pods for job: %w", err)
	}

	if len(pods.Items) == 0 {
		log.Error(nil, "No pods found for job")
		return nil, fmt.Errorf("no pods found for job %s", jobName)
	}

	// Use the first pod
	jobPod := pods.Items[0]

	// Get results from completed job
	results, err := utils.GetJobResults(ctx, r.Client, jobPod.Namespace, jobPod.Name)
	if err != nil {
		log.Error(err, "Failed to get job results")
		return nil, fmt.Errorf("failed to get job results: %w", err)
	}

	// Parse FIO results
	fioStats, err := utils.ParseFioOutput(results)
	if err != nil {
		log.Error(err, "Failed to parse FIO output")
		return nil, fmt.Errorf("failed to parse FIO output: %w", err)
	}

	log.Info("FIO test completed successfully",
		"readIOPS", fioStats.ReadIOPS,
		"writeIOPS", fioStats.WriteIOPS,
		"readBW", fioStats.ReadBW,
		"writeBW", fioStats.WriteBW)

	// Create a copy to return as pointer
	statsCopy := fioStats
	return &statsCopy, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FioReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testtoolsv1.Fio{}).
		Complete(r)
}
