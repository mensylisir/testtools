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
	"github.com/xiaoming/testtools/pkg/utils"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TestReportReconciler 管理 TestReport 资源的调谐过程
type TestReportReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// 添加事件记录器
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=testreports,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=testreports/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=testreports/finalizers,verbs=update
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=fios,verbs=get;list;watch
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=fios/status,verbs=get
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=digs,verbs=get;list;watch
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=digs/status,verbs=get
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=pings,verbs=get;list;watch
// +kubebuilder:rbac:groups=testtools.xiaoming.com,resources=pings/status,verbs=get

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

	// 添加节流逻辑：使用CompletionTime字段来限制处理频率
	if testReport.Status.StartTime != nil {
		// 默认最小间隔为30秒
		minInterval := 30 * time.Second

		// 计算距离上次更新的时间（使用StartTime作为参考点）
		elapsed := time.Since(testReport.Status.StartTime.Time)

		// 如果最后一次结果更新的时间可用，使用它
		if len(testReport.Status.Results) > 0 {
			lastResult := testReport.Status.Results[len(testReport.Status.Results)-1]
			elapsed = time.Since(lastResult.ExecutionTime.Time)
		}

		if elapsed < minInterval {
			nextRunDelay := minInterval - elapsed
			logger.V(1).Info("Too soon to process report again, scheduling future reconcile",
				"report", testReport.Name,
				"namespace", testReport.Namespace,
				"elapsed", elapsed,
				"nextRunIn", nextRunDelay)
			return ctrl.Result{RequeueAfter: nextRunDelay}, nil
		}
	}

	// Initialize the report status if it hasn't been initialized yet
	if testReport.Status.Phase == "" {
		if err := r.initializeTestReport(ctx, &testReport, logger); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Check if the test report is already completed
	if testReport.Status.Phase == "Completed" {
		logger.V(1).Info("TestReport already completed", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, nil
	}

	// 保存原始状态，用于后续比较是否有变化
	originalStatus := testReport.Status.DeepCopy()

	// Collect results from resources
	if err := r.collectResults(ctx, &testReport, logger); err != nil {
		logger.Error(err, "Failed to collect test results")
		return ctrl.Result{}, err
	}

	// 检查状态是否有实质性变化
	statusChanged := !r.isTestReportStatusEqual(originalStatus, &testReport.Status)

	// 仅在状态有变化时更新TestReport
	if statusChanged {
		logger.Info("TestReport status changed, updating", "namespace", req.Namespace, "name", req.Name)
		if err := r.updateTestReport(ctx, &testReport, logger); err != nil {
			logger.Error(err, "Failed to update TestReport status")
			return ctrl.Result{}, err
		}
	} else {
		logger.V(1).Info("No significant changes in TestReport, skipping update",
			"namespace", req.Namespace, "name", req.Name)
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
	logger.V(1).Info("Collecting test results", "namespace", testReport.Namespace, "name", testReport.Name)

	// If there are no resource selectors, nothing to collect
	if len(testReport.Spec.ResourceSelectors) == 0 {
		return nil
	}

	// 检查所有资源都已完成，用于判断是否应该将报告标记为完成
	allResourcesCompleted := true

	// Process each resource selector
	for _, selector := range testReport.Spec.ResourceSelectors {
		// 处理 Dig 资源
		if selector.Kind == "Dig" && selector.APIVersion == "testtools.xiaoming.com/v1" {
			completed, err := r.collectDigResults(ctx, testReport, selector, logger)
			if err != nil {
				logger.Error(err, "Failed to collect Dig results", "selector", selector)
				allResourcesCompleted = false
				continue
			}
			if !completed {
				allResourcesCompleted = false
			}
		}

		// 处理 Fio 资源
		if selector.Kind == "Fio" && selector.APIVersion == "testtools.xiaoming.com/v1" {
			completed, err := r.collectFioResults(ctx, testReport, selector, logger)
			if err != nil {
				logger.Error(err, "Failed to collect Fio results", "selector", selector)
				allResourcesCompleted = false
				continue
			}
			if !completed {
				allResourcesCompleted = false
			}
		}

		// 处理 Ping 资源
		if selector.Kind == "Ping" && selector.APIVersion == "testtools.xiaoming.com/v1" {
			completed, err := r.collectPingResults(ctx, testReport, selector, logger)
			if err != nil {
				logger.Error(err, "Failed to collect Ping results", "selector", selector)
				allResourcesCompleted = false
				continue
			}
			if !completed {
				allResourcesCompleted = false
			}
		}

		// 处理 Nc 资源
		if selector.Kind == "Nc" && selector.APIVersion == "testtools.xiaoming.com/v1" {
			completed, err := r.collectNcResults(ctx, testReport, selector, logger)
			if err != nil {
				logger.Error(err, "Failed to collect Nc results", "selector", selector)
				allResourcesCompleted = false
				continue
			}
			if !completed {
				allResourcesCompleted = false
			}
		}

		// 处理 TcpPing 资源
		if selector.Kind == "TcpPing" && selector.APIVersion == "testtools.xiaoming.com/v1" {
			completed, err := r.collectTcpPingResults(ctx, testReport, selector, logger)
			if err != nil {
				logger.Error(err, "Failed to collect TcpPing results", "selector", selector)
				allResourcesCompleted = false
				continue
			}
			if !completed {
				allResourcesCompleted = false
			}
		}

		// 处理 Iperf 资源
		if selector.Kind == "Iperf" && selector.APIVersion == "testtools.xiaoming.com/v1" {
			completed, err := r.collectIperfResults(ctx, testReport, selector, logger)
			if err != nil {
				logger.Error(err, "Failed to collect Iperf results", "selector", selector)
				allResourcesCompleted = false
				continue
			}
			if !completed {
				allResourcesCompleted = false
			}
		}

		// 处理 Skoop 资源
		if selector.Kind == "Skoop" && selector.APIVersion == "testtools.xiaoming.com/v1" {
			completed, err := r.collectSkoopResults(ctx, testReport, selector, logger)
			if err != nil {
				logger.Error(err, "Failed to collect Skoop results", "selector", selector)
				allResourcesCompleted = false
				continue
			}
			if !completed {
				allResourcesCompleted = false
			}
		}

		// 可以在这里添加对其他资源类型的支持
	}

	// 如果所有资源都已完成，将报告标记为完成
	if allResourcesCompleted && testReport.Status.Phase != "Completed" {
		logger.Info("All resources are completed, marking TestReport as completed",
			"namespace", testReport.Namespace, "name", testReport.Name)
		if err := r.completeTestReport(ctx, testReport, logger); err != nil {
			logger.Error(err, "Failed to mark TestReport as completed",
				"namespace", testReport.Namespace, "name", testReport.Name)
			return err
		}
	}

	return nil
}

// collectDigResults 收集 Dig 资源的结果
func (r *TestReportReconciler) collectDigResults(ctx context.Context, testReport *testtoolsv1.TestReport, selector testtoolsv1.ResourceSelector, logger logr.Logger) (bool, error) {
	dig := &testtoolsv1.Dig{}
	err := r.Get(ctx, types.NamespacedName{Name: selector.Name, Namespace: selector.Namespace}, dig)
	if err != nil {
		if errors.IsNotFound(err) {
			// 资源已被删除，记录信息并继续
			logger.Info("Dig资源不存在，可能已被删除",
				"name", selector.Name,
				"namespace", selector.Namespace)

			// 返回false但不报错，表示此资源不可用但不阻止其他资源的处理
			return false, nil
		}
		logger.Error(err, "获取Dig资源失败",
			"name", selector.Name,
			"namespace", selector.Namespace)
		return false, err
	}

	// 检查最后执行时间，避免重复添加结果
	if dig.Status.LastExecutionTime == nil {
		logger.Info("Dig测试尚未执行",
			"dig", dig.Name,
			"namespace", dig.Namespace)
		return false, nil
	}

	// 检查是否已包含此结果
	for _, result := range testReport.Status.Results {
		if result.ResourceName == dig.Name &&
			result.ResourceNamespace == dig.Namespace &&
			result.ResourceKind == "Dig" &&
			result.ExecutionTime.Equal(dig.Status.LastExecutionTime) {
			// 已存在此结果
			logger.Info("Dig测试结果已存在",
				"dig", dig.Name,
				"executionTime", dig.Status.LastExecutionTime)
			return false, nil
		}
	}

	// 使用事件记录关联关系
	r.Recorder.Event(dig, corev1.EventTypeNormal, "CollectedByTestReport",
		fmt.Sprintf("结果被测试报告 %s 收集", testReport.Name))

	// 添加结果
	err = r.addDigResult(testReport, dig)
	if err != nil {
		logger.Error(err, "添加Dig结果失败",
			"dig", dig.Name,
			"namespace", dig.Namespace)
		return false, err
	}

	logger.Info("成功添加Dig测试结果",
		"dig", dig.Name,
		"namespace", dig.Namespace,
		"executionTime", dig.Status.LastExecutionTime)
	return true, nil
}

// collectFioResults 收集 Fio 资源的结果
func (r *TestReportReconciler) collectFioResults(ctx context.Context, testReport *testtoolsv1.TestReport, selector testtoolsv1.ResourceSelector, logger logr.Logger) (bool, error) {
	// 获取Fio资源
	fio := &testtoolsv1.Fio{}
	err := r.Get(ctx, types.NamespacedName{Name: selector.Name, Namespace: selector.Namespace}, fio)
	if err != nil {
		if errors.IsNotFound(err) {
			// 资源已被删除，记录信息并继续
			logger.Info("Fio资源不存在，可能已被删除",
				"name", selector.Name,
				"namespace", selector.Namespace)

			// 返回false但不报错，表示此资源不可用但不阻止其他资源的处理
			return false, nil
		}
		logger.Error(err, "获取Fio资源失败",
			"name", selector.Name,
			"namespace", selector.Namespace)
		return false, err
	}

	// 检查最后执行时间，避免重复添加结果
	if fio.Status.LastExecutionTime == nil {
		logger.Info("Fio测试尚未执行",
			"fio", fio.Name,
			"namespace", fio.Namespace)
		return false, nil
	}

	// 检查是否已包含此结果
	for _, result := range testReport.Status.Results {
		if result.ResourceName == fio.Name &&
			result.ResourceNamespace == fio.Namespace &&
			result.ResourceKind == "Fio" &&
			result.ExecutionTime.Equal(fio.Status.LastExecutionTime) {
			// 已存在此结果
			logger.Info("Fio测试结果已存在",
				"fio", fio.Name,
				"executionTime", fio.Status.LastExecutionTime)
			return false, nil
		}
	}

	// 使用事件记录关联关系
	r.Recorder.Event(fio, corev1.EventTypeNormal, "CollectedByTestReport",
		fmt.Sprintf("结果被测试报告 %s 收集", testReport.Name))

	// 添加结果
	err = r.addFioResult(testReport, fio)
	if err != nil {
		logger.Error(err, "添加Fio结果失败",
			"fio", fio.Name,
			"namespace", fio.Namespace)
		return false, err
	}

	logger.Info("成功添加Fio测试结果",
		"fio", fio.Name,
		"namespace", fio.Namespace,
		"executionTime", fio.Status.LastExecutionTime)
	return true, nil
}

// collectPingResults 收集 Ping 资源的结果
func (r *TestReportReconciler) collectPingResults(ctx context.Context, testReport *testtoolsv1.TestReport, selector testtoolsv1.ResourceSelector, logger logr.Logger) (bool, error) {
	logger.Info("开始收集Ping结果", "selector", selector)
	namespace := selector.Namespace
	if namespace == "" {
		namespace = testReport.Namespace
	}

	// 如果提供了特定名称，获取该特定的 Ping 资源
	if selector.Name != "" {
		var ping testtoolsv1.Ping
		err := r.Get(ctx, types.NamespacedName{Name: selector.Name, Namespace: namespace}, &ping)
		if err != nil {
			logger.Error(err, "获取指定的Ping资源失败",
				"name", selector.Name,
				"namespace", namespace)
			return false, err
		}

		logger.Info("找到指定的Ping资源",
			"name", ping.Name,
			"namespace", ping.Namespace,
			"lastExecutionTime", ping.Status.LastExecutionTime)

		// 使用事件记录关联关系
		r.Recorder.Event(&ping, corev1.EventTypeNormal, "CollectedByTestReport",
			fmt.Sprintf("结果被测试报告 %s 收集", testReport.Name))

		// 添加结果到测试报告
		if err := r.addPingResult(testReport, &ping); err != nil {
			logger.Error(err, "添加Ping结果失败",
				"name", ping.Name,
				"namespace", ping.Namespace)
			return false, err
		}

		logger.Info("成功添加Ping结果",
			"name", ping.Name,
			"namespace", ping.Namespace,
			"resultsCount", len(testReport.Status.Results))
		return true, nil
	}

	// 否则，根据标签选择器列出 Ping 资源
	var pingList testtoolsv1.PingList
	listOpts := []client.ListOption{client.InNamespace(namespace)}

	if selector.LabelSelector != nil && len(selector.LabelSelector.MatchLabels) > 0 {
		labels := client.MatchingLabels(selector.LabelSelector.MatchLabels)
		listOpts = append(listOpts, labels)
	}

	if err := r.List(ctx, &pingList, listOpts...); err != nil {
		logger.Error(err, "获取Ping资源列表失败",
			"namespace", namespace,
			"labelSelector", selector.LabelSelector)
		return false, err
	}

	logger.Info("找到Ping资源列表", "count", len(pingList.Items))

	// 处理每个 Ping 资源
	for i := range pingList.Items {
		ping := &pingList.Items[i]

		// 使用事件记录关联关系
		r.Recorder.Event(ping, corev1.EventTypeNormal, "CollectedByTestReport",
			fmt.Sprintf("结果被测试报告 %s 收集", testReport.Name))

		if err := r.addPingResult(testReport, ping); err != nil {
			logger.Error(err, "添加Ping结果失败", "ping", ping.Name)
		} else {
			logger.Info("成功添加Ping结果",
				"name", ping.Name,
				"namespace", ping.Namespace)
		}
	}

	logger.Info("Ping结果收集完成", "resultsCount", len(testReport.Status.Results))
	return true, nil
}

// collectNcResults 收集 Nc 资源的结果
func (r *TestReportReconciler) collectNcResults(ctx context.Context, testReport *testtoolsv1.TestReport, selector testtoolsv1.ResourceSelector, logger logr.Logger) (bool, error) {
	nc := &testtoolsv1.Nc{}
	err := r.Get(ctx, types.NamespacedName{Name: selector.Name, Namespace: selector.Namespace}, nc)
	if err != nil {
		if errors.IsNotFound(err) {
			// 资源已被删除，记录信息并继续
			logger.Info("Nc资源不存在，可能已被删除",
				"name", selector.Name,
				"namespace", selector.Namespace)

			// 返回false但不报错，表示此资源不可用但不阻止其他资源的处理
			return false, nil
		}
		logger.Error(err, "获取Nc资源失败",
			"name", selector.Name,
			"namespace", selector.Namespace)
		return false, err
	}

	// 检查最后执行时间，避免重复添加结果
	if nc.Status.LastExecutionTime == nil {
		logger.Info("Nc测试尚未执行",
			"nc", nc.Name,
			"namespace", nc.Namespace)
		return false, nil
	}

	// 检查是否已包含此结果
	for _, result := range testReport.Status.Results {
		if result.ResourceName == nc.Name &&
			result.ResourceNamespace == nc.Namespace &&
			result.ResourceKind == "Nc" &&
			result.ExecutionTime.Equal(nc.Status.LastExecutionTime) {
			// 已存在此结果
			logger.Info("Nc测试结果已存在",
				"nc", nc.Name,
				"executionTime", nc.Status.LastExecutionTime)
			return true, nil
		}
	}

	// 使用事件记录关联关系
	r.Recorder.Event(nc, corev1.EventTypeNormal, "CollectedByTestReport",
		fmt.Sprintf("结果被测试报告 %s 收集", testReport.Name))

	// 添加结果
	err = r.addNcResult(testReport, nc)
	if err != nil {
		logger.Error(err, "添加Nc结果失败",
			"nc", nc.Name,
			"namespace", nc.Namespace)
		return false, err
	}

	logger.Info("成功添加Nc测试结果",
		"nc", nc.Name,
		"namespace", nc.Namespace,
		"executionTime", nc.Status.LastExecutionTime)
	return true, nil
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

	// 记录添加的结果详情
	log.FromContext(context.Background()).Info("添加Dig测试结果到报告",
		"report", testReport.Name,
		"dig", dig.Name,
		"status", dig.Status.Status,
		"queryTime", dig.Status.AverageResponseTime)

	// 查找是否存在相同资源的结果
	found := false
	for i, existing := range testReport.Status.Results {
		if existing.ResourceName == result.ResourceName &&
			existing.ResourceNamespace == result.ResourceNamespace &&
			existing.ResourceKind == result.ResourceKind {
			// 替换现有结果
			testReport.Status.Results[i] = result
			found = true
			break
		}
	}

	// 如果没有找到现有结果，添加新结果
	if !found {
		testReport.Status.Results = append(testReport.Status.Results, result)
	}

	// 更新测试报告摘要
	testReport.Status.Summary.Total = len(testReport.Status.Results)
	testReport.Status.Summary.Metrics = result.MetricValues

	// 计算成功和失败数量
	succeeded := 0
	failed := 0
	for _, r := range testReport.Status.Results {
		if r.Success {
			succeeded++
		} else {
			failed++
		}
	}
	testReport.Status.Summary.Succeeded = succeeded
	testReport.Status.Summary.Failed = failed

	return nil
}

// extractDigSummary 提取 Dig 输出的摘要信息
func extractDigSummary(digOutput string) string {
	if digOutput == "" {
		return "No output available"
	}

	// 创建一个结构化的输出
	var structuredOutput strings.Builder

	// 提取查询信息
	domainLine := ""
	serverLine := ""
	statusLine := ""
	timeLine := ""

	lines := strings.Split(digOutput, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "Query time:") {
			timeLine = line
		} else if strings.Contains(line, "SERVER:") {
			serverLine = line
		} else if strings.Contains(line, "status:") {
			statusLine = line
		} else if strings.Contains(line, "<<>") && strings.Contains(line, "IN") {
			domainLine = line
		}
	}

	// 构建结构化输出
	if domainLine != "" {
		structuredOutput.WriteString("查询: " + domainLine + "\n")
	}
	if serverLine != "" {
		structuredOutput.WriteString("服务器: " + serverLine + "\n")
	}
	if statusLine != "" {
		structuredOutput.WriteString("状态: " + statusLine + "\n")
	}
	if timeLine != "" {
		structuredOutput.WriteString("查询时间: " + timeLine + "\n")
	}

	// 提取应答部分
	answerSection := false
	answerLines := []string{}

	for _, line := range lines {
		if strings.Contains(line, "ANSWER SECTION:") {
			answerSection = true
			continue
		} else if answerSection && strings.Contains(line, "SECTION:") {
			break
		} else if answerSection && strings.TrimSpace(line) != "" {
			answerLines = append(answerLines, line)
		}
	}

	// 添加应答部分
	if len(answerLines) > 0 {
		structuredOutput.WriteString("\n应答:\n")
		for _, line := range answerLines {
			structuredOutput.WriteString(line + "\n")
		}
	}

	// 如果结构化输出为空，返回原始输出的截断版本
	if structuredOutput.Len() == 0 {
		if len(digOutput) > 300 {
			return digOutput[:300] + "... (截断)"
		}
		return digOutput
	}

	return structuredOutput.String()
}

// extractFirstLineContaining 从文本中提取包含特定子串的第一行
func extractFirstLineContaining(text string, substring string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if strings.Contains(line, substring) {
			return line
		}
	}
	return ""
}

