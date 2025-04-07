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
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
)

// DigReconciler reconciles a Dig object
type DigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=digs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=digs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=digs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Dig", "namespace", req.Namespace, "name", req.Name)

	// Fetch the Dig instance
	var dig testtoolsv1.Dig
	if err := r.Get(ctx, req.NamespacedName, &dig); err != nil {
		// We'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification).
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 预设TestReportName，确保在状态更新时能被保留
	if dig.Status.TestReportName == "" {
		reportName := fmt.Sprintf("dig-%s-report", dig.Name)
		dig.Status.TestReportName = reportName
		if err := r.Status().Update(ctx, &dig); err != nil {
			logger.Error(err, "Failed to update Dig status with TestReportName")
			return ctrl.Result{}, err
		}
		// 重新获取最新的Dig资源
		if err := r.Get(ctx, req.NamespacedName, &dig); err != nil {
			logger.Error(err, "Failed to re-fetch Dig after updating TestReportName")
			return ctrl.Result{}, err
		}
	}

	// Execute the dig command based on the CR spec
	result, err := r.executeDig(ctx, &dig, logger)

	// Update the status regardless of the command execution result
	if updateStatusErr := r.updateStatus(ctx, &dig, result, err); updateStatusErr != nil {
		logger.Error(updateStatusErr, "Failed to update Dig status")
		return ctrl.Result{}, updateStatusErr
	}

	// 检查是否需要创建或更新TestReport
	if err := r.ensureTestReport(ctx, &dig, logger); err != nil {
		logger.Error(err, "Failed to ensure TestReport")
	}

	// If there's a schedule, requeue based on that schedule
	if dig.Spec.Schedule != "" {
		// This is a simple implementation. For a more advanced cron-like schedule,
		// you would need to implement a proper scheduler or use a library.
		return ctrl.Result{RequeueAfter: time.Duration(60) * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// ensureTestReport ensures that a TestReport exists for the Dig resource
func (r *DigReconciler) ensureTestReport(ctx context.Context, dig *testtoolsv1.Dig, logger logr.Logger) error {
	// 构造TestReport名称，格式为"dig-{dig名称}-report"
	reportName := fmt.Sprintf("dig-%s-report", dig.Name)

	// 检查TestReport是否已经存在
	var existingReport testtoolsv1.TestReport
	err := r.Get(ctx, client.ObjectKey{Namespace: dig.Namespace, Name: reportName}, &existingReport)

	// 如果TestReport不存在，则创建一个新的
	if err != nil {
		logger.Info("Creating new TestReport for Dig", "dig", dig.Name, "report", reportName)

		// 构造新的TestReport对象
		newReport := testtoolsv1.TestReport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      reportName,
				Namespace: dig.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: dig.APIVersion,
						Kind:       dig.Kind,
						Name:       dig.Name,
						UID:        dig.UID,
						Controller: func() *bool { b := true; return &b }(),
					},
				},
			},
			Spec: testtoolsv1.TestReportSpec{
				TestName:     fmt.Sprintf("%s DNS Test", dig.Name),
				TestType:     "DNS",
				Target:       fmt.Sprintf("%s DNS查询", dig.Spec.Domain),
				TestDuration: 1800, // 默认30分钟
				Interval:     60,   // 默认每60秒收集一次结果
				ResourceSelectors: []testtoolsv1.ResourceSelector{
					{
						APIVersion: "testtools.xiaoming.com/v1",
						Kind:       "Dig",
						Name:       dig.Name,
						Namespace:  dig.Namespace,
					},
				},
			},
		}

		// 创建TestReport
		if err := r.Create(ctx, &newReport); err != nil {
			return err
		}

		// 更新Dig状态，添加关联的TestReport名称
		dig.Status.TestReportName = reportName
		return r.Status().Update(ctx, dig)
	}

	// TestReport已经存在，确保Dig状态包含TestReport名称
	if dig.Status.TestReportName != reportName {
		logger.Info("Updating Dig with TestReport reference", "dig", dig.Name, "report", reportName)
		dig.Status.TestReportName = reportName
		return r.Status().Update(ctx, dig)
	}

	// TestReport已经存在，不需要更新
	logger.Info("TestReport already exists for Dig", "dig", dig.Name, "report", reportName)
	return nil
}

