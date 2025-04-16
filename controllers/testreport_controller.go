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
	"strings"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"reflect"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	"github.com/xiaoming/testtools/pkg/analytics"
	"github.com/xiaoming/testtools/pkg/exporters"
	"github.com/xiaoming/testtools/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
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
	// 获取Dig资源
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

	// 使用事件记录关联关系，并使用UpdateWithRetry安全地更新Dig资源的Status
	r.Recorder.Event(dig, corev1.EventTypeNormal, "CollectedByTestReport",
		fmt.Sprintf("结果被测试报告 %s 收集", testReport.Name))

	// 只有在TestReportName为空时才更新
	if dig.Status.TestReportName == "" {
		err = r.updateDigStatus(ctx, types.NamespacedName{Name: dig.Name, Namespace: dig.Namespace}, func(digObj *testtoolsv1.Dig) {
			digObj.Status.TestReportName = testReport.Name
		})
		if err != nil {
			logger.Error(err, "更新Dig资源的TestReportName失败",
				"name", dig.Name,
				"namespace", dig.Namespace)
			// 继续处理，不要因为更新失败而中断
		} else {
			logger.Info("已更新Dig资源的TestReportName",
				"name", dig.Name,
				"namespace", dig.Namespace,
				"reportName", testReport.Name)
		}
	}

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

	// 只有在TestReportName为空时才更新
	if fio.Status.TestReportName == "" {
		err = r.updateFioStatus(ctx, types.NamespacedName{Name: fio.Name, Namespace: fio.Namespace}, func(fioObj *testtoolsv1.Fio) {
			fioObj.Status.TestReportName = testReport.Name
		})
		if err != nil {
			logger.Error(err, "更新Fio资源的TestReportName失败",
				"name", fio.Name,
				"namespace", fio.Namespace)
			// 继续处理，不要因为更新失败而中断
		} else {
			logger.Info("已更新Fio资源的TestReportName",
				"name", fio.Name,
				"namespace", fio.Namespace,
				"reportName", testReport.Name)
		}
	}

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

		// 使用事件记录关联关系，并使用UpdateWithRetry安全地更新Ping资源的Status
		r.Recorder.Event(&ping, corev1.EventTypeNormal, "CollectedByTestReport",
			fmt.Sprintf("结果被测试报告 %s 收集", testReport.Name))

		// 只有在TestReportName为空时才更新
		if ping.Status.TestReportName == "" {
			err = r.updatePingStatus(ctx, types.NamespacedName{Name: ping.Name, Namespace: ping.Namespace}, func(pingObj *testtoolsv1.Ping) {
				pingObj.Status.TestReportName = testReport.Name
			})
			if err != nil {
				logger.Error(err, "更新Ping资源的TestReportName失败",
					"name", ping.Name,
					"namespace", ping.Namespace)
				// 继续处理，不要因为更新失败而中断
			} else {
				logger.Info("已更新Ping资源的TestReportName",
					"name", ping.Name,
					"namespace", ping.Namespace,
					"reportName", testReport.Name)
			}
		}

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

		// 使用事件记录关联关系，并使用UpdateWithRetry安全地更新Ping资源的Status
		r.Recorder.Event(ping, corev1.EventTypeNormal, "CollectedByTestReport",
			fmt.Sprintf("结果被测试报告 %s 收集", testReport.Name))

		// 只有在TestReportName为空时才更新
		if ping.Status.TestReportName == "" {
			err := r.updatePingStatus(ctx, types.NamespacedName{Name: ping.Name, Namespace: ping.Namespace}, func(pingObj *testtoolsv1.Ping) {
				pingObj.Status.TestReportName = testReport.Name
			})
			if err != nil {
				logger.Error(err, "更新Ping资源的TestReportName失败",
					"name", ping.Name,
					"namespace", ping.Namespace)
				// 继续处理，不要因为更新失败而中断
			} else {
				logger.Info("已更新Ping资源的TestReportName",
					"name", ping.Name,
					"namespace", ping.Namespace,
					"reportName", testReport.Name)
			}
		}

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

	return nil
}