// addPingResult adds a Ping resource's result to the test report
func (r *TestReportReconciler) addPingResult(testReport *testtoolsv1.TestReport, ping *testtoolsv1.Ping) error {
	// 如果已经有结果了，避免重复添加
	for _, result := range testReport.Status.Results {
		if result.ResourceKind == "Ping" && result.ResourceName == ping.Name && result.ResourceNamespace == ping.Namespace {
			return nil
		}
	}

	// 创建新的测试结果
	result := testtoolsv1.TestResult{
		ResourceKind:      "Ping",
		ResourceName:      ping.Name,
		ResourceNamespace: ping.Namespace,
		ExecutionTime:     *ping.Status.LastExecutionTime,
		Success:           ping.Status.Status == "Succeeded",
		Output:            extractPingSummary(ping.Status.LastResult),
		RawOutput:         ping.Status.LastResult,
	}

	// 设置指标值
	result.MetricValues = make(map[string]string)

	// 添加响应时间指标
	if ping.Status.PacketLoss != "" {
		// 尝试将字符串转换为float以便格式化
		packetLoss, err := strconv.ParseFloat(ping.Status.PacketLoss, 64)
		if err == nil {
			result.MetricValues["Packet Loss"] = fmt.Sprintf("%.1f%%", packetLoss)
		} else {
			result.MetricValues["Packet Loss"] = fmt.Sprintf("%s%%", ping.Status.PacketLoss)
		}
	}
	if ping.Status.MinRtt != "" {
		// 尝试将字符串转换为float以便格式化
		minRtt, err := strconv.ParseFloat(ping.Status.MinRtt, 64)
		if err == nil {
			result.MetricValues["Min RTT"] = fmt.Sprintf("%.2f ms", minRtt)
		} else {
			result.MetricValues["Min RTT"] = fmt.Sprintf("%s ms", ping.Status.MinRtt)
		}
		result.ResponseTime = ping.Status.MinRtt
	}
	if ping.Status.AvgRtt != "" {
		// 尝试将字符串转换为float以便格式化
		avgRtt, err := strconv.ParseFloat(ping.Status.AvgRtt, 64)
		if err == nil {
			result.MetricValues["Avg RTT"] = fmt.Sprintf("%.2f ms", avgRtt)
		} else {
			result.MetricValues["Avg RTT"] = fmt.Sprintf("%s ms", ping.Status.AvgRtt)
		}
		// 如果最小RTT未设置，则使用平均RTT作为响应时间
		if result.ResponseTime == "" {
			result.ResponseTime = ping.Status.AvgRtt
		}
	}
	if ping.Status.MaxRtt != "" {
		// 尝试将字符串转换为float以便格式化
		maxRtt, err := strconv.ParseFloat(ping.Status.MaxRtt, 64)
		if err == nil {
			result.MetricValues["Max RTT"] = fmt.Sprintf("%.2f ms", maxRtt)
		} else {
			result.MetricValues["Max RTT"] = fmt.Sprintf("%s ms", ping.Status.MaxRtt)
		}
	}
	result.MetricValues["Query Count"] = fmt.Sprintf("%d", ping.Status.QueryCount)
	result.MetricValues["Success Count"] = fmt.Sprintf("%d", ping.Status.SuccessCount)
	result.MetricValues["Failure Count"] = fmt.Sprintf("%d", ping.Status.FailureCount)

	// 添加结果到测试报告
	testReport.Status.Results = append(testReport.Status.Results, result)
	testReport.Status.Summary.Metrics = result.MetricValues

	// 更新摘要指标
	testReport.Status.Summary.Total++
	if result.Success {
		testReport.Status.Summary.Succeeded++
	} else {
		testReport.Status.Summary.Failed++
	}

	// 更新平均响应时间
	if len(result.ResponseTime) > 0 {
		if len(testReport.Status.Summary.AverageResponseTime) == 0 {
			testReport.Status.Summary.AverageResponseTime = result.ResponseTime
		} else {
			// 计算新的平均响应时间
			totalSucceeded := testReport.Status.Summary.Succeeded
			if totalSucceeded > 0 {
				averageResponseTime, _ := strconv.ParseFloat(testReport.Status.Summary.AverageResponseTime, 64)
				responseTime, _ := strconv.ParseFloat(result.ResponseTime, 64)
				testReport.Status.Summary.AverageResponseTime = fmt.Sprintf("%f", (averageResponseTime*float64(totalSucceeded-1)+responseTime)/float64(totalSucceeded))
			}
		}
	}

	// 更新最小和最大响应时间
	if len(result.ResponseTime) > 0 {
		if len(testReport.Status.Summary.MinResponseTime) == 0 || result.ResponseTime < testReport.Status.Summary.MinResponseTime {
			testReport.Status.Summary.MinResponseTime = result.ResponseTime
		}
		if result.ResponseTime > testReport.Status.Summary.MaxResponseTime {
			testReport.Status.Summary.MaxResponseTime = result.ResponseTime
		}
	}

	return nil
}

