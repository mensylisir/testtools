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
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
)

// TestReportReconciler reconciles a TestReport object
type TestReportReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=testreports,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=testreports/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=testreports/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *TestReportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling TestReport", "namespace", req.Namespace, "name", req.Name)

	// Fetch the TestReport instance
	var testReport testtoolsv1.TestReport
	if err := r.Get(ctx, req.NamespacedName, &testReport); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Initialize the report status if it hasn't been initialized yet
	if testReport.Status.Phase == "" {
		if err := r.initializeTestReport(ctx, &testReport, logger); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Check if the test report is already completed
	if testReport.Status.Phase == "Completed" {
		logger.Info("TestReport already completed", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, nil
	}

	// Collect results from resources
	if err := r.collectResults(ctx, &testReport, logger); err != nil {
		logger.Error(err, "Failed to collect test results")
		return ctrl.Result{}, err
	}

	// Update the test report status
	if err := r.updateTestReport(ctx, &testReport, logger); err != nil {
		logger.Error(err, "Failed to update TestReport status")
		return ctrl.Result{}, err
	}

	// If we need to continue collecting results (based on test duration and interval)
	if !r.isTestCompleted(&testReport) {
		interval := time.Duration(testReport.Spec.Interval) * time.Second
		if interval == 0 {
			interval = 60 * time.Second // Default to 60 seconds
		}
		logger.Info("Scheduling next reconciliation", "interval", interval)
		return ctrl.Result{RequeueAfter: interval}, nil
	}

	// Mark the test as completed
	return ctrl.Result{}, r.completeTestReport(ctx, &testReport, logger)
}

// initializeTestReport initializes a new test report
func (r *TestReportReconciler) initializeTestReport(ctx context.Context, testReport *testtoolsv1.TestReport, logger logr.Logger) error {
	logger.Info("Initializing TestReport", "namespace", testReport.Namespace, "name", testReport.Name)

	// Set the initial status
	now := metav1.Now()
	testReport.Status.Phase = "Running"
	testReport.Status.StartTime = &now

	// Initialize summary metrics
	testReport.Status.Summary = testtoolsv1.TestSummary{
		Total:     0,
		Succeeded: 0,
		Failed:    0,
	}

	// Add initial condition
	setTestReportCondition(testReport, metav1.Condition{
		Type:               "Running",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: now,
		Reason:             "TestStarted",
		Message:            "Test report collection started",
	})

	// Update the TestReport status
	return r.Status().Update(ctx, testReport)
}

// collectResults collects test results from the selected resources
func (r *TestReportReconciler) collectResults(ctx context.Context, testReport *testtoolsv1.TestReport, logger logr.Logger) error {
	logger.Info("Collecting test results", "namespace", testReport.Namespace, "name", testReport.Name)

	// If there are no resource selectors, nothing to collect
	if len(testReport.Spec.ResourceSelectors) == 0 {
		return nil
	}

	// Process each resource selector
	for _, selector := range testReport.Spec.ResourceSelectors {
		// Currently only supporting Dig resources
		if selector.Kind == "Dig" && selector.APIVersion == "testtools.xiaoming.com/v1" {
			if err := r.collectDigResults(ctx, testReport, selector, logger); err != nil {
				logger.Error(err, "Failed to collect Dig results", "selector", selector)
				continue
			}
		}
	}

	return nil
}

// collectDigResults collects results from Dig resources
func (r *TestReportReconciler) collectDigResults(ctx context.Context, testReport *testtoolsv1.TestReport, selector testtoolsv1.ResourceSelector, logger logr.Logger) error {
	namespace := selector.Namespace
	if namespace == "" {
		namespace = testReport.Namespace
	}

	// If a specific name is provided, get that specific Dig
	if selector.Name != "" {
		var dig testtoolsv1.Dig
		err := r.Get(ctx, types.NamespacedName{Name: selector.Name, Namespace: namespace}, &dig)
		if err != nil {
			return err
		}

		return r.addDigResult(testReport, &dig)
	}

	// Otherwise, list Digs based on label selector
	var digList testtoolsv1.DigList
	listOpts := []client.ListOption{client.InNamespace(namespace)}

	if selector.LabelSelector != nil && len(selector.LabelSelector.MatchLabels) > 0 {
		labels := client.MatchingLabels(selector.LabelSelector.MatchLabels)
		listOpts = append(listOpts, labels)
	}

	if err := r.List(ctx, &digList, listOpts...); err != nil {
		return err
	}

	// Process each Dig
	for i := range digList.Items {
		if err := r.addDigResult(testReport, &digList.Items[i]); err != nil {
			logger.Error(err, "Failed to add Dig result", "dig", digList.Items[i].Name)
		}
	}

	return nil
}

// addDigResult adds a Dig resource's result to the test report
func (r *TestReportReconciler) addDigResult(testReport *testtoolsv1.TestReport, dig *testtoolsv1.Dig) error {
	// Skip if there's no execution time
	if dig.Status.LastExecutionTime == nil {
		return nil
	}

	// Create a new test result
	result := testtoolsv1.TestResult{
		ResourceName:      dig.Name,
		ResourceNamespace: dig.Namespace,
		ResourceKind:      "Dig",
		ExecutionTime:     *dig.Status.LastExecutionTime,
		Success:           dig.Status.Status == "Succeeded",
		ResponseTime:      dig.Status.AverageResponseTime,
		Output:            extractDigSummary(dig.Status.LastResult),
		RawOutput:         dig.Status.LastResult,
	}

	// Extract metrics
	result.MetricValues = map[string]string{
		"QueryCount":   fmt.Sprintf("%d", dig.Status.QueryCount),
		"SuccessCount": fmt.Sprintf("%d", dig.Status.SuccessCount),
		"FailureCount": fmt.Sprintf("%d", dig.Status.FailureCount),
	}

	// 保留最新的10个结果
	maxResults := 10

	// 检查是否已有相同资源的结果
	existingIndex := -1
	for i, existing := range testReport.Status.Results {
		if existing.ResourceName == result.ResourceName &&
			existing.ResourceNamespace == result.ResourceNamespace &&
			existing.ResourceKind == result.ResourceKind {
			existingIndex = i
			break
		}
	}

	// 如果已有该资源的结果，则替换它
	if existingIndex >= 0 {
		testReport.Status.Results[existingIndex] = result
	} else {
		// 添加新结果
		testReport.Status.Results = append(testReport.Status.Results, result)

		// 如果结果超过最大限制，则删除最旧的结果
		if len(testReport.Status.Results) > maxResults {
			// 按执行时间排序，保留最新的
			sort.Slice(testReport.Status.Results, func(i, j int) bool {
				return testReport.Status.Results[i].ExecutionTime.Time.After(
					testReport.Status.Results[j].ExecutionTime.Time)
			})

			// 只保留最新的maxResults个结果
			testReport.Status.Results = testReport.Status.Results[:maxResults]
		}
	}

	return nil
}

// extractDigSummary extracts important information from the dig output
func extractDigSummary(digOutput string) string {
	if digOutput == "" {
		return "No dig output available"
	}

	var structuredOutput strings.Builder
	structuredOutput.WriteString("============ DNS查询结果 ============\n")

	// 提取查询信息
	commandLine := extractSection(digOutput, "; <<>> DiG", "\n")
	if commandLine != "" {
		structuredOutput.WriteString("命令: " + commandLine + "\n")
	}

	// 提取查询状态信息
	headerLine := extractSection(digOutput, ";; ->>HEADER<<-", "\n")
	if headerLine != "" {
		structuredOutput.WriteString("状态: " + headerLine + "\n")
	}

	// 提取问题部分
	questionSection := extractMultilineSection(digOutput, ";; QUESTION SECTION:", ";; ")
	if questionSection != "" {
		structuredOutput.WriteString("\n=== 查询内容 ===\n" + questionSection)
	}

	// 提取回答部分
	answerSection := extractMultilineSection(digOutput, ";; ANSWER SECTION:", ";; ")
	if answerSection != "" {
		structuredOutput.WriteString("\n=== 查询结果 ===\n" + answerSection)
	}

	// 提取查询时间和服务器信息
	queryTimeLine := extractSection(digOutput, ";; Query time:", "\n")
	if queryTimeLine != "" {
		structuredOutput.WriteString("\n=== 性能指标 ===\n" + queryTimeLine + "\n")
	}

	serverLine := extractSection(digOutput, ";; SERVER:", "\n")
	if serverLine != "" {
		structuredOutput.WriteString(serverLine + "\n")
	}

	whenLine := extractSection(digOutput, ";; WHEN:", "\n")
	if whenLine != "" {
		structuredOutput.WriteString(whenLine + "\n")
	}

	msgSizeLine := extractSection(digOutput, ";; MSG SIZE", "\n")
	if msgSizeLine != "" {
		structuredOutput.WriteString(msgSizeLine + "\n")
	}

	structuredOutput.WriteString("==================================\n")

	return structuredOutput.String()
}

// extractSection 从文本中提取指定起始标记之后，到指定结束标记之前的内容
func extractSection(text, startMarker, endMarker string) string {
	startIndex := strings.Index(text, startMarker)
	if startIndex == -1 {
		return ""
	}

	text = text[startIndex:]
	endIndex := strings.Index(text, endMarker)

	if endIndex == -1 {
		return text
	}

	if startMarker == endMarker {
		// 如果开始和结束标记相同，我们需要找到第二个出现的位置
		endIndex = strings.Index(text[1:], endMarker)
		if endIndex == -1 {
			return text
		}
		return text[:endIndex+1]
	}

	return text[:endIndex]
}

// extractMultilineSection 提取一个可能跨多行的区段
func extractMultilineSection(text, startMarker, nextSectionMarker string) string {
	startIndex := strings.Index(text, startMarker)
	if startIndex == -1 {
		return ""
	}

	// 移动到区段内容的开始
	startIndex += len(startMarker)
	remainingText := text[startIndex:]

	// 找到下一个区段的开始
	nextSectionIndex := strings.Index(remainingText, nextSectionMarker)
	if nextSectionIndex == -1 {
		// 如果没有下一个区段，返回剩余所有内容
		return remainingText
	}

	return remainingText[:nextSectionIndex]
}

// updateTestReport updates the test report status with summary metrics
func (r *TestReportReconciler) updateTestReport(ctx context.Context, testReport *testtoolsv1.TestReport, logger logr.Logger) error {
	// Calculate summary metrics
	var total, succeeded, failed int
	var totalResponseTime, minResponseTime, maxResponseTime float64

	// Initialize min response time to a high value
	minResponseTime = 999999.0

	for _, result := range testReport.Status.Results {
		total++
		if result.Success {
			succeeded++
		} else {
			failed++
		}

		if result.ResponseTime > 0 {
			totalResponseTime += result.ResponseTime
			if result.ResponseTime < minResponseTime {
				minResponseTime = result.ResponseTime
			}
			if result.ResponseTime > maxResponseTime {
				maxResponseTime = result.ResponseTime
			}
		}
	}

	// Update summary
	testReport.Status.Summary.Total = total
	testReport.Status.Summary.Succeeded = succeeded
	testReport.Status.Summary.Failed = failed

	// Calculate average response time if we have valid responses
	if total > 0 && totalResponseTime > 0 {
		testReport.Status.Summary.AverageResponseTime = totalResponseTime / float64(total)
		// Only set min/max if we have valid values
		if minResponseTime < 999999.0 {
			testReport.Status.Summary.MinResponseTime = minResponseTime
			testReport.Status.Summary.MaxResponseTime = maxResponseTime
		}
	}

	// Update the TestReport status
	return r.Status().Update(ctx, testReport)
}

// isTestCompleted checks if the test should be completed
func (r *TestReportReconciler) isTestCompleted(testReport *testtoolsv1.TestReport) bool {
	// If no test duration is specified, test runs indefinitely
	if testReport.Spec.TestDuration <= 0 {
		return false
	}

	// Check if we've exceeded the test duration
	if testReport.Status.StartTime != nil {
		testDuration := time.Duration(testReport.Spec.TestDuration) * time.Second
		timeElapsed := time.Since(testReport.Status.StartTime.Time)
		return timeElapsed >= testDuration
	}

	return false
}

// completeTestReport marks the test report as completed
func (r *TestReportReconciler) completeTestReport(ctx context.Context, testReport *testtoolsv1.TestReport, logger logr.Logger) error {
	logger.Info("Completing TestReport", "namespace", testReport.Namespace, "name", testReport.Name)

	// Set completion status
	now := metav1.Now()
	testReport.Status.Phase = "Completed"
	testReport.Status.CompletionTime = &now

	// Update condition
	setTestReportCondition(testReport, metav1.Condition{
		Type:               "Completed",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: now,
		Reason:             "TestCompleted",
		Message:            "Test report collection completed",
	})

	// Update the TestReport status
	return r.Status().Update(ctx, testReport)
}

// setTestReportCondition updates or adds a condition to the TestReport
func setTestReportCondition(testReport *testtoolsv1.TestReport, condition metav1.Condition) {
	// Find and update existing condition or add new one
	for i, cond := range testReport.Status.Conditions {
		if cond.Type == condition.Type {
			testReport.Status.Conditions[i] = condition
			return
		}
	}

	// Condition not found, add it
	testReport.Status.Conditions = append(testReport.Status.Conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *TestReportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testtoolsv1.TestReport{}).
		Complete(r)
}