// executeDig executes the dig command with the given parameters
func (r *DigReconciler) executeDig(ctx context.Context, dig *testtoolsv1.Dig, logger logr.Logger) (string, error) {
	start := time.Now()

	// Validate dig command parameters
	if dig.Spec.Domain == "" {
		logger.Error(fmt.Errorf("domain is required"), "Invalid Dig specification",
			"dig", dig.Name,
			"namespace", dig.Namespace)
		return "", fmt.Errorf("domain is required")
	}

	// Construct the dig command
	args := r.buildDigArgs(dig)
	logger.Info("Executing dig command",
		"args", args,
		"domain", dig.Spec.Domain,
		"server", dig.Spec.Server,
		"queryType", dig.Spec.QueryType,
		"timeout", dig.Spec.Timeout,
		"useTCP", dig.Spec.UseTCP,
		"useIPv4", dig.Spec.UseIPv4Only)

	// Create the command
	cmd := exec.Command("dig", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute the command
	logger.Info("Starting dig execution",
		"command", fmt.Sprintf("dig %s", strings.Join(args, " ")))
	err := cmd.Run()
	executionTime := time.Since(start)
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	logger.Info("Dig execution completed",
		"duration", executionTime,
		"exitCode", exitCode)

	// Process the result
	var result string
	if err != nil {
		stdoutStr := stdout.String()
		stderrStr := stderr.String()
		logger.Error(err, "Failed to execute dig command",
			"stderr", stderrStr,
			"stdout", stdoutStr,
			"exitCode", exitCode,
			"duration", executionTime,
			"args", args,
			"domain", dig.Spec.Domain,
			"server", dig.Spec.Server,
			"queryType", dig.Spec.QueryType,
			"currentStatus", dig.Status.Status,
			"queryCount", dig.Status.QueryCount,
			"successCount", dig.Status.SuccessCount,
			"failureCount", dig.Status.FailureCount)
		result = stderrStr
		if result == "" {
			result = stdoutStr
		}
		return result, fmt.Errorf("dig command failed: %w (exit code: %d)", err, exitCode)
	}

	result = stdout.String()
	resultLines := strings.Split(result, "\n")
	firstLine := ""
	if len(resultLines) > 0 {
		firstLine = resultLines[0]
	}
	logger.Info("Dig command executed successfully",
		"duration", executionTime,
		"outputLength", len(result),
		"firstLine", firstLine,
		"queryCount", dig.Status.QueryCount,
		"successCount", dig.Status.SuccessCount)
	return result, nil
}

// buildDigArgs constructs the arguments for the dig command based on the CR spec
func (r *DigReconciler) buildDigArgs(dig *testtoolsv1.Dig) []string {
	var args []string

	// Add the server if specified
	if dig.Spec.Server != "" {
		args = append(args, "@"+dig.Spec.Server)
	}

	// Add source IP and port if specified
	if dig.Spec.SourceIP != "" {
		sourceArg := "-b " + dig.Spec.SourceIP
		if dig.Spec.SourcePort > 0 {
			sourceArg += "#" + strconv.Itoa(int(dig.Spec.SourcePort))
		}
		args = append(args, sourceArg)
	}

	// Add query class if specified
	if dig.Spec.QueryClass != "" {
		args = append(args, "-c", dig.Spec.QueryClass)
	}

	// Add query file if specified
	if dig.Spec.QueryFile != "" {
		args = append(args, "-f", dig.Spec.QueryFile)
	}

	// Add key file if specified
	if dig.Spec.KeyFile != "" {
		args = append(args, "-k", dig.Spec.KeyFile)
	}

	// Add port if specified
	if dig.Spec.Port > 0 {
		args = append(args, "-p", strconv.Itoa(int(dig.Spec.Port)))
	}

	// Add query name if specified
	if dig.Spec.QueryName != "" {
		args = append(args, "-q", dig.Spec.QueryName)
	}

	// Add query type if specified
	if dig.Spec.QueryType != "" {
		args = append(args, "-t", dig.Spec.QueryType)
	}

	// Add microseconds flag if specified
	if dig.Spec.UseMicroseconds {
		args = append(args, "-u")
	}

	// Add reverse query if specified
	if dig.Spec.ReverseQuery != "" {
		args = append(args, "-x", dig.Spec.ReverseQuery)
	}

	// Add TSIG key if specified
	if dig.Spec.TSIGKey != "" {
		args = append(args, "-y", dig.Spec.TSIGKey)
	}

	// Add IPv4/IPv6 flags if specified
	if dig.Spec.UseIPv4Only {
		args = append(args, "-4")
	}
	if dig.Spec.UseIPv6Only {
		args = append(args, "-6")
	}

	// Add TCP flag if specified
	if dig.Spec.UseTCP {
		args = append(args, "+tcp")
	}

	// Add timeout if specified
	if dig.Spec.Timeout > 0 {
		args = append(args, "+time="+strconv.Itoa(int(dig.Spec.Timeout)))
	}

	// Add the domain (required)
	args = append(args, dig.Spec.Domain)

	return args
}

// updateStatus updates the status of the Dig CR
func (r *DigReconciler) updateStatus(ctx context.Context, dig *testtoolsv1.Dig, result string, execErr error) error {
	// Create a copy of the original for comparison
	originalDig := dig.DeepCopy()

	// 保存当前的TestReportName，确保不会丢失
	testReportName := dig.Status.TestReportName

	// Update execution time
	now := metav1.Now()
	dig.Status.LastExecutionTime = &now

	// Update query count
	dig.Status.QueryCount++

	// Update result
	dig.Status.LastResult = result

	// Update status based on execution error
	if execErr != nil {
		dig.Status.Status = "Failed"
		dig.Status.FailureCount++
		// Add or update the failure condition
		setCondition(&dig.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionFalse,
			LastTransitionTime: now,
			Reason:             "ExecutionFailed",
			Message:            execErr.Error(),
		})
	} else {
		dig.Status.Status = "Succeeded"
		dig.Status.SuccessCount++
		// Add or update the success condition
		setCondition(&dig.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "ExecutionSucceeded",
			Message:            "Dig command executed successfully",
		})

		// Parse response time if available in the result
		responseTimeMs := parseResponseTime(result)
		if responseTimeMs > 0 {
			// Update average response time
			if dig.Status.AverageResponseTime == 0 {
				dig.Status.AverageResponseTime = responseTimeMs
			} else {
				// Calculate running average
				totalQueries := float64(dig.Status.SuccessCount)
				dig.Status.AverageResponseTime = ((dig.Status.AverageResponseTime * (totalQueries - 1)) + responseTimeMs) / totalQueries
			}
		}
	}

	// 确保TestReportName不会丢失
	if testReportName != "" {
		dig.Status.TestReportName = testReportName
	}

	// Update the status
	if !statusEqual(originalDig.Status, dig.Status) {
		return r.Status().Update(ctx, dig)
	}

	return nil
}