// addFioResult adds a Fio resource's result to the test report
func (r *TestReportReconciler) addFioResult(testReport *testtoolsv1.TestReport, fio *testtoolsv1.Fio) error {
	// Skip if there's no execution time
	if fio.Status.LastExecutionTime == nil {
		return nil
	}

	// Create a new test result
	result := testtoolsv1.TestResult{
		ResourceName:      fio.Name,
		ResourceNamespace: fio.Namespace,
		ResourceKind:      "Fio",
		ExecutionTime:     *fio.Status.LastExecutionTime,
		Success:           fio.Status.Status == "Succeeded",
		ResponseTime:      "0", // FIO没有响应时间的概念
		Output:            formatFioOutput(fio.Status.Stats),
		RawOutput:         fio.Status.LastResult,
	}

	// Extract metrics
	result.MetricValues = map[string]string{
		"QueryCount":   fmt.Sprintf("%d", fio.Status.QueryCount),
		"SuccessCount": fmt.Sprintf("%d", fio.Status.SuccessCount),
		"FailureCount": fmt.Sprintf("%d", fio.Status.FailureCount),
		"ReadIOPS":     fmt.Sprintf("%s", fio.Status.Stats.ReadIOPS),
		"WriteIOPS":    fmt.Sprintf("%s", fio.Status.Stats.WriteIOPS),
		"ReadBW":       fmt.Sprintf("%s KB/s", fio.Status.Stats.ReadBW),
		"WriteBW":      fmt.Sprintf("%s KB/s", fio.Status.Stats.WriteBW),
		"ReadLatency":  fmt.Sprintf("%s μs", fio.Status.Stats.ReadLatency),
		"WriteLatency": fmt.Sprintf("%s μs", fio.Status.Stats.WriteLatency),
	}

	// 记录添加的结果详情
	log.FromContext(context.Background()).Info("添加Fio测试结果到报告",
		"report", testReport.Name,
		"fio", fio.Name,
		"status", fio.Status.Status,
		"readIOPS", fio.Status.Stats.ReadIOPS,
		"writeIOPS", fio.Status.Stats.WriteIOPS)

	// 查找是否存在相同资源的结果
	found := false
	for i, existing := range testReport.Status.Results {
		if existing.ResourceName == result.ResourceName &&
			existing.ResourceNamespace == result.ResourceNamespace &&
			existing.ResourceKind == result.ResourceKind {
			// 替换现有结果
			testReport.Status.Results[i] = result
			found = true
			break
		}
	}

	// 如果没有找到现有结果，添加新结果
	if !found {
		testReport.Status.Results = append(testReport.Status.Results, result)
	}

	// 更新测试报告摘要
	testReport.Status.Summary.Total = len(testReport.Status.Results)
	testReport.Status.Summary.Metrics = result.MetricValues
	// 计算成功和失败数量
	succeeded := 0
	failed := 0
	for _, r := range testReport.Status.Results {
		if r.Success {
			succeeded++
		} else {
			failed++
		}
	}
	testReport.Status.Summary.Succeeded = succeeded
	testReport.Status.Summary.Failed = failed

	return nil
}