// addFioResult 将 Fio 资源的结果添加到测试报告中
func (r *TestReportReconciler) addFioResult(testReport *testtoolsv1.TestReport, fio *testtoolsv1.Fio) error {
	if fio.Status.LastExecutionTime == nil {
		return fmt.Errorf("fio has no last execution time")
	}

	// 解析FIO测试的结果
	// 创建新的结果
	result := testtoolsv1.TestResult{
		ResourceName:      fio.Name,
		ResourceNamespace: fio.Namespace,
		ResourceKind:      "Fio",
		ExecutionTime:     *fio.Status.LastExecutionTime,
		Success:           fio.Status.Status == "Succeeded",
		ResponseTime:      0, // FIO没有单一的响应时间
		Output:            extractFioSummary(fio.Status.LastResult),
		RawOutput:         fio.Status.LastResult,
	}

	// 如果测试失败，记录错误信息
	if fio.Status.Status == "Failed" {
		result.Error = fmt.Sprintf("FIO测试失败: %s", fio.Status.LastResult)
	}

	// 添加基本指标到MetricValues
	result.MetricValues = make(map[string]string)
	result.MetricValues["status"] = fio.Status.Status
	result.MetricValues["queryCount"] = fmt.Sprintf("%d", fio.Status.QueryCount)
	result.MetricValues["successCount"] = fmt.Sprintf("%d", fio.Status.SuccessCount)
	result.MetricValues["failureCount"] = fmt.Sprintf("%d", fio.Status.FailureCount)

	// 添加执行的命令
	if fio.Status.ExecutedCommand != "" {
		result.MetricValues["executedCommand"] = fio.Status.ExecutedCommand
	}

	// 添加更多详细的性能指标
	result.MetricValues["readIOPS"] = fmt.Sprintf("%.2f", fio.Status.Stats.ReadIOPS)
	result.MetricValues["writeIOPS"] = fmt.Sprintf("%.2f", fio.Status.Stats.WriteIOPS)
	result.MetricValues["readBW"] = fmt.Sprintf("%.2f", fio.Status.Stats.ReadBW)
	result.MetricValues["writeBW"] = fmt.Sprintf("%.2f", fio.Status.Stats.WriteBW)
	result.MetricValues["readLatency"] = fmt.Sprintf("%.2f", fio.Status.Stats.ReadLatency)
	result.MetricValues["writeLatency"] = fmt.Sprintf("%.2f", fio.Status.Stats.WriteLatency)

	// 添加测试配置信息
	result.MetricValues["filePath"] = fio.Spec.FilePath
	result.MetricValues["readWrite"] = fio.Spec.ReadWrite
	result.MetricValues["blockSize"] = fio.Spec.BlockSize
	result.MetricValues["ioDepth"] = fmt.Sprintf("%d", fio.Spec.IODepth)
	result.MetricValues["size"] = fio.Spec.Size
	result.MetricValues["ioEngine"] = fio.Spec.IOEngine
	result.MetricValues["numJobs"] = fmt.Sprintf("%d", fio.Spec.NumJobs)
	if fio.Spec.Runtime > 0 {
		result.MetricValues["runtime"] = fmt.Sprintf("%d", fio.Spec.Runtime)
	}
	result.MetricValues["directIO"] = fmt.Sprintf("%t", fio.Spec.DirectIO)

	// 添加延迟百分位数据（如果存在）
	if fio.Status.Stats.LatencyPercentiles != nil && len(fio.Status.Stats.LatencyPercentiles) > 0 {
		for percentile, value := range fio.Status.Stats.LatencyPercentiles {
			result.MetricValues["latencyP"+percentile] = fmt.Sprintf("%.2f", value)
		}
	}

	// 添加到结果集
	testReport.Status.Results = append(testReport.Status.Results, result)
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
	domainLine := extractFirstLineContaining(digOutput, "IN")
	if domainLine != "" {
		structuredOutput.WriteString("域名查询: " + domainLine + "\n")
	}

	// 提取服务器信息
	serverLine := extractFirstLineContaining(digOutput, "SERVER:")
	if serverLine != "" {
		structuredOutput.WriteString("服务器: " + strings.TrimSpace(serverLine) + "\n")
	}

	// 提取查询时间
	queryTimeLine := extractFirstLineContaining(digOutput, "Query time:")
	if queryTimeLine != "" {
		structuredOutput.WriteString("查询时间: " + strings.TrimSpace(queryTimeLine) + "\n")
	}

	// 提取消息大小
	msgSizeLine := extractFirstLineContaining(digOutput, "MSG SIZE")
	if msgSizeLine != "" {
		structuredOutput.WriteString("消息大小: " + strings.TrimSpace(msgSizeLine) + "\n")
	}

	// 提取应答部分
	answerSection := extractSection(digOutput, "ANSWER SECTION:", "AUTHORITY SECTION:")
	if answerSection != "" {
		structuredOutput.WriteString("\n应答部分:\n" + answerSection + "\n")
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

// extractFirstLineContaining 提取包含指定子字符串的第一行
func extractFirstLineContaining(text, substring string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if strings.Contains(line, substring) {
			return line
		}
	}
	return ""
}

// extractFioSummary 提取 Fio 输出的摘要信息
func extractFioSummary(fioOutput string) string {
	if fioOutput == "" {
		return "No output available"
	}

	// 尝试从 FIO 输出中提取摘要信息
	// 这里根据实际的 FIO 输出格式进行调整
	lines := strings.Split(fioOutput, "\n")
	summary := make([]string, 0)

	// 只保留包含关键信息的行
	for _, line := range lines {
		if strings.Contains(line, "iops") ||
			strings.Contains(line, "bw=") ||
			strings.Contains(line, "lat") {
			summary = append(summary, line)
		}
	}

	// 如果找不到有用的摘要，返回简短版本的原始输出
	if len(summary) == 0 {
		if len(fioOutput) > 300 {
			return fioOutput[:300] + "... (truncated)"
		}
		return fioOutput
	}

	return strings.Join(summary, "\n")
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

// analyzeTrends 分析性能趋势
func (r *TestReportReconciler) analyzeTrends(ctx context.Context, testReport *testtoolsv1.TestReport) {
	if testReport.Spec.AnalyticsConfig == nil || !testReport.Spec.AnalyticsConfig.EnableTrendAnalysis {
		return // 未启用趋势分析
	}

	if len(testReport.Status.Results) < 3 {
		return // 数据不足以分析趋势
	}

	logger := log.FromContext(ctx)
	logger.Info("执行趋势分析", "report", testReport.Name)

	// 使用analytics包进行趋势分析
	trend := analytics.PerformanceTrend(testReport.Status.Results)

	// 如果Analytics字段为nil，初始化它
	if testReport.Status.Analytics == nil {
		testReport.Status.Analytics = &testtoolsv1.Analytics{
			PerformanceTrend: make(map[string]string),
			ChangeRate:       make(map[string]float64),
		}
	}

	// 更新趋势
	testReport.Status.Analytics.PerformanceTrend = trend.Trend
	testReport.Status.Analytics.ChangeRate = trend.ChangeRate

	// 检查是否有显著的趋势变化需要提醒
	for metric, trendType := range trend.Trend {
		if trendType == "下降" && isPerformanceMetric(metric) {
			// 性能指标下降，设置条件
			now := metav1.NewTime(time.Now())
			utils.SetCondition(&testReport.Status.Conditions, metav1.Condition{
				Type:               "TrendWarning",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "PerformanceDegrading",
				Message:            fmt.Sprintf("性能指标 %s 呈下降趋势", metric),
			})
		}
	}
}

// detectAnomalies 检测异常
func (r *TestReportReconciler) detectAnomalies(ctx context.Context, testReport *testtoolsv1.TestReport) {
	if testReport.Spec.AnalyticsConfig == nil || !testReport.Spec.AnalyticsConfig.EnableAnomalyDetection {
		return // 未启用异常检测
	}

	if len(testReport.Status.Results) < 5 {
		return // 数据不足以检测异常
	}

	logger := log.FromContext(ctx)
	logger.Info("执行异常检测", "report", testReport.Name)

	// 使用analytics包进行异常检测
	anomalies := analytics.DetectAnomalies(testReport.Status.Results)

	// 更新异常列表
	testReport.Status.Anomalies = make([]testtoolsv1.Anomaly, 0, len(anomalies))
	for _, anomaly := range anomalies {
		testReport.Status.Anomalies = append(testReport.Status.Anomalies, testtoolsv1.Anomaly{
			Metric:        anomaly.Metric,
			Value:         anomaly.Value,
			DetectionTime: anomaly.DetectionTime,
			ExecutionTime: anomaly.ExecutionTime,
			Severity:      anomaly.Severity,
			Description:   anomaly.Description,
		})

		// 如果发现严重异常，设置告警条件
		if anomaly.Severity == "高" {
			now := metav1.NewTime(time.Now())
			utils.SetCondition(&testReport.Status.Conditions, metav1.Condition{
				Type:               "AnomalyDetected",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "SevereAnomaly",
				Message:            anomaly.Description,
			})
		}
	}
}

// exportMetrics 导出指标到外部系统
func (r *TestReportReconciler) exportMetrics(ctx context.Context, testReport *testtoolsv1.TestReport) {
	if testReport.Spec.ExportConfig == nil || testReport.Spec.ExportConfig.Destination == "" {
		return // 未配置导出目标
	}

	logger := log.FromContext(ctx)

	if testReport.Spec.ExportConfig.Format == "prometheus" {
		logger.Info("导出指标到Prometheus", "destination", testReport.Spec.ExportConfig.Destination)
		exporter := exporters.NewPrometheusExporter(
			testReport.Spec.ExportConfig.Destination,
			fmt.Sprintf("testtools_%s", testReport.Name),
		)

		if err := exporter.ExportResults(ctx, testReport); err != nil {
			logger.Error(err, "导出指标到Prometheus失败")
		}
	}
}

// isPerformanceMetric 判断是否为性能指标
func isPerformanceMetric(metric string) bool {
	performanceMetrics := map[string]bool{
		"ReadIOPS":     true,
		"WriteIOPS":    true,
		"ReadBW":       true,
		"WriteBW":      true,
		"ReadLatency":  true,
		"WriteLatency": true,
	}
	return performanceMetrics[metric]
}

// updateTestReport 更新测试报告的状态
func (r *TestReportReconciler) updateTestReport(ctx context.Context, testReport *testtoolsv1.TestReport, logger logr.Logger) error {
	// 首先计算摘要信息
	total := len(testReport.Status.Results)
	succeeded := 0
	failed := 0
	var totalResponseTime float64 = 0
	var minResponseTime float64 = 0
	var maxResponseTime float64 = 0
	hasResponseTimes := false

	// 统计成功和失败的测试数量，以及响应时间
	for _, result := range testReport.Status.Results {
		if result.Success {
			succeeded++
		} else {
			failed++
		}

		// 计算响应时间统计
		if result.ResponseTime > 0 {
			if !hasResponseTimes {
				// 初始化min和max为第一个有效值
				minResponseTime = result.ResponseTime
				maxResponseTime = result.ResponseTime
				hasResponseTimes = true
			} else {
				if result.ResponseTime < minResponseTime {
					minResponseTime = result.ResponseTime
				}
				if result.ResponseTime > maxResponseTime {
					maxResponseTime = result.ResponseTime
				}
			}
			totalResponseTime += result.ResponseTime
		}
	}

	// 计算平均响应时间
	var avgResponseTime float64 = 0
	if total > 0 && hasResponseTimes {
		// 只有当有有效响应时间时才计算平均值
		avgResponseTime = totalResponseTime / float64(succeeded)
	}

	// 更新Summary字段
	testReport.Status.Summary = testtoolsv1.TestSummary{
		Total:               total,
		Succeeded:           succeeded,
		Failed:              failed,
		AverageResponseTime: avgResponseTime,
		MinResponseTime:     minResponseTime,
		MaxResponseTime:     maxResponseTime,
	}

	// 使用乐观锁更新资源状态
	for i := 0; i < 3; i++ {
		err := r.Status().Update(ctx, testReport)
		if err == nil {
			logger.Info("成功更新TestReport状态",
				"testReport", testReport.Name,
				"summary", testReport.Status.Summary)
			return nil
		}

		if !errors.IsConflict(err) {
			logger.Error(err, "更新TestReport状态失败", "testReport", testReport.Name)
			return err
		}

		// 发生冲突，重新获取并应用更改
		var latestReport testtoolsv1.TestReport
		if err := r.Get(ctx, types.NamespacedName{Namespace: testReport.Namespace, Name: testReport.Name}, &latestReport); err != nil {
			logger.Error(err, "在冲突后重新获取TestReport失败", "testReport", testReport.Name)
			return err
		}

		// 复制状态更改到最新对象
		latestReport.Status.Summary = testReport.Status.Summary
		latestReport.Status.Phase = testReport.Status.Phase
		if testReport.Status.CompletionTime != nil {
			latestReport.Status.CompletionTime = testReport.Status.CompletionTime.DeepCopy()
		}

		// 更新指向最新对象
		testReport = &latestReport
		logger.Info("在冲突后重新尝试更新", "尝试", i+1)
	}

	return fmt.Errorf("在多次尝试后仍无法更新TestReport状态")
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

	maxRetries := 5
	retryDelay := 200 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		// 如果不是第一次尝试，重新获取对象
		if i > 0 {
			err := r.Get(ctx, client.ObjectKey{Name: testReport.Name, Namespace: testReport.Namespace}, testReport)
			if err != nil {
				logger.Error(err, "Failed to re-fetch TestReport for completion retry", "attempt", i+1)
				return err
			}
			logger.V(1).Info("Retrying TestReport completion", "attempt", i+1, "maxRetries", maxRetries)
		}

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
		if err := r.Status().Update(ctx, testReport); err != nil {
			if apierrors.IsConflict(err) {
				// 如果是冲突错误，进行重试
				logger.V(1).Info("Conflict completing TestReport, will retry",
					"attempt", i+1,
					"maxRetries", maxRetries,
					"error", err)
				if i < maxRetries-1 {
					time.Sleep(retryDelay)
					// 指数退避策略
					retryDelay *= 2
					continue
				}
			}
			logger.Error(err, "Failed to complete TestReport")
			return err
		}

		return nil
	}

	return fmt.Errorf("failed to complete TestReport after %d retries", maxRetries)
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

// isTestReportStatusEqual 检查两个TestReport状态是否相等
func (r *TestReportReconciler) isTestReportStatusEqual(a *testtoolsv1.TestReportStatus, b *testtoolsv1.TestReportStatus) bool {
	// 如果阶段不同，状态不同
	if a.Phase != b.Phase {
		return false
	}

	// 如果结果数量不同，状态不同
	if len(a.Results) != len(b.Results) {
		return false
	}

	// 检查汇总统计信息
	if a.Summary.Total != b.Summary.Total ||
		a.Summary.Succeeded != b.Summary.Succeeded ||
		a.Summary.Failed != b.Summary.Failed ||
		a.Summary.AverageResponseTime != b.Summary.AverageResponseTime {
		return false
	}

	// 对于具有相同数量的结果，简单检查最后一个结果的执行时间是否不同
	// 这是一个简化的检查，假设如果有新结果，它会添加到列表末尾
	// 或者替换列表中的一个条目，无论哪种情况，时间都会不同
	if len(a.Results) > 0 && len(b.Results) > 0 {
		lastA := a.Results[len(a.Results)-1]
		lastB := b.Results[len(b.Results)-1]
		if !lastA.ExecutionTime.Equal(&lastB.ExecutionTime) {
			return false
		}
	}

	// 如果通过了所有检查，则认为状态相同
	return true
}

// SetupWithManager sets up the controller with the Manager.
func (r *TestReportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// 设置事件记录器
	r.Recorder = mgr.GetEventRecorderFor("testreport-controller")

	// 使用完整版本，监听 TestReport、Fio、Dig和Ping资源
	return ctrl.NewControllerManagedBy(mgr).
		For(&testtoolsv1.TestReport{}).
		// 添加对 Fio 资源的监视
		Watches(
			&testtoolsv1.Fio{},
			handler.EnqueueRequestsFromMapFunc(r.findReportsForFio),
		).
		// 添加对 Dig 资源的监视
		Watches(
			&testtoolsv1.Dig{},
			handler.EnqueueRequestsFromMapFunc(r.findReportsForDig),
		).
		// 添加对 Ping 资源的监视
		Watches(
			&testtoolsv1.Ping{},
			handler.EnqueueRequestsFromMapFunc(r.findReportsForPing),
		).
		Complete(r)
}

// findReportsForFio 查找与 Fio 资源关联的所有 TestReport
func (r *TestReportReconciler) findReportsForFio(ctx context.Context, fio client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)
	fioObj, ok := fio.(*testtoolsv1.Fio)
	if !ok {
		logger.Error(fmt.Errorf("expected a Fio object but got %T", fio), "Failed to convert object to Fio")
		return nil
	}

	// 检查资源是否正在被删除
	if !fioObj.ObjectMeta.DeletionTimestamp.IsZero() {
		logger.Info("Fio资源正在被删除，不创建或更新TestReport",
			"fio", fioObj.Name,
			"namespace", fioObj.Namespace)
		return nil
	}

	// 如果TestReportName为空，创建新的TestReport
	if fioObj.Status.TestReportName == "" {
		// 尝试创建新的TestReport
		if err := r.CreateFioReport(ctx, fioObj); err != nil {
			if !errors.IsAlreadyExists(err) {
				// 只有当错误不是因为已存在时才记录，避免大量日志
				logger.Error(err, "创建Fio的TestReport失败", "fio", fioObj.Name, "namespace", fioObj.Namespace)
			}
			// 继续处理，因为即使创建失败，我们仍然可以尝试返回已存在的报告
		}
	}

	// 如果fioObj有TestReportName，返回对应的TestReport请求
	if fioObj.Status.TestReportName != "" {
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Name:      fioObj.Status.TestReportName,
					Namespace: fioObj.Namespace,
				},
			},
		}
	}

	// 尝试推断报告名称（即使Status.TestReportName为空）
	reportName := fmt.Sprintf("fio-%s-report", fioObj.Name)
	logger.Info("Fio没有设置TestReportName，使用推断的名称", "fio", fioObj.Name, "reportName", reportName)

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      reportName,
				Namespace: fioObj.Namespace,
			},
		},
	}
}