// parseResponseTime parses the query time from dig output
func parseResponseTime(output string) float64 {
	// Sample dig output line: ";; Query time: 23 msec"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Query time") {
			parts := strings.Split(line, ":")
			if len(parts) < 2 {
				continue
			}
			timePart := strings.TrimSpace(parts[1])
			msecParts := strings.Split(timePart, " ")
			if len(msecParts) < 2 {
				continue
			}
			if val, err := strconv.ParseFloat(msecParts[0], 64); err == nil {
				return val
			}
		}
	}
	return 0
}

// setCondition sets the given condition in the condition list
func setCondition(conditions *[]metav1.Condition, newCondition metav1.Condition) {
	// If conditions is nil, create a new slice
	if *conditions == nil {
		*conditions = []metav1.Condition{}
	}

	// Find the condition
	var existingCondition *metav1.Condition
	for i := range *conditions {
		if (*conditions)[i].Type == newCondition.Type {
			existingCondition = &(*conditions)[i]
			break
		}
	}

	// If condition doesn't exist, add it
	if existingCondition == nil {
		*conditions = append(*conditions, newCondition)
		return
	}

	// Update existing condition
	if existingCondition.Status != newCondition.Status {
		existingCondition.LastTransitionTime = newCondition.LastTransitionTime
	}
	existingCondition.Status = newCondition.Status
	existingCondition.Reason = newCondition.Reason
	existingCondition.Message = newCondition.Message
}

// statusEqual checks if two status objects are equal
func statusEqual(a, b testtoolsv1.DigStatus) bool {
	// This is a simple implementation for demonstration
	// In a real implementation, you would compare all relevant fields
	return a.QueryCount == b.QueryCount &&
		a.Status == b.Status &&
		a.SuccessCount == b.SuccessCount &&
		a.FailureCount == b.FailureCount &&
		a.AverageResponseTime == b.AverageResponseTime &&
		a.TestReportName == b.TestReportName
}

// SetupWithManager sets up the controller with the Manager.
func (r *DigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testtoolsv1.Dig{}).
		Complete(r)
}