// formatFioOutput 格式化FIO输出为可读性更好的格式
func formatFioOutput(stats testtoolsv1.FioStats) string {
	var sb strings.Builder

	sb.WriteString("FIO测试结果摘要:\n\n")
	sb.WriteString(fmt.Sprintf("读取性能:\n"))

	// 添加IOPS数据，如果为空则显示为0
	readIOPS := "0"
	if stats.ReadIOPS != "" {
		readIOPS = stats.ReadIOPS
	}
	sb.WriteString(fmt.Sprintf("  IOPS: %s\n", readIOPS))

	// 添加带宽数据，如果为空则显示为0
	readBW := "0"
	if stats.ReadBW != "" {
		readBW = stats.ReadBW
	}
	sb.WriteString(fmt.Sprintf("  带宽: %s KB/s\n", readBW))

	// 添加延迟数据，如果为空则显示为0
	readLatency := "0.00"
	if stats.ReadLatency != "" {
		readLatency = stats.ReadLatency
	}
	sb.WriteString(fmt.Sprintf("  延迟: %s μs\n", readLatency))

	sb.WriteString(fmt.Sprintf("\n写入性能:\n"))

	// 添加IOPS数据，如果为空则显示为0
	writeIOPS := "0"
	if stats.WriteIOPS != "" {
		writeIOPS = stats.WriteIOPS
	}
	sb.WriteString(fmt.Sprintf("  IOPS: %s\n", writeIOPS))

	// 添加带宽数据，如果为空则显示为0
	writeBW := "0"
	if stats.WriteBW != "" {
		writeBW = stats.WriteBW
	}
	sb.WriteString(fmt.Sprintf("  带宽: %s KB/s\n", writeBW))

	// 添加延迟数据，如果为空则显示为0
	writeLatency := "0.00"
	if stats.WriteLatency != "" {
		writeLatency = stats.WriteLatency
	}
	sb.WriteString(fmt.Sprintf("  延迟: %s μs\n", writeLatency))

	return sb.String()
}

// extractPingSummary 提取 Ping 输出的摘要信息
func extractPingSummary(pingOutput string) string {
	if pingOutput == "" {
		return "No output available"
	}

	// 创建一个结构化的输出
	var structuredOutput strings.Builder

	// 提取目标主机信息
	hostLine := extractFirstLineContaining(pingOutput, "PING")
	if hostLine != "" {
		structuredOutput.WriteString("目标主机: " + hostLine + "\n")
	}

	// 提取丢包率
	packetLossLine := extractFirstLineContaining(pingOutput, "packet loss")
	if packetLossLine != "" {
		structuredOutput.WriteString("丢包率: " + strings.TrimSpace(packetLossLine) + "\n")
	}

	// 提取延迟统计
	rttLine := extractFirstLineContaining(pingOutput, "min/avg/max")
	if rttLine != "" {
		structuredOutput.WriteString("延迟统计: " + strings.TrimSpace(rttLine) + "\n")
	}

	// 提取单次 ping 结果（最多5行）
	pingLines := extractPingLines(pingOutput, 5)
	if len(pingLines) > 0 {
		structuredOutput.WriteString("\n单次ping结果:\n")
		for _, line := range pingLines {
			structuredOutput.WriteString(line + "\n")
		}
	}

	// 如果结构化输出为空，返回原始输出的截断版本
	if structuredOutput.Len() == 0 {
		if len(pingOutput) > 300 {
			return pingOutput[:300] + "... (截断)"
		}
		return pingOutput
	}

	return structuredOutput.String()
}

// extractPingLines 提取单次 ping 结果行（最多提取 maxLines 行）
func extractPingLines(text string, maxLines int) []string {
	var result []string
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		if strings.Contains(line, "bytes from") && strings.Contains(line, "time=") {
			result = append(result, line)
			if len(result) >= maxLines {
				break
			}
		}
	}

	return result
}