// updateFioStatus 使用重试机制更新Fio资源的状态
func (r *TestReportReconciler) updateFioStatus(ctx context.Context, name types.NamespacedName, updateFn func(*testtoolsv1.Fio)) error {
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

		// 保存旧状态用于比较
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

// CreateFioReport 为 Fio 资源创建一个 TestReport
func (r *TestReportReconciler) CreateFioReport(ctx context.Context, fio *testtoolsv1.Fio) error {
	logger := log.FromContext(ctx)

	// 生成报告名称
	reportName := fmt.Sprintf("fio-%s-report", fio.Name)

	// 首先检查Fio对象的TestReportName是否已经设置
	// 如果已经设置了相同的值，直接返回以避免重复操作
	if fio.Status.TestReportName == reportName {
		logger.Info("Fio已设置相同的TestReportName，无需操作",
			"fio", fio.Name,
			"reportName", reportName)
		return nil
	}

	// 检查是否已存在报告
	var existingReport testtoolsv1.TestReport
	err := r.Get(ctx, types.NamespacedName{
		Name:      reportName,
		Namespace: fio.Namespace,
	}, &existingReport)

	if err == nil {
		// 报告已存在，更新Fio对象关联
		logger.Info("TestReport已存在，将更新Fio的关联", "name", reportName, "namespace", fio.Namespace)

		// 使用updateFioStatus更新状态
		updateErr := r.updateFioStatus(ctx, types.NamespacedName{Name: fio.Name, Namespace: fio.Namespace}, func(fioObj *testtoolsv1.Fio) {
			fioObj.Status.TestReportName = reportName
		})

		if updateErr != nil {
			logger.Error(updateErr, "更新Fio的TestReportName失败",
				"fio", fio.Name,
				"namespace", fio.Namespace,
				"reportName", reportName)
			return updateErr
		}

		logger.Info("成功更新Fio的TestReportName",
			"fio", fio.Name,
			"namespace", fio.Namespace,
			"reportName", reportName)
		return nil
	} else if !errors.IsNotFound(err) {
		// 发生错误且不是因为资源不存在
		logger.Error(err, "检查TestReport是否存在时出错", "name", reportName, "namespace", fio.Namespace)
		return err
	}

	// 创建新的 TestReport
	testReport := &testtoolsv1.TestReport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reportName,
			Namespace: fio.Namespace,
		},
		Spec: testtoolsv1.TestReportSpec{
			TestName:     fmt.Sprintf("%s Fio Test", fio.Name),
			TestType:     "Performance",
			Target:       fio.Spec.FilePath,
			TestDuration: 1800, // 默认30分钟
			Interval:     60,   // 默认每60秒收集一次结果
			ResourceSelectors: []testtoolsv1.ResourceSelector{
				{
					APIVersion: "testtools.xiaoming.com/v1",
					Kind:       "Fio",
					Name:       fio.Name,
					Namespace:  fio.Namespace,
				},
			},
			AnalyticsConfig: &testtoolsv1.AnalyticsConfig{
				EnableTrendAnalysis:    true,
				EnableAnomalyDetection: true,
				HistoryLimit:           10,
			},
		},
	}

	// 使用SetControllerReference设置所有者引用
	if err := ctrl.SetControllerReference(fio, testReport, r.Scheme); err != nil {
		logger.Error(err, "设置所有者引用失败", "fio", fio.Name, "report", reportName)
		return err
	}

	// 使用重试机制创建TestReport
	maxRetries := 3
	retryDelay := 200 * time.Millisecond

	var createErr error
	for i := 0; i < maxRetries; i++ {
		createErr = r.Create(ctx, testReport)
		if createErr == nil {
			// 创建成功
			logger.Info("创建了新的TestReport",
				"fio", fio.Name,
				"namespace", fio.Namespace,
				"reportName", reportName)

			// 使用事件记录关联关系
			r.Recorder.Event(fio, corev1.EventTypeNormal, "TestReportCreated",
				fmt.Sprintf("创建了测试报告 %s", reportName))
			break
		}

		// 检查是否是因为已经存在
		if errors.IsAlreadyExists(createErr) {
			logger.Info("TestReport已经存在，将更新Fio关联", "fio", fio.Name, "reportName", reportName)
			createErr = nil
			break
		}

		// 如果是冲突，重试
		if errors.IsConflict(createErr) && i < maxRetries-1 {
			logger.Info("创建TestReport时发生冲突，将重试",
				"attempt", i+1,
				"maxRetries", maxRetries,
				"fio", fio.Name,
				"reportName", reportName)
			time.Sleep(retryDelay)
			retryDelay *= 2 // 指数增长
			continue
		}

		// 其他错误或达到最大重试次数
		logger.Error(createErr, "创建Fio的TestReport失败", "fio", fio.Name, "namespace", fio.Namespace)
		return createErr
	}

	// 如果创建失败但不是因为已存在，返回错误
	if createErr != nil && !errors.IsAlreadyExists(createErr) {
		return createErr
	}

	// 更新Fio的TestReportName字段，使用重试机制
	updateMaxRetries := 5
	updateRetryDelay := 100 * time.Millisecond
	var updateErr error

	for i := 0; i < updateMaxRetries; i++ {
		updateErr = r.updateFioStatus(ctx, types.NamespacedName{Name: fio.Name, Namespace: fio.Namespace}, func(fioObj *testtoolsv1.Fio) {
			fioObj.Status.TestReportName = reportName
		})

		if updateErr == nil {
			logger.Info("成功更新Fio的TestReportName",
				"fio", fio.Name,
				"reportName", reportName)
			break
		}

		// 如果是冲突，重试
		if errors.IsConflict(updateErr) && i < updateMaxRetries-1 {
			logger.Info("更新Fio的TestReportName时发生冲突，将重试",
				"attempt", i+1,
				"maxRetries", updateMaxRetries)
			time.Sleep(updateRetryDelay)
			updateRetryDelay *= 2 // 指数增长
			continue
		}

		logger.Error(updateErr, "创建TestReport后无法更新Fio的TestReportName字段",
			"fio", fio.Name,
			"reportName", reportName)
		// 不返回错误，因为TestReport已创建成功
		break
	}

	return nil
}