// isTestReportStatusEqual 检查两个测试报告状态是否相等
func (r *TestReportReconciler) isTestReportStatusEqual(oldStatus *testtoolsv1.TestReportStatus, newStatus *testtoolsv1.TestReportStatus) bool {
	// 比较基本字段
	if oldStatus.Phase != newStatus.Phase ||
		!reflect.DeepEqual(oldStatus.Summary, newStatus.Summary) {
		return false
	}

	// 比较结果列表（长度和内容）
	if len(oldStatus.Results) != len(newStatus.Results) {
		return false
	}

	// 比较每个结果
	for i, result := range oldStatus.Results {
		newResult := newStatus.Results[i]
		if result.ResourceName != newResult.ResourceName ||
			result.ResourceNamespace != newResult.ResourceNamespace ||
			result.ResourceKind != newResult.ResourceKind ||
			result.Success != newResult.Success ||
			!result.ExecutionTime.Equal(&newResult.ExecutionTime) {
			return false
		}
	}

	// 比较条件
	if len(oldStatus.Conditions) != len(newStatus.Conditions) {
		return false
	}

	// 条件可能顺序不同，所以需要逐个比较
	for _, oldCond := range oldStatus.Conditions {
		found := false
		for _, newCond := range newStatus.Conditions {
			if oldCond.Type == newCond.Type &&
				oldCond.Status == newCond.Status &&
				oldCond.Reason == newCond.Reason &&
				oldCond.Message == newCond.Message {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// updateTestReport 更新测试报告的状态
func (r *TestReportReconciler) updateTestReport(ctx context.Context, testReport *testtoolsv1.TestReport, logger logr.Logger) error {
	err := r.Status().Update(ctx, testReport)
	if err != nil {
		logger.Error(err, "Failed to update TestReport status")
		return err
	}
	return nil
}

// isTestCompleted 判断测试是否已完成
func (r *TestReportReconciler) isTestCompleted(testReport *testtoolsv1.TestReport) bool {
	// 如果已经标记为完成，则返回true
	if testReport.Status.Phase == "Completed" {
		return true
	}

	// 如果设置了TestDuration，使用它作为测试持续时间
	var duration time.Duration
	if testReport.Spec.TestDuration > 0 {
		duration = time.Duration(testReport.Spec.TestDuration) * time.Second
	} else {
		// 如果没有设置TestDuration，则使用间隔的10倍作为默认持续时间
		interval := time.Duration(testReport.Spec.Interval) * time.Second
		if interval == 0 {
			interval = 60 * time.Second // 默认间隔60秒
		}
		duration = interval * 10 // 默认运行10个间隔
	}

	// 如果没有开始时间，则不能确定是否完成
	if testReport.Status.StartTime == nil {
		return false
	}

	// 根据开始时间和持续时间判断是否已经完成
	return time.Since(testReport.Status.StartTime.Time) >= duration
}

// completeTestReport 标记测试报告为已完成
func (r *TestReportReconciler) completeTestReport(ctx context.Context, testReport *testtoolsv1.TestReport, logger logr.Logger) error {
	// 设置完成时间
	now := metav1.Now()
	testReport.Status.CompletionTime = &now
	testReport.Status.Phase = "Completed"

	// 添加完成条件
	setTestReportCondition(testReport, metav1.Condition{
		Type:               "Completed",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: now,
		Reason:             "TestCompleted",
		Message:            "Test report collection completed",
	})

	// 更新状态
	return r.Status().Update(ctx, testReport)
}

// setTestReportCondition 设置测试报告的条件
func setTestReportCondition(testReport *testtoolsv1.TestReport, condition metav1.Condition) {
	// 检查条件是否已存在
	for i, cond := range testReport.Status.Conditions {
		if cond.Type == condition.Type {
			// 仅在状态发生变化时更新
			if cond.Status != condition.Status {
				testReport.Status.Conditions[i] = condition
			}
			return
		}
	}

	// 不存在则添加新条件
	testReport.Status.Conditions = append(testReport.Status.Conditions, condition)
}

// findTestReportForResource 查找与资源相关的TestReport
func (r *TestReportReconciler) findTestReportForResource(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	// 直接获取GVK
	gvk := obj.GetObjectKind().GroupVersionKind()

	// 从object获取metaObject
	metaObj, ok := obj.(metav1.Object)
	if !ok {
		logger.Error(fmt.Errorf("object is not a metav1.Object"), "无法获取元数据")
		return nil
	}

	resourceName := metaObj.GetName()
	resourceNamespace := metaObj.GetNamespace()
	resourceKind := gvk.Kind

	// 如果GVK为空，根据对象类型判断
	if resourceKind == "" {
		switch obj.(type) {
		case *testtoolsv1.Dig:
			resourceKind = "Dig"
		case *testtoolsv1.Ping:
			resourceKind = "Ping"
		case *testtoolsv1.Fio:
			resourceKind = "Fio"
		case *testtoolsv1.Nc:
			resourceKind = "Nc"
		case *testtoolsv1.TcpPing:
			resourceKind = "TcpPing"
		case *testtoolsv1.Iperf:
			resourceKind = "Iperf"
		case *testtoolsv1.Skoop:
			resourceKind = "Skoop"
		default:
			logger.Error(fmt.Errorf("unknown resource type"), "无法确定资源类型")
			return nil
		}
	}

	// 判断是否应该创建新的TestReport
	if r.shouldCreateTestReport(ctx, obj) {
		logger.Info("为资源创建新的TestReport",
			"kind", resourceKind,
			"name", resourceName,
			"namespace", resourceNamespace)

		// 创建TestReport，避免异步可能导致的问题
		testReport, err := r.createTestReportForResource(ctx, obj)
		if err != nil {
			logger.Error(err, "创建TestReport失败",
				"kind", resourceKind,
				"name", resourceName)
		} else {
			logger.Info("成功创建TestReport",
				"testReport", testReport.Name,
				"kind", resourceKind,
				"name", resourceName)
			// 移除对markResourceWithTestReport的调用，各资源控制器自行负责设置TestReportName
		}
	}

	// 查找引用了该资源的TestReport
	var testReportList testtoolsv1.TestReportList
	err := r.List(ctx, &testReportList, client.InNamespace(resourceNamespace))
	if err != nil {
		logger.Error(err, "获取TestReport列表失败")
		return nil
	}

	// 查找引用了该资源的TestReport
	var requests []reconcile.Request
	for _, report := range testReportList.Items {
		for _, selector := range report.Spec.ResourceSelectors {
			if selector.Kind == resourceKind &&
				(selector.Name == resourceName || selector.Name == "") &&
				(selector.Namespace == resourceNamespace || selector.Namespace == "") {
				// 找到匹配的TestReport
				logger.Info("找到匹配的TestReport",
					"testReport", report.Name,
					"kind", resourceKind,
					"name", resourceName)

				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      report.Name,
						Namespace: report.Namespace,
					},
				})
			}
		}
	}

	return requests
}

// shouldCreateTestReport 判断是否应该为资源创建新的TestReport
func (r *TestReportReconciler) shouldCreateTestReport(ctx context.Context, obj client.Object) bool {
	// 打印详细的日志用于排查问题
	logger := log.FromContext(ctx)

	// 从object获取metaObject
	metaObj, ok := obj.(metav1.Object)
	if !ok {
		logger.Error(fmt.Errorf("object is not a metav1.Object"), "无法获取元数据", "object", obj)
		return false
	}

	// 获取资源信息
	resourceName := metaObj.GetName()
	resourceNamespace := metaObj.GetNamespace()
	resourceKind := obj.GetObjectKind().GroupVersionKind().Kind

	// 如果GVK为空，根据对象类型判断
	if resourceKind == "" {
		switch obj.(type) {
		case *testtoolsv1.Dig:
			resourceKind = "Dig"
		case *testtoolsv1.Ping:
			resourceKind = "Ping"
		case *testtoolsv1.Fio:
			resourceKind = "Fio"
		case *testtoolsv1.Nc:
			resourceKind = "Nc"
		case *testtoolsv1.TcpPing:
			resourceKind = "TcpPing"
		case *testtoolsv1.Iperf:
			resourceKind = "Iperf"
		case *testtoolsv1.Skoop:
			resourceKind = "Skoop"
		default:
			logger.Error(fmt.Errorf("unknown resource type"), "无法确定资源类型")
			return false
		}
	}

	// 打印资源详细信息，用于调试
	logger.Info("检查资源是否需要创建TestReport",
		"kind", resourceKind,
		"name", resourceName,
		"namespace", resourceNamespace,
		"annotations", metaObj.GetAnnotations(),
		"labels", metaObj.GetLabels())

	// 获取资源的状态
	var status string
	var testReportName string

	// 根据资源类型获取Status和TestReportName
	switch v := obj.(type) {
	case *testtoolsv1.Dig:
		status = string(v.Status.Status)
		testReportName = v.Status.TestReportName
	case *testtoolsv1.Ping:
		status = string(v.Status.Status)
		testReportName = v.Status.TestReportName
	case *testtoolsv1.Fio:
		status = string(v.Status.Status)
		testReportName = v.Status.TestReportName
	case *testtoolsv1.Nc:
		status = string(v.Status.Status)
		testReportName = v.Status.TestReportName
	case *testtoolsv1.TcpPing:
		status = string(v.Status.Status)
		testReportName = v.Status.TestReportName
	case *testtoolsv1.Iperf:
		status = string(v.Status.Status)
		testReportName = v.Status.TestReportName
	case *testtoolsv1.Skoop:
		status = string(v.Status.Status)
		testReportName = v.Status.TestReportName
	default:
		logger.Error(fmt.Errorf("unsupported resource type"), "资源类型不支持自动创建TestReport",
			"kind", resourceKind)
		return false
	}

	logger.Info("资源状态信息",
		"kind", resourceKind,
		"name", resourceName,
		"status", status,
		"testReportName", testReportName)

	// 修改判断逻辑：只要TestReportName不为空，就应该创建TestReport
	// 不再要求status必须是Succeeded或Failed
	if testReportName != "" {
		// 检查是否已经存在对应的TestReport
		existingReport := &testtoolsv1.TestReport{}
		err := r.Get(ctx, types.NamespacedName{Name: testReportName, Namespace: resourceNamespace}, existingReport)
		if err == nil {
			// TestReport已存在，不需要再创建
			logger.Info("TestReport已存在，不需要创建",
				"testReportName", testReportName,
				"kind", resourceKind,
				"name", resourceName)
			return false
		} else if !errors.IsNotFound(err) {
			// 获取TestReport时出错（不是NotFound错误）
			logger.Error(err, "检查TestReport是否存在时出错",
				"testReportName", testReportName)
			return false
		}

		// TestReport不存在，需要创建
		logger.Info("发现需要创建的TestReport",
			"testReportName", testReportName,
			"kind", resourceKind,
			"name", resourceName)
		return true
	}

	// TestReportName为空，不创建TestReport
	logger.Info("TestReportName为空，不创建TestReport",
		"kind", resourceKind,
		"name", resourceName)
	return false
}

// createTestReportForResource 为资源创建新的TestReport
func (r *TestReportReconciler) createTestReportForResource(ctx context.Context, obj client.Object) (*testtoolsv1.TestReport, error) {
	logger := log.FromContext(ctx)

	// 从object获取metaObject
	metaObj, ok := obj.(metav1.Object)
	if !ok {
		return nil, fmt.Errorf("object is not a metav1.Object")
	}

	// 获取资源信息
	resourceName := metaObj.GetName()
	resourceNamespace := metaObj.GetNamespace()
	resourceKind := obj.GetObjectKind().GroupVersionKind().Kind

	// 获取 GVK 信息
	gvk := obj.GetObjectKind().GroupVersionKind()
	apiVersion := ""

	// 如果 GVK 为空，从对象类型推断
	if gvk.Empty() {
		switch obj.(type) {
		case *testtoolsv1.Dig:
			resourceKind = "Dig"
			apiVersion = "testtools.xiaoming.com/v1"
		case *testtoolsv1.Ping:
			resourceKind = "Ping"
			apiVersion = "testtools.xiaoming.com/v1"
		case *testtoolsv1.Fio:
			resourceKind = "Fio"
			apiVersion = "testtools.xiaoming.com/v1"
		case *testtoolsv1.Nc:
			resourceKind = "Nc"
			apiVersion = "testtools.xiaoming.com/v1"
		case *testtoolsv1.TcpPing:
			resourceKind = "TcpPing"
			apiVersion = "testtools.xiaoming.com/v1"
		case *testtoolsv1.Iperf:
			resourceKind = "Iperf"
			apiVersion = "testtools.xiaoming.com/v1"
		case *testtoolsv1.Skoop:
			resourceKind = "Skoop"
			apiVersion = "testtools.xiaoming.com/v1"
		default:
			return nil, fmt.Errorf("unknown resource type")
		}
	} else {
		apiVersion = gvk.GroupVersion().String()
		resourceKind = gvk.Kind
	}

	// 修复: 确保 reportName 不以连字符开头，符合 Kubernetes 命名规则
	kindLower := strings.ToLower(resourceKind)
	// 确保名称符合规范: 只能包含小写字母、数字和连字符，不能以连字符开头或结尾
	reportName := utils.GenerateTestReportName(kindLower, resourceName)

	// 检查TestReport是否已存在
	existingReport := &testtoolsv1.TestReport{}
	err := r.Get(ctx, types.NamespacedName{Name: reportName, Namespace: resourceNamespace}, existingReport)
	if err == nil {
		// TestReport已存在，直接返回
		logger.Info("TestReport已存在，跳过创建",
			"testReport", reportName,
			"kind", resourceKind,
			"name", resourceName)
		return existingReport, nil
	} else if !errors.IsNotFound(err) {
		// 发生了除"not found"之外的错误
		logger.Error(err, "检查TestReport是否存在时出错")
		return nil, err
	}

	// 创建TestReport
	testReport := &testtoolsv1.TestReport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reportName,
			Namespace: resourceNamespace,
			// 添加所有者引用，使TestReport随其所有者资源一起删除
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: apiVersion,
					Kind:       resourceKind,
					Name:       resourceName,
					UID:        metaObj.GetUID(),
					Controller: pointer.BoolPtr(true),
				},
			},
		},
		Spec: testtoolsv1.TestReportSpec{
			TestName: reportName,
			TestType: getTestTypeFromKind(resourceKind),
			Target:   getTargetFromResource(obj),
			ResourceSelectors: []testtoolsv1.ResourceSelector{
				{
					APIVersion: "testtools.xiaoming.com/v1",
					Kind:       resourceKind,
					Name:       resourceName,
					Namespace:  resourceNamespace,
				},
			},
		},
	}

	// 创建TestReport
	if err := r.Create(ctx, testReport); err != nil {
		// 如果是因为资源已存在导致的错误，忽略它
		if errors.IsAlreadyExists(err) {
			logger.Info("TestReport已存在，另一个协程已创建",
				"testReport", reportName,
				"kind", resourceKind,
				"name", resourceName)

			// 尝试获取这个已存在的TestReport
			existingReport := &testtoolsv1.TestReport{}
			if getErr := r.Get(ctx, types.NamespacedName{Name: reportName, Namespace: resourceNamespace}, existingReport); getErr == nil {
				return existingReport, nil
			}
			// 如果无法获取，返回nil和原始错误
			return nil, err
		}

		logger.Error(err, "创建TestReport失败")
		return nil, err
	}

	// 记录成功信息
	logger.Info("成功创建TestReport",
		"testReport", reportName,
		"kind", resourceKind,
		"name", resourceName,
		"namespace", resourceNamespace)

	return testReport, nil
}

// getTestTypeFromKind 根据资源类型获取测试类型
func getTestTypeFromKind(kind string) string {
	switch kind {
	case "Dig":
		return "DNS"
	case "Ping":
		return "PING"
	case "Fio":
		return "Performance"
	default:
		return "Network"
	}
}

// getTargetFromResource 从资源获取目标
func getTargetFromResource(obj client.Object) string {
	switch obj.GetObjectKind().GroupVersionKind().Kind {
	case "Dig":
		if dig, ok := obj.(*testtoolsv1.Dig); ok {
			return dig.Spec.Domain
		}
	case "Ping":
		if ping, ok := obj.(*testtoolsv1.Ping); ok {
			return ping.Spec.Host
		}
	case "Fio":
		if fio, ok := obj.(*testtoolsv1.Fio); ok {
			return fio.Spec.FilePath
		}
	}
	return ""
}

// SetupWithManager sets up the controller with the Manager.
func (r *TestReportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// 添加事件记录器
	r.Recorder = mgr.GetEventRecorderFor("testreport-controller")

	return ctrl.NewControllerManagedBy(mgr).
		For(&testtoolsv1.TestReport{}).
		// 监听所有资源变更
		Watches(
			&testtoolsv1.Dig{},
			handler.EnqueueRequestsFromMapFunc(r.findTestReportForResource),
		).
		Watches(
			&testtoolsv1.Ping{},
			handler.EnqueueRequestsFromMapFunc(r.findTestReportForResource),
		).
		Watches(
			&testtoolsv1.Fio{},
			handler.EnqueueRequestsFromMapFunc(r.findTestReportForResource),
		).
		Watches(
			&testtoolsv1.Nc{},
			handler.EnqueueRequestsFromMapFunc(r.findTestReportForResource),
		).
		Watches(
			&testtoolsv1.TcpPing{},
			handler.EnqueueRequestsFromMapFunc(r.findTestReportForResource),
		).
		Watches(
			&testtoolsv1.Iperf{},
			handler.EnqueueRequestsFromMapFunc(r.findTestReportForResource),
		).
		Watches(
			&testtoolsv1.Skoop{},
			handler.EnqueueRequestsFromMapFunc(r.findTestReportForResource),
		).
		Complete(r)
}

// addNcResult 添加 Nc 测试结果到测试报告
func (r *TestReportReconciler) addNcResult(testReport *testtoolsv1.TestReport, nc *testtoolsv1.Nc) error {
	// 创建结果记录
	result := testtoolsv1.TestResult{
		ResourceKind:      "Nc",
		ResourceName:      nc.Name,
		ResourceNamespace: nc.Namespace,
		ExecutionTime:     *nc.Status.LastExecutionTime,
		//Success:           nc.Status.ConnectionSuccess,
		Success:      nc.Status.Status == "Succeeded",
		ResponseTime: nc.Status.ConnectionLatency,
	}

	// 设置输出摘要
	result.Output = fmt.Sprintf("主机: %s, 端口: %d, 协议: %s, 连接成功: %t, 延迟: %s",
		nc.Spec.Host,
		nc.Spec.Port,
		nc.Spec.Protocol,
		nc.Status.ConnectionSuccess,
		nc.Status.ConnectionLatency)

	// 提取原始输出
	if nc.Status.LastResult != "" {
		result.RawOutput = nc.Status.LastResult
		// 如果原始输出过长，则截断
		if len(result.RawOutput) > 5000 {
			result.RawOutput = result.RawOutput[:5000] + "... (输出已截断)"
		}
	}

	// 添加统计信息
	result.MetricValues = map[string]string{
		"ConnectionSuccess":     fmt.Sprintf("%t", nc.Status.ConnectionSuccess),
		"ConnectionLatency(ms)": nc.Status.ConnectionLatency,
		"Protocol":              nc.Spec.Protocol,
		"Host":                  nc.Spec.Host,
		"Port":                  fmt.Sprintf("%d", nc.Spec.Port),
		"QueryCount":            fmt.Sprintf("%d", nc.Status.QueryCount),
		"SuccessCount":          fmt.Sprintf("%d", nc.Status.SuccessCount),
		"FailureCount":          fmt.Sprintf("%d", nc.Status.FailureCount),
	}

	// 添加结果到报告
	testReport.Status.Results = append(testReport.Status.Results, result)
	testReport.Status.Summary.Metrics = result.MetricValues

	// 更新汇总计数
	testReport.Status.Summary.Total++
	if nc.Status.Status == "Succeeded" || nc.Status.ConnectionSuccess {
		testReport.Status.Summary.Succeeded++
	} else {
		testReport.Status.Summary.Failed++
	}

	return nil
}