// findReportsForDig 查找与 Dig 资源关联的所有 TestReport
func (r *TestReportReconciler) findReportsForDig(ctx context.Context, dig client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)
	digObj, ok := dig.(*testtoolsv1.Dig)
	if !ok {
		logger.Error(fmt.Errorf("expected a Dig object but got %T", dig), "Failed to convert object to Dig")
		return nil
	}

	// 如果TestReportName为空，创建新的TestReport
	if digObj.Status.TestReportName == "" {
		// 尝试创建新的TestReport
		if err := r.CreateDigReport(ctx, digObj); err != nil {
			if !errors.IsAlreadyExists(err) {
				// 只有当错误不是因为已存在时才记录，避免大量日志
				logger.Error(err, "创建Dig的TestReport失败", "dig", digObj.Name, "namespace", digObj.Namespace)
			}
			// 继续处理，因为即使创建失败，我们仍然可以尝试返回已存在的报告
		}
	}

	// 如果digObj有TestReportName，返回对应的TestReport请求
	if digObj.Status.TestReportName != "" {
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Name:      digObj.Status.TestReportName,
					Namespace: digObj.Namespace,
				},
			},
		}
	}

	// 尝试推断报告名称（即使Status.TestReportName为空）
	reportName := fmt.Sprintf("dig-%s-report", digObj.Name)
	logger.Info("Dig没有设置TestReportName，使用推断的名称", "dig", digObj.Name, "reportName", reportName)

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      reportName,
				Namespace: digObj.Namespace,
			},
		},
	}
}