// collectTcpPingResults 收集 TcpPing 资源的结果
func (r *TestReportReconciler) collectTcpPingResults(ctx context.Context, testReport *testtoolsv1.TestReport, selector testtoolsv1.ResourceSelector, logger logr.Logger) (bool, error) {
	tcpPing := &testtoolsv1.TcpPing{}
	err := r.Get(ctx, types.NamespacedName{Name: selector.Name, Namespace: selector.Namespace}, tcpPing)
	if err != nil {
		if errors.IsNotFound(err) {
			// 资源已被删除，记录信息并继续
			logger.Info("TcpPing资源不存在，可能已被删除",
				"name", selector.Name,
				"namespace", selector.Namespace)

			// 返回false但不报错，表示此资源不可用但不阻止其他资源的处理
			return false, nil
		}
		logger.Error(err, "获取TcpPing资源失败",
			"name", selector.Name,
			"namespace", selector.Namespace)
		return false, err
	}

	// 检查最后执行时间，避免重复添加结果
	if tcpPing.Status.LastExecutionTime == nil {
		logger.Info("TcpPing测试尚未执行",
			"tcpPing", tcpPing.Name,
			"namespace", tcpPing.Namespace)
		return false, nil
	}

	// 检查是否已包含此结果
	for _, result := range testReport.Status.Results {
		if result.ResourceName == tcpPing.Name &&
			result.ResourceNamespace == tcpPing.Namespace &&
			result.ResourceKind == "TcpPing" &&
			result.ExecutionTime.Equal(tcpPing.Status.LastExecutionTime) {
			// 已存在此结果
			logger.Info("TcpPing测试结果已存在",
				"tcpPing", tcpPing.Name,
				"executionTime", tcpPing.Status.LastExecutionTime)
			return true, nil
		}
	}

	// 使用事件记录关联关系
	r.Recorder.Event(tcpPing, corev1.EventTypeNormal, "CollectedByTestReport",
		fmt.Sprintf("结果被测试报告 %s 收集", testReport.Name))

	// 添加结果
	err = r.addTcpPingResult(testReport, tcpPing)
	if err != nil {
		logger.Error(err, "添加TcpPing结果失败",
			"tcpPing", tcpPing.Name,
			"namespace", tcpPing.Namespace)
		return false, err
	}

	logger.Info("成功添加TcpPing测试结果",
		"tcpPing", tcpPing.Name,
		"namespace", tcpPing.Namespace,
		"executionTime", tcpPing.Status.LastExecutionTime)
	return true, nil
}

// addTcpPingResult 添加 TcpPing 测试结果到测试报告
func (r *TestReportReconciler) addTcpPingResult(testReport *testtoolsv1.TestReport, tcpPing *testtoolsv1.TcpPing) error {
	// 创建结果记录
	result := testtoolsv1.TestResult{
		ResourceKind:      "TcpPing",
		ResourceName:      tcpPing.Name,
		ResourceNamespace: tcpPing.Namespace,
		ExecutionTime:     *tcpPing.Status.LastExecutionTime,
		//Success:           tcpPing.Status.Stats.Received > 0,
		Success:      tcpPing.Status.Status == "Succeeded",
		ResponseTime: tcpPing.Status.Stats.AvgLatency,
	}

	// 设置输出摘要
	result.Output = fmt.Sprintf("主机: %s, 端口: %d, 发送: %d, 接收: %d, 丢包率: %s, 平均延迟: %s",
		tcpPing.Spec.Host,
		tcpPing.Spec.Port,
		tcpPing.Status.Stats.Transmitted,
		tcpPing.Status.Stats.Received,
		tcpPing.Status.Stats.PacketLoss,
		tcpPing.Status.Stats.AvgLatency)

	// 提取原始输出
	if tcpPing.Status.LastResult != "" {
		result.RawOutput = tcpPing.Status.LastResult
		// 如果原始输出过长，则截断
		if len(result.RawOutput) > 5000 {
			result.RawOutput = result.RawOutput[:5000] + "... (输出已截断)"
		}
	}

	// 添加统计信息
	result.MetricValues = map[string]string{
		"Host":              tcpPing.Spec.Host,
		"Port":              fmt.Sprintf("%d", tcpPing.Spec.Port),
		"Transmitted":       fmt.Sprintf("%d", tcpPing.Status.Stats.Transmitted),
		"Received":          fmt.Sprintf("%d", tcpPing.Status.Stats.Received),
		"PacketLoss":        tcpPing.Status.Stats.PacketLoss,
		"MinLatency(ms)":    tcpPing.Status.Stats.MinLatency,
		"AvgLatency(ms)":    tcpPing.Status.Stats.AvgLatency,
		"MaxLatency(ms)":    tcpPing.Status.Stats.MaxLatency,
		"StdDevLatency(ms)": tcpPing.Status.Stats.StdDevLatency,
		"QueryCount":        fmt.Sprintf("%d", tcpPing.Status.QueryCount),
		"SuccessCount":      fmt.Sprintf("%d", tcpPing.Status.SuccessCount),
		"FailureCount":      fmt.Sprintf("%d", tcpPing.Status.FailureCount),
	}

	// 添加结果到报告
	testReport.Status.Results = append(testReport.Status.Results, result)
	testReport.Status.Summary.Metrics = result.MetricValues

	// 更新汇总计数
	testReport.Status.Summary.Total++
	if tcpPing.Status.Status == "Succeeded" || tcpPing.Status.Stats.Received > 0 {
		testReport.Status.Summary.Succeeded++
	} else {
		testReport.Status.Summary.Failed++
	}

	return nil
}

// collectIperfResults 收集 Iperf 资源的结果
func (r *TestReportReconciler) collectIperfResults(ctx context.Context, testReport *testtoolsv1.TestReport, selector testtoolsv1.ResourceSelector, logger logr.Logger) (bool, error) {
	iperf := &testtoolsv1.Iperf{}
	err := r.Get(ctx, types.NamespacedName{Name: selector.Name, Namespace: selector.Namespace}, iperf)
	if err != nil {
		if errors.IsNotFound(err) {
			// 资源已被删除，记录信息并继续
			logger.Info("Iperf资源不存在，可能已被删除",
				"name", selector.Name,
				"namespace", selector.Namespace)

			// 返回false但不报错，表示此资源不可用但不阻止其他资源的处理
			return false, nil
		}
		logger.Error(err, "获取Iperf资源失败",
			"name", selector.Name,
			"namespace", selector.Namespace)
		return false, err
	}

	// 检查最后执行时间，避免重复添加结果
	if iperf.Status.LastExecutionTime == nil {
		logger.Info("Iperf测试尚未执行",
			"iperf", iperf.Name,
			"namespace", iperf.Namespace)
		return false, nil
	}

	// 检查是否已包含此结果
	for _, result := range testReport.Status.Results {
		if result.ResourceName == iperf.Name &&
			result.ResourceNamespace == iperf.Namespace &&
			result.ResourceKind == "Iperf" &&
			result.ExecutionTime.Equal(iperf.Status.LastExecutionTime) {
			// 已存在此结果
			logger.Info("Iperf测试结果已存在",
				"iperf", iperf.Name,
				"executionTime", iperf.Status.LastExecutionTime)
			return true, nil
		}
	}

	// 使用事件记录关联关系
	r.Recorder.Event(iperf, corev1.EventTypeNormal, "CollectedByTestReport",
		fmt.Sprintf("结果被测试报告 %s 收集", testReport.Name))

	// 添加结果
	err = r.addIperfResult(testReport, iperf)
	if err != nil {
		logger.Error(err, "添加Iperf结果失败",
			"iperf", iperf.Name,
			"namespace", iperf.Namespace)
		return false, err
	}

	logger.Info("成功添加Iperf测试结果",
		"iperf", iperf.Name,
		"namespace", iperf.Namespace,
		"executionTime", iperf.Status.LastExecutionTime)
	return true, nil
}