// updateDigStatus 使用乐观锁更新Dig状态
func (r *TestReportReconciler) updateDigStatus(ctx context.Context, name types.NamespacedName, updateFn func(*testtoolsv1.Dig)) error {
	logger := log.FromContext(ctx)
	retries := 3
	backoff := 100 * time.Millisecond

	for i := 0; i < retries; i++ {
		// 每次都获取最新版本
		var dig testtoolsv1.Dig
		if err := r.Get(ctx, name, &dig); err != nil {
			if errors.IsNotFound(err) {
				logger.Info("无法找到要更新的Dig资源", "name", name.Name, "namespace", name.Namespace)
				return err
			}
			logger.Error(err, "获取Dig资源失败", "name", name.Name, "namespace", name.Namespace)
			return err
		}

		// 深拷贝当前状态以便比较
		oldStatus := dig.Status.DeepCopy()

		// 应用更新函数
		updateFn(&dig)

		// 如果状态实际未变化，无需更新
		if reflect.DeepEqual(oldStatus, &dig.Status) {
			logger.Info("Dig状态未变化，跳过更新", "name", name.Name, "namespace", name.Namespace)
			return nil
		}

		// 尝试更新
		if err := r.Status().Update(ctx, &dig); err != nil {
			if errors.IsConflict(err) {
				// 冲突则重试
				logger.Info("更新Dig状态时发生冲突，准备重试",
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

			logger.Error(err, "更新Dig状态失败",
				"name", name.Name,
				"namespace", name.Namespace,
				"attempt", i+1)
			return err
		}

		logger.Info("成功更新Dig状态", "name", name.Name, "namespace", name.Namespace)
		return nil
	}

	return fmt.Errorf("在%d次尝试后仍无法更新Dig状态", retries)
}

// CreateDigReport 为 Dig 资源创建一个 TestReport
func (r *TestReportReconciler) CreateDigReport(ctx context.Context, dig *testtoolsv1.Dig) error {
	logger := log.FromContext(ctx)

	// 生成报告名称
	reportName := fmt.Sprintf("dig-%s-report", dig.Name)

	// 首先检查Dig对象的TestReportName是否已经设置
	// 如果已经设置了相同的值，直接返回以避免重复操作
	if dig.Status.TestReportName == reportName {
		logger.Info("Dig已设置相同的TestReportName，无需操作",
			"dig", dig.Name,
			"reportName", reportName)
		return nil
	}

	// 检查是否已存在报告
	var existingReport testtoolsv1.TestReport
	err := r.Get(ctx, types.NamespacedName{
		Name:      reportName,
		Namespace: dig.Namespace,
	}, &existingReport)

	if err == nil {
		// 报告已存在，更新Dig对象关联
		logger.Info("TestReport已存在，将更新Dig的关联", "name", reportName, "namespace", dig.Namespace)

		// 使用updateDigStatus更新状态
		updateErr := r.updateDigStatus(ctx, types.NamespacedName{Name: dig.Name, Namespace: dig.Namespace}, func(digObj *testtoolsv1.Dig) {
			digObj.Status.TestReportName = reportName
		})

		if updateErr != nil {
			logger.Error(updateErr, "更新Dig的TestReportName失败",
				"dig", dig.Name,
				"namespace", dig.Namespace,
				"reportName", reportName)
			return updateErr
		}

		logger.Info("成功更新Dig的TestReportName",
			"dig", dig.Name,
			"namespace", dig.Namespace,
			"reportName", reportName)
		return nil
	} else if !errors.IsNotFound(err) {
		// 发生错误且不是因为资源不存在
		logger.Error(err, "检查TestReport是否存在时出错", "name", reportName, "namespace", dig.Namespace)
		return err
	}

	// 创建新的 TestReport
	testReport := &testtoolsv1.TestReport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reportName,
			Namespace: dig.Namespace,
		},
		Spec: testtoolsv1.TestReportSpec{
			TestName:     fmt.Sprintf("%s Dig Test", dig.Name),
			TestType:     "DNS",
			Target:       fmt.Sprintf("%s DNS解析测试", dig.Spec.Domain),
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

	// 使用SetControllerReference设置所有者引用
	if err := ctrl.SetControllerReference(dig, testReport, r.Scheme); err != nil {
		logger.Error(err, "设置所有者引用失败", "dig", dig.Name, "report", reportName)
		return err
	}

	// 使用重试机制创建TestReport
	maxRetries := 3
	retryDelay := 200 * time.Millisecond

	var createErr error
	for i := 0; i < maxRetries; i++ {
		createErr = r.Create(ctx, testReport)
		if createErr == nil {
			// 创建成功
			logger.Info("创建了新的TestReport",
				"dig", dig.Name,
				"namespace", dig.Namespace,
				"reportName", reportName)

			// 使用事件记录关联关系
			r.Recorder.Event(dig, corev1.EventTypeNormal, "TestReportCreated",
				fmt.Sprintf("创建了测试报告 %s", reportName))
			break
		}

		// 检查是否是因为已经存在
		if errors.IsAlreadyExists(createErr) {
			logger.Info("TestReport已经存在，将更新Dig关联", "dig", dig.Name, "reportName", reportName)
			createErr = nil
			break
		}

		// 如果是冲突，重试
		if errors.IsConflict(createErr) && i < maxRetries-1 {
			logger.Info("创建TestReport时发生冲突，将重试",
				"attempt", i+1,
				"maxRetries", maxRetries,
				"dig", dig.Name,
				"reportName", reportName)
			time.Sleep(retryDelay)
			retryDelay *= 2 // 指数增长
			continue
		}

		// 其他错误或达到最大重试次数
		logger.Error(createErr, "创建Dig的TestReport失败", "dig", dig.Name, "namespace", dig.Namespace)
		return createErr
	}

	// 如果创建失败但不是因为已存在，返回错误
	if createErr != nil && !errors.IsAlreadyExists(createErr) {
		return createErr
	}

	// 更新Dig的TestReportName字段，使用重试机制
	updateMaxRetries := 5
	updateRetryDelay := 100 * time.Millisecond
	var updateErr error

	for i := 0; i < updateMaxRetries; i++ {
		updateErr = r.updateDigStatus(ctx, types.NamespacedName{Name: dig.Name, Namespace: dig.Namespace}, func(digObj *testtoolsv1.Dig) {
			digObj.Status.TestReportName = reportName
		})

		if updateErr == nil {
			logger.Info("成功更新Dig的TestReportName",
				"dig", dig.Name,
				"reportName", reportName)
			break
		}

		// 如果是冲突，重试
		if errors.IsConflict(updateErr) && i < updateMaxRetries-1 {
			logger.Info("更新Dig的TestReportName时发生冲突，将重试",
				"attempt", i+1,
				"maxRetries", updateMaxRetries)
			time.Sleep(updateRetryDelay)
			updateRetryDelay *= 2 // 指数增长
			continue
		}

		logger.Error(updateErr, "创建TestReport后无法更新Dig的TestReportName字段",
			"dig", dig.Name,
			"reportName", reportName)
		// 不返回错误，因为TestReport已创建成功
		break
	}

	return nil
}

// findReportsForPing 查找与 Ping 资源关联的所有 TestReport
func (r *TestReportReconciler) findReportsForPing(ctx context.Context, ping client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)
	pingObj, ok := ping.(*testtoolsv1.Ping)
	if !ok {
		logger.Error(fmt.Errorf("expected a Ping object but got %T", ping), "Failed to convert object to Ping")
		return nil
	}

	// 如果TestReportName为空，创建新的TestReport
	if pingObj.Status.TestReportName == "" {
		// 尝试创建新的TestReport
		if err := r.CreatePingReport(ctx, pingObj); err != nil {
			if !errors.IsAlreadyExists(err) {
				// 只有当错误不是因为已存在时才记录，避免大量日志
				logger.Error(err, "创建Ping的TestReport失败", "ping", pingObj.Name, "namespace", pingObj.Namespace)
			}
			// 继续处理，因为即使创建失败，我们仍然可以尝试返回已存在的报告
		}
	}

	// 如果pingObj有TestReportName，返回对应的TestReport请求
	if pingObj.Status.TestReportName != "" {
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Name:      pingObj.Status.TestReportName,
					Namespace: pingObj.Namespace,
				},
			},
		}
	}

	// 尝试推断报告名称（即使Status.TestReportName为空）
	reportName := fmt.Sprintf("ping-%s-report", pingObj.Name)
	logger.Info("Ping没有设置TestReportName，使用推断的名称", "ping", pingObj.Name, "reportName", reportName)

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      reportName,
				Namespace: pingObj.Namespace,
			},
		},
	}
}

// CreatePingReport 为 Ping 资源创建一个 TestReport
func (r *TestReportReconciler) CreatePingReport(ctx context.Context, ping *testtoolsv1.Ping) error {
	logger := log.FromContext(ctx)

	// 生成报告名称
	reportName := fmt.Sprintf("ping-%s-report", ping.Name)

	// 首先检查Ping对象的TestReportName是否已经设置
	// 如果已经设置了相同的值，直接返回以避免重复操作
	if ping.Status.TestReportName == reportName {
		logger.Info("Ping已设置相同的TestReportName，无需操作",
			"ping", ping.Name,
			"reportName", reportName)
		return nil
	}

	// 检查是否已存在报告
	var existingReport testtoolsv1.TestReport
	err := r.Get(ctx, types.NamespacedName{
		Name:      reportName,
		Namespace: ping.Namespace,
	}, &existingReport)

	if err == nil {
		// 报告已存在，更新Ping对象关联
		logger.Info("TestReport已存在，将更新Ping的关联", "name", reportName, "namespace", ping.Namespace)

		// 使用updatePingStatus更新状态
		updateErr := r.updatePingStatus(ctx, types.NamespacedName{Name: ping.Name, Namespace: ping.Namespace}, func(pingObj *testtoolsv1.Ping) {
			pingObj.Status.TestReportName = reportName
		})

		if updateErr != nil {
			logger.Error(updateErr, "更新Ping的TestReportName失败",
				"ping", ping.Name,
				"namespace", ping.Namespace,
				"reportName", reportName)
			return updateErr
		}

		logger.Info("成功更新Ping的TestReportName",
			"ping", ping.Name,
			"namespace", ping.Namespace,
			"reportName", reportName)
		return nil
	} else if !errors.IsNotFound(err) {
		// 发生错误
		logger.Error(err, "检查TestReport是否存在时出错", "name", reportName, "namespace", ping.Namespace)
		return err
	}

	// 创建新的 TestReport
	testReport := &testtoolsv1.TestReport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reportName,
			Namespace: ping.Namespace,
		},
		Spec: testtoolsv1.TestReportSpec{
			TestName:     fmt.Sprintf("%s Ping Test", ping.Name),
			TestType:     "Network",
			Target:       fmt.Sprintf("%s 网络可达性测试", ping.Spec.Host),
			TestDuration: 1800, // 默认30分钟
			Interval:     60,   // 默认每60秒收集一次结果
			ResourceSelectors: []testtoolsv1.ResourceSelector{
				{
					APIVersion: "testtools.xiaoming.com/v1",
					Kind:       "Ping",
					Name:       ping.Name,
					Namespace:  ping.Namespace,
				},
			},
		},
	}

	// 使用SetControllerReference设置所有者引用
	if err := ctrl.SetControllerReference(ping, testReport, r.Scheme); err != nil {
		logger.Error(err, "设置所有者引用失败", "ping", ping.Name, "report", reportName)
		return err
	}

	// 使用重试机制创建TestReport
	maxRetries := 3
	retryDelay := 200 * time.Millisecond

	var createErr error
	for i := 0; i < maxRetries; i++ {
		createErr = r.Create(ctx, testReport)
		if createErr == nil {
			// 创建成功
			logger.Info("创建了新的TestReport",
				"ping", ping.Name,
				"namespace", ping.Namespace,
				"reportName", reportName)

			// 使用事件记录关联关系
			r.Recorder.Event(ping, corev1.EventTypeNormal, "TestReportCreated",
				fmt.Sprintf("创建了测试报告 %s", reportName))
			break
		}

		// 检查是否是因为已经存在
		if errors.IsAlreadyExists(createErr) {
			logger.Info("TestReport已经存在，将更新Ping关联", "ping", ping.Name, "reportName", reportName)
			createErr = nil
			break
		}

		// 如果是冲突，重试
		if errors.IsConflict(createErr) && i < maxRetries-1 {
			logger.Info("创建TestReport时发生冲突，将重试",
				"attempt", i+1,
				"maxRetries", maxRetries,
				"ping", ping.Name,
				"reportName", reportName)
			time.Sleep(retryDelay)
			retryDelay *= 2 // 指数增长
			continue
		}

		// 其他错误或达到最大重试次数
		logger.Error(createErr, "创建Ping的TestReport失败", "ping", ping.Name, "namespace", ping.Namespace)
		return createErr
	}

	// 如果创建失败但不是因为已存在，返回错误
	if createErr != nil && !errors.IsAlreadyExists(createErr) {
		return createErr
	}

	// 更新Ping的TestReportName字段，使用重试机制
	updateMaxRetries := 5
	updateRetryDelay := 100 * time.Millisecond
	var updateErr error

	for i := 0; i < updateMaxRetries; i++ {
		updateErr = r.updatePingStatus(ctx, types.NamespacedName{Name: ping.Name, Namespace: ping.Namespace}, func(pingObj *testtoolsv1.Ping) {
			pingObj.Status.TestReportName = reportName
		})

		if updateErr == nil {
			logger.Info("成功更新Ping的TestReportName",
				"ping", ping.Name,
				"reportName", reportName)
			break
		}

		// 如果是冲突，重试
		if errors.IsConflict(updateErr) && i < updateMaxRetries-1 {
			logger.Info("更新Ping的TestReportName时发生冲突，将重试",
				"attempt", i+1,
				"maxRetries", updateMaxRetries)
			time.Sleep(updateRetryDelay)
			updateRetryDelay *= 2 // 指数增长
			continue
		}

		logger.Error(updateErr, "创建TestReport后无法更新Ping的TestReportName字段",
			"ping", ping.Name,
			"reportName", reportName)
		// 不返回错误，因为TestReport已创建成功
		break
	}

	return nil
}