// addIperfResult 添加 Iperf 测试结果到测试报告
func (r *TestReportReconciler) addIperfResult(testReport *testtoolsv1.TestReport, iperf *testtoolsv1.Iperf) error {
	// 创建结果记录
	result := testtoolsv1.TestResult{
		ResourceKind:      "Iperf",
		ResourceName:      iperf.Name,
		ResourceNamespace: iperf.Namespace,
		ExecutionTime:     *iperf.Status.LastExecutionTime,
		Success:           iperf.Status.Status == "Succeeded",
		ResponseTime:      iperf.Status.Stats.RttMs,
	}

	// 设置输出摘要
	if iperf.Spec.Mode == "client" {
		result.Output = fmt.Sprintf("主机: %s, 端口: %d, 协议: %s, 发送带宽: %s, 接收带宽: %s, RTT: %s",
			iperf.Spec.Host,
			iperf.Spec.Port,
			iperf.Spec.Protocol,
			iperf.Status.Stats.SendBandwidth,
			iperf.Status.Stats.ReceiveBandwidth,
			iperf.Status.Stats.RttMs)
	} else {
		result.Output = fmt.Sprintf("监听端口: %d, 协议: %s, 模式: %s",
			iperf.Spec.Port,
			iperf.Spec.Protocol,
			iperf.Spec.Mode)
	}

	// 提取原始输出
	if iperf.Status.LastResult != "" {
		result.RawOutput = iperf.Status.LastResult
		// 如果原始输出过长，则截断
		if len(result.RawOutput) > 5000 {
			result.RawOutput = result.RawOutput[:5000] + "... (输出已截断)"
		}
	}

	// 添加统计信息
	result.MetricValues = map[string]string{
		"Host":             iperf.Spec.Host,
		"Port":             fmt.Sprintf("%d", iperf.Spec.Port),
		"Protocol":         iperf.Spec.Protocol,
		"Mode":             iperf.Spec.Mode,
		"SendBandwidth":    iperf.Status.Stats.SendBandwidth,
		"ReceiveBandwidth": iperf.Status.Stats.ReceiveBandwidth,
		"RTT(ms)":          iperf.Status.Stats.RttMs,
		"JTT(ms)":          iperf.Status.Stats.Jitter,
		"LostPackets":      fmt.Sprintf("%d", iperf.Status.Stats.LostPackets),
		"LostPercent":      iperf.Status.Stats.LostPercent,
		"Retransmits":      fmt.Sprintf("%d", iperf.Status.Stats.Retransmits),
		"CpuUtilization":   iperf.Status.Stats.CpuUtilization,
		"QueryCount":       fmt.Sprintf("%d", iperf.Status.QueryCount),
		"SuccessCount":     fmt.Sprintf("%d", iperf.Status.SuccessCount),
		"FailureCount":     fmt.Sprintf("%d", iperf.Status.FailureCount),
	}

	// 添加结果到报告
	testReport.Status.Results = append(testReport.Status.Results, result)
	testReport.Status.Summary.Metrics = result.MetricValues

	// 更新汇总计数
	testReport.Status.Summary.Total++
	if iperf.Status.Status == "Succeeded" {
		testReport.Status.Summary.Succeeded++
	} else {
		testReport.Status.Summary.Failed++
	}

	return nil
}

// collectSkoopResults 收集 Skoop 资源的结果
func (r *TestReportReconciler) collectSkoopResults(ctx context.Context, testReport *testtoolsv1.TestReport, selector testtoolsv1.ResourceSelector, logger logr.Logger) (bool, error) {
	skoop := &testtoolsv1.Skoop{}
	err := r.Get(ctx, types.NamespacedName{Name: selector.Name, Namespace: selector.Namespace}, skoop)
	if err != nil {
		if errors.IsNotFound(err) {
			// 资源已被删除，记录信息并继续
			logger.Info("Skoop资源不存在，可能已被删除",
				"name", selector.Name,
				"namespace", selector.Namespace)

			// 返回false但不报错，表示此资源不可用但不阻止其他资源的处理
			return false, nil
		}
		logger.Error(err, "获取Skoop资源失败",
			"name", selector.Name,
			"namespace", selector.Namespace)
		return false, err
	}

	// 检查最后执行时间，避免重复添加结果
	if skoop.Status.LastExecutionTime == nil {
		logger.Info("Skoop测试尚未执行",
			"skoop", skoop.Name,
			"namespace", skoop.Namespace)
		return false, nil
	}

	// 检查是否已包含此结果
	for _, result := range testReport.Status.Results {
		if result.ResourceName == skoop.Name &&
			result.ResourceNamespace == skoop.Namespace &&
			result.ResourceKind == "Skoop" &&
			result.ExecutionTime.Equal(skoop.Status.LastExecutionTime) {
			// 已存在此结果
			logger.Info("Skoop测试结果已存在",
				"skoop", skoop.Name,
				"executionTime", skoop.Status.LastExecutionTime)
			return true, nil
		}
	}

	// 使用事件记录关联关系
	r.Recorder.Event(skoop, corev1.EventTypeNormal, "CollectedByTestReport",
		fmt.Sprintf("结果被测试报告 %s 收集", testReport.Name))

	// 添加结果
	err = r.addSkoopResult(testReport, skoop)
	if err != nil {
		logger.Error(err, "添加Skoop结果失败",
			"skoop", skoop.Name,
			"namespace", skoop.Namespace)
		return false, err
	}

	logger.Info("成功添加Skoop测试结果",
		"skoop", skoop.Name,
		"namespace", skoop.Namespace,
		"executionTime", skoop.Status.LastExecutionTime)
	return true, nil
}

// addSkoopResult 添加 Skoop 测试结果到测试报告
func (r *TestReportReconciler) addSkoopResult(testReport *testtoolsv1.TestReport, skoop *testtoolsv1.Skoop) error {
	// 找出网络路径中的延迟
	var latency string
	if len(skoop.Status.Path) > 0 {
		for _, node := range skoop.Status.Path {
			if node.LatencyMs != "" {
				latency = node.LatencyMs
				break
			}
		}
	}

	// 创建结果记录
	result := testtoolsv1.TestResult{
		ResourceKind:      "Skoop",
		ResourceName:      skoop.Name,
		ResourceNamespace: skoop.Namespace,
		ExecutionTime:     *skoop.Status.LastExecutionTime,
		Success:           skoop.Status.Status == "Succeeded",
		ResponseTime:      latency,
	}

	// 设置输出摘要
	result.Output = fmt.Sprintf("源地址: %s, 目标地址: %s, 端口: %d, 协议: %s, 状态: %s",
		skoop.Spec.Task.SourceAddress,
		skoop.Spec.Task.DestinationAddress,
		skoop.Spec.Task.DestinationPort,
		skoop.Spec.Task.Protocol,
		skoop.Status.Status)

	// 提取原始输出
	if skoop.Status.LastResult != "" {
		result.RawOutput = skoop.Status.LastResult
		// 如果原始输出过长，则截断
		if len(result.RawOutput) > 5000 {
			result.RawOutput = result.RawOutput[:5000] + "... (输出已截断)"
		}
	}

	// 添加诊断概要
	if skoop.Status.Summary != "" {
		if result.Output != "" {
			result.Output += "\n"
		}
		result.Output += "诊断概要: " + skoop.Status.Summary
	}

	// 添加统计信息
	result.MetricValues = map[string]string{
		"SourceAddress":      skoop.Spec.Task.SourceAddress,
		"DestinationAddress": skoop.Spec.Task.DestinationAddress,
		"DestinationPort":    fmt.Sprintf("%d", skoop.Spec.Task.DestinationPort),
		"Protocol":           skoop.Spec.Task.Protocol,
		"Status":             skoop.Status.Status,
	}

	// 添加路径节点信息
	if len(skoop.Status.Path) > 0 {
		for i, node := range skoop.Status.Path {
			nodeKey := fmt.Sprintf("路径节点%d", i+1)
			nodeValue := fmt.Sprintf("%s(%s)", node.Name, node.Type)
			if node.IP != "" {
				nodeValue += fmt.Sprintf(", IP: %s", node.IP)
			}
			if node.LatencyMs != "" {
				nodeValue += fmt.Sprintf(", 延迟: %s ms", node.LatencyMs)
			}
			result.MetricValues[nodeKey] = nodeValue
		}
	}

	// 添加问题信息
	if len(skoop.Status.Issues) > 0 {
		for i, issue := range skoop.Status.Issues {
			issueKey := fmt.Sprintf("问题%d", i+1)
			issueValue := fmt.Sprintf("%s: %s", issue.Level, issue.Message)
			result.MetricValues[issueKey] = issueValue
		}
	}

	// 添加其他统计信息
	result.MetricValues["QueryCount"] = fmt.Sprintf("%d", skoop.Status.QueryCount)
	result.MetricValues["SuccessCount"] = fmt.Sprintf("%d", skoop.Status.SuccessCount)
	result.MetricValues["FailureCount"] = fmt.Sprintf("%d", skoop.Status.FailureCount)

	// 添加结果到报告
	testReport.Status.Results = append(testReport.Status.Results, result)

	// 更新汇总计数
	testReport.Status.Summary.Total++
	if skoop.Status.Status == "Succeeded" {
		testReport.Status.Summary.Succeeded++
	} else {
		testReport.Status.Summary.Failed++
	}

	return nil
}