// addPingResult adds a Ping resource's result to the test report
func (r *TestReportReconciler) addPingResult(testReport *testtoolsv1.TestReport, ping *testtoolsv1.Ping) error {
	// Skip if there's no execution time
	if ping.Status.LastExecutionTime == nil {
		return nil
	}

	// Create a new test result
	result := testtoolsv1.TestResult{
		ResourceName:      ping.Name,
		ResourceNamespace: ping.Namespace,
		ResourceKind:      "Ping",
		ExecutionTime:     *ping.Status.LastExecutionTime,
		Success:           ping.Status.Status == "Succeeded",
		ResponseTime:      ping.Status.AvgRtt,
		Output:            extractPingSummary(ping.Status.LastResult),
		RawOutput:         ping.Status.LastResult,
	}

	// Extract metrics
	result.MetricValues = map[string]string{
		"QueryCount":   fmt.Sprintf("%d", ping.Status.QueryCount),
		"SuccessCount": fmt.Sprintf("%d", ping.Status.SuccessCount),
		"FailureCount": fmt.Sprintf("%d", ping.Status.FailureCount),
		"PacketLoss":   fmt.Sprintf("%.2f", ping.Status.PacketLoss),
		"MinRtt":       fmt.Sprintf("%.2f", ping.Status.MinRtt),
		"AvgRtt":       fmt.Sprintf("%.2f", ping.Status.AvgRtt),
		"MaxRtt":       fmt.Sprintf("%.2f", ping.Status.MaxRtt),
	}

	// 记录添加的结果详情
	log.FromContext(context.Background()).Info("添加Ping测试结果到报告",
		"report", testReport.Name,
		"ping", ping.Name,
		"status", ping.Status.Status,
		"packetLoss", ping.Status.PacketLoss,
		"avgRtt", ping.Status.AvgRtt,
		"maxRtt", ping.Status.MaxRtt)

	// Find if there's an existing result for the same resource
	found := false
	for i, existing := range testReport.Status.Results {
		if existing.ResourceName == result.ResourceName &&
			existing.ResourceNamespace == result.ResourceNamespace &&
			existing.ResourceKind == result.ResourceKind {
			// Replace the existing result
			testReport.Status.Results[i] = result
			found = true
			break
		}
	}

	// If no existing result was found, add a new one
	if !found {
		testReport.Status.Results = append(testReport.Status.Results, result)
	}

	// 更新测试报告摘要，确保计数正确
	testReport.Status.Summary.Total = len(testReport.Status.Results)

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

// updatePingStatus 使用乐观锁更新Ping状态
func (r *TestReportReconciler) updatePingStatus(ctx context.Context, name types.NamespacedName, updateFn func(*testtoolsv1.Ping)) error {
	logger := log.FromContext(ctx)
	retries := 3
	backoff := 100 * time.Millisecond

	for i := 0; i < retries; i++ {
		// 每次都获取最新版本
		var ping testtoolsv1.Ping
		if err := r.Get(ctx, name, &ping); err != nil {
			if errors.IsNotFound(err) {
				logger.Info("无法找到要更新的Ping资源", "name", name.Name, "namespace", name.Namespace)
				return err
			}
			logger.Error(err, "获取Ping资源失败", "name", name.Name, "namespace", name.Namespace)
			return err
		}

		// 深拷贝当前状态以便比较
		oldStatus := ping.Status.DeepCopy()

		// 应用更新函数
		updateFn(&ping)

		// 如果状态实际未变化，无需更新
		if reflect.DeepEqual(oldStatus, &ping.Status) {
			logger.Info("Ping状态未变化，跳过更新", "name", name.Name, "namespace", name.Namespace)
			return nil
		}

		// 尝试更新
		if err := r.Status().Update(ctx, &ping); err != nil {
			if errors.IsConflict(err) {
				// 冲突则重试
				logger.Info("更新Ping状态时发生冲突，准备重试",
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

			logger.Error(err, "更新Ping状态失败",
				"name", name.Name,
				"namespace", name.Namespace,
				"attempt", i+1)
			return err
		}

		logger.Info("成功更新Ping状态", "name", name.Name, "namespace", name.Namespace)
		return nil
	}

	return fmt.Errorf("在%d次尝试后仍无法更新Ping状态", retries)
}
