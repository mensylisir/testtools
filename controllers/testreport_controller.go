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

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	"github.com/xiaoming/testtools/pkg/analytics"
	"github.com/xiaoming/testtools/pkg/exporters"
	"github.com/xiaoming/testtools/pkg/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// TestReportReconciler reconciles a TestReport object
type TestReportReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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

	// Process each resource selector
	for _, selector := range testReport.Spec.ResourceSelectors {
		// 处理 Dig 资源
		if selector.Kind == "Dig" && selector.APIVersion == "testtools.xiaoming.com/v1" {
			if err := r.collectDigResults(ctx, testReport, selector, logger); err != nil {
				logger.Error(err, "Failed to collect Dig results", "selector", selector)
				continue
			}
		}

		// 处理 Fio 资源
		if selector.Kind == "Fio" && selector.APIVersion == "testtools.xiaoming.com/v1" {
			if err := r.collectFioResults(ctx, testReport, selector, logger); err != nil {
				logger.Error(err, "Failed to collect Fio results", "selector", selector)
				continue
			}
		}

		// 可以在这里添加对其他资源类型的支持
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

// collectFioResults 收集 Fio 资源的结果
func (r *TestReportReconciler) collectFioResults(ctx context.Context, testReport *testtoolsv1.TestReport, selector testtoolsv1.ResourceSelector, logger logr.Logger) error {
	logger.Info("开始收集FIO结果", "selector", selector)
	namespace := selector.Namespace
	if namespace == "" {
		namespace = testReport.Namespace
	}

	// 如果提供了特定名称，获取该特定的 Fio 资源
	if selector.Name != "" {
		var fio testtoolsv1.Fio
		err := r.Get(ctx, types.NamespacedName{Name: selector.Name, Namespace: namespace}, &fio)
		if err != nil {
			logger.Error(err, "获取指定的FIO资源失败",
				"name", selector.Name,
				"namespace", namespace)
			return err
		}

		logger.Info("找到指定的FIO资源",
			"name", fio.Name,
			"namespace", fio.Namespace,
			"lastExecutionTime", fio.Status.LastExecutionTime)

		// 确保该 Fio 资源的 TestReportName 与当前 TestReport 匹配
		if fio.Status.TestReportName == "" {
			fioCopy := fio.DeepCopy()
			fioCopy.Status.TestReportName = testReport.Name
			if err := r.Status().Update(ctx, fioCopy); err != nil {
				logger.Error(err, "更新FIO资源的TestReportName失败",
					"name", fio.Name,
					"namespace", fio.Namespace,
					"reportName", testReport.Name)
			} else {
				logger.Info("已更新FIO资源的TestReportName",
					"name", fio.Name,
					"namespace", fio.Namespace,
					"reportName", testReport.Name)
				// 重新获取更新后的对象
				if err := r.Get(ctx, types.NamespacedName{Name: selector.Name, Namespace: namespace}, &fio); err != nil {
					logger.Error(err, "重新获取FIO资源失败")
				}
			}
		}

		if err := r.addFioResult(testReport, &fio); err != nil {
			logger.Error(err, "添加FIO结果失败",
				"name", fio.Name,
				"namespace", fio.Namespace)
			return err
		}

		logger.Info("成功添加FIO结果",
			"name", fio.Name,
			"namespace", fio.Namespace,
			"resultsCount", len(testReport.Status.Results))
		return nil
	}

	// 否则，根据标签选择器列出 Fio 资源
	var fioList testtoolsv1.FioList
	listOpts := []client.ListOption{client.InNamespace(namespace)}

	if selector.LabelSelector != nil && len(selector.LabelSelector.MatchLabels) > 0 {
		labels := client.MatchingLabels(selector.LabelSelector.MatchLabels)
		listOpts = append(listOpts, labels)
	}

	if err := r.List(ctx, &fioList, listOpts...); err != nil {
		logger.Error(err, "获取FIO资源列表失败",
			"namespace", namespace,
			"labelSelector", selector.LabelSelector)
		return err
	}

	logger.Info("找到FIO资源列表", "count", len(fioList.Items))

	// 处理每个 Fio 资源
	for i := range fioList.Items {
		fio := &fioList.Items[i]

		// 确保该 Fio 资源的 TestReportName 与当前 TestReport 匹配
		if fio.Status.TestReportName == "" {
			fioCopy := fio.DeepCopy()
			fioCopy.Status.TestReportName = testReport.Name
			if err := r.Status().Update(ctx, fioCopy); err != nil {
				logger.Error(err, "更新FIO资源的TestReportName失败",
					"name", fio.Name,
					"namespace", fio.Namespace,
					"reportName", testReport.Name)
				continue
			} else {
				logger.Info("已更新FIO资源的TestReportName",
					"name", fio.Name,
					"namespace", fio.Namespace,
					"reportName", testReport.Name)
				// 重新获取更新后的对象
				if err := r.Get(ctx, types.NamespacedName{Name: fio.Name, Namespace: fio.Namespace}, fio); err != nil {
					logger.Error(err, "重新获取FIO资源失败")
					continue
				}
			}
		}

		if err := r.addFioResult(testReport, fio); err != nil {
			logger.Error(err, "添加FIO结果失败",
				"name", fio.Name,
				"namespace", fio.Namespace)
		} else {
			logger.Info("成功添加FIO结果",
				"name", fio.Name,
				"namespace", fio.Namespace)
		}
	}

	logger.Info("FIO结果收集完成",
		"resultsCount", len(testReport.Status.Results))
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
	// 如果没有执行时间，跳过
	if fio.Status.LastExecutionTime == nil {
		return nil
	}

	// 创建新的测试结果
	result := testtoolsv1.TestResult{
		ResourceName:      fio.Name,
		ResourceNamespace: fio.Namespace,
		ResourceKind:      "Fio",
		ExecutionTime:     *fio.Status.LastExecutionTime,
		Success:           fio.Status.Status == "Succeeded",
		// 使用总 IOPS 作为响应时间的替代指标
		ResponseTime: fio.Status.Stats.ReadIOPS + fio.Status.Stats.WriteIOPS,
		Output:       extractFioSummary(fio.Status.LastResult),
		RawOutput:    fio.Status.LastResult,
	}

	// 提取指标
	result.MetricValues = map[string]string{
		"QueryCount":   fmt.Sprintf("%d", fio.Status.QueryCount),
		"SuccessCount": fmt.Sprintf("%d", fio.Status.SuccessCount),
		"FailureCount": fmt.Sprintf("%d", fio.Status.FailureCount),
		"ReadIOPS":     fmt.Sprintf("%.2f", fio.Status.Stats.ReadIOPS),
		"WriteIOPS":    fmt.Sprintf("%.2f", fio.Status.Stats.WriteIOPS),
		"ReadBW":       fmt.Sprintf("%.2f", fio.Status.Stats.ReadBW),
		"WriteBW":      fmt.Sprintf("%.2f", fio.Status.Stats.WriteBW),
		"ReadLatency":  fmt.Sprintf("%.2f", fio.Status.Stats.ReadLatency),
		"WriteLatency": fmt.Sprintf("%.2f", fio.Status.Stats.WriteLatency),
	}

	// 添加百分位延迟指标
	for percentile, value := range fio.Status.Stats.LatencyPercentiles {
		result.MetricValues[fmt.Sprintf("latency_%s", percentile)] = fmt.Sprintf("%.2f", value)
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

// updateTestReport updates the test report status
func (r *TestReportReconciler) updateTestReport(ctx context.Context, testReport *testtoolsv1.TestReport, logger logr.Logger) error {
	logger.V(1).Info("Updating TestReport status", "namespace", testReport.Namespace, "name", testReport.Name)

	oldStatus := testReport.Status.DeepCopy()

	// Update summary metrics
	testReport.Status.Summary.Total = len(testReport.Status.Results)
	testReport.Status.Summary.Succeeded = 0
	testReport.Status.Summary.Failed = 0
	totalResponseTime := 0.0

	for _, result := range testReport.Status.Results {
		if result.Success {
			testReport.Status.Summary.Succeeded++
		} else {
			testReport.Status.Summary.Failed++
		}
		totalResponseTime += result.ResponseTime
	}

	// Calculate average response time
	if testReport.Status.Summary.Total > 0 {
		testReport.Status.Summary.AverageResponseTime = totalResponseTime / float64(testReport.Status.Summary.Total)
	}

	// 执行趋势分析
	// TODO: 添加趋势分析功能
	// r.analyzeTrends(ctx, testReport)

	// 执行异常检测
	// TODO: 添加异常检测功能
	// r.detectAnomalies(ctx, testReport)

	// 如果状态相同，无需更新
	if r.isTestReportStatusEqual(oldStatus, &testReport.Status) {
		logger.V(1).Info("TestReport status unchanged, skipping update")
		return nil
	}

	// 更新状态
	if err := r.Status().Update(ctx, testReport); err != nil {
		logger.Error(err, "Failed to update TestReport status")
		return err
	}

	// 导出指标到外部系统
	// TODO: 添加指标导出功能
	// r.exportMetrics(ctx, testReport)

	logger.Info("TestReport status updated", "phase", testReport.Status.Phase)
	return nil
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
	// 如果 Fio 资源没有设置 TestReportName，则无法关联
	fioObj, ok := fio.(*testtoolsv1.Fio)
	if !ok {
		log.FromContext(ctx).Info("资源不是Fio类型")
		return nil
	}

	if fioObj.Status.TestReportName == "" {
		log.FromContext(ctx).Info("Fio资源没有设置TestReportName",
			"name", fioObj.Name,
			"namespace", fioObj.Namespace)

		// 尝试主动设置TestReportName
		reportName := fmt.Sprintf("fio-%s-report", fioObj.Name)
		fioCopy := fioObj.DeepCopy()
		fioCopy.Status.TestReportName = reportName

		if err := r.Status().Update(ctx, fioCopy); err != nil {
			log.FromContext(ctx).Error(err, "尝试设置Fio的TestReportName失败",
				"name", fioObj.Name,
				"namespace", fioObj.Namespace)
		} else {
			log.FromContext(ctx).Info("已为Fio资源设置TestReportName",
				"name", fioObj.Name,
				"namespace", fioObj.Namespace,
				"reportName", reportName)
		}
		return nil
	}

	logger := log.FromContext(ctx)
	logger.Info("处理Fio资源关联的TestReport",
		"fio", fioObj.Name,
		"namespace", fioObj.Namespace,
		"TestReportName", fioObj.Status.TestReportName)

	// 检查是否需要创建 TestReport
	var testReport testtoolsv1.TestReport
	err := r.Get(ctx, types.NamespacedName{
		Name:      fioObj.Status.TestReportName,
		Namespace: fioObj.Namespace,
	}, &testReport)

	// 如果 TestReport 不存在，创建它
	if err != nil && apierrors.IsNotFound(err) {
		logger.Info("TestReport不存在，准备创建",
			"reportName", fioObj.Status.TestReportName,
			"fio", fioObj.Name,
			"namespace", fioObj.Namespace)

		if err := r.CreateFioReport(ctx, fioObj); err != nil {
			logger.Error(err, "创建Fio的TestReport失败",
				"fio", fioObj.Name,
				"namespace", fioObj.Namespace,
				"reportName", fioObj.Status.TestReportName)
		} else {
			logger.Info("成功创建Fio的TestReport",
				"fio", fioObj.Name,
				"namespace", fioObj.Namespace,
				"reportName", fioObj.Status.TestReportName)
		}
	} else if err != nil {
		logger.Error(err, "获取TestReport时出错",
			"reportName", fioObj.Status.TestReportName,
			"fio", fioObj.Name,
			"namespace", fioObj.Namespace)
	} else {
		logger.Info("找到现有TestReport",
			"reportName", fioObj.Status.TestReportName,
			"fio", fioObj.Name,
			"namespace", fioObj.Namespace)
	}

	// 返回关联的 TestReport 请求
	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      fioObj.Status.TestReportName,
				Namespace: fioObj.Namespace,
			},
		},
	}
}

// CreateFioReport 为 Fio 资源创建一个 TestReport
func (r *TestReportReconciler) CreateFioReport(ctx context.Context, fio *testtoolsv1.Fio) error {
	logger := log.FromContext(ctx)

	// 使用 Fio 资源中已经设置的 TestReportName
	reportName := fio.Status.TestReportName
	if reportName == "" {
		// 如果不存在，则生成一个
		reportName = fmt.Sprintf("fio-%s-report", fio.Name)
		logger.Info("Fio资源未设置TestReportName，使用生成的名称",
			"name", fio.Name,
			"namespace", fio.Namespace,
			"reportName", reportName)
	}

	// 检查是否已存在报告
	var existingReport testtoolsv1.TestReport
	err := r.Get(ctx, types.NamespacedName{
		Name:      reportName,
		Namespace: fio.Namespace,
	}, &existingReport)

	if err == nil {
		// 报告已存在，不需要创建
		logger.Info("TestReport已存在", "name", reportName, "namespace", fio.Namespace)
		return nil
	} else if !errors.IsNotFound(err) {
		// 发生错误
		logger.Error(err, "检查TestReport是否存在时出错", "name", reportName, "namespace", fio.Namespace)
		return err
	}

	logger.Info("创建新的TestReport", "name", reportName, "namespace", fio.Namespace)

	// 创建新的测试报告
	testReport := &testtoolsv1.TestReport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reportName,
			Namespace: fio.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: testtoolsv1.GroupVersion.String(),
					Kind:       "Fio",
					Name:       fio.Name,
					UID:        fio.UID,
					Controller: func() *bool { b := true; return &b }(),
				},
			},
		},
		Spec: testtoolsv1.TestReportSpec{
			TestName: fmt.Sprintf("fio-%s", fio.Name),
			TestType: "Performance",
			Target:   fio.Spec.FilePath,
			ResourceSelectors: []testtoolsv1.ResourceSelector{
				{
					APIVersion: testtoolsv1.GroupVersion.String(),
					Kind:       "Fio",
					Name:       fio.Name,
					Namespace:  fio.Namespace,
				},
			},
		},
	}

	// 创建报告
	err = r.Create(ctx, testReport)
	if err != nil {
		logger.Error(err, "创建Fio的TestReport失败", "fio", fio.Name, "namespace", fio.Namespace, "reportName", reportName)
		return err
	}

	logger.Info("成功创建Fio的TestReport", "fio", fio.Name, "report", reportName, "namespace", fio.Namespace)

	// 确保 Fio 资源的 TestReportName 已设置
	if fio.Status.TestReportName != reportName {
		fioCopy := fio.DeepCopy()
		fioCopy.Status.TestReportName = reportName
		if err := r.Status().Update(ctx, fioCopy); err != nil {
			logger.Error(err, "更新Fio的TestReportName失败",
				"fio", fio.Name,
				"namespace", fio.Namespace,
				"reportName", reportName)
		}
	}

	return nil
}

// findReportsForDig 查找与 Dig 资源关联的所有 TestReport
func (r *TestReportReconciler) findReportsForDig(ctx context.Context, dig client.Object) []reconcile.Request {
	// 如果 Dig 资源没有设置 TestReportName，则无法关联
	digObj, ok := dig.(*testtoolsv1.Dig)
	if !ok || digObj.Status.TestReportName == "" {
		return nil
	}

	logger := log.FromContext(ctx)

	// 检查是否需要创建 TestReport
	var testReport testtoolsv1.TestReport
	err := r.Get(ctx, types.NamespacedName{
		Name:      digObj.Status.TestReportName,
		Namespace: digObj.Namespace,
	}, &testReport)

	// 如果 TestReport 不存在，创建它
	if err != nil && apierrors.IsNotFound(err) {
		if err := r.CreateDigReport(ctx, digObj); err != nil {
			logger.Error(err, "Failed to create TestReport for Dig",
				"dig", digObj.Name,
				"namespace", digObj.Namespace)
		}
	}

	// 返回关联的 TestReport 请求
	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      digObj.Status.TestReportName,
				Namespace: digObj.Namespace,
			},
		},
	}
}

// CreateDigReport 为 Dig 资源创建一个 TestReport
func (r *TestReportReconciler) CreateDigReport(ctx context.Context, dig *testtoolsv1.Dig) error {
	logger := log.FromContext(ctx)

	// 确保 TestReportName 已设置
	if dig.Status.TestReportName == "" {
		return fmt.Errorf("dig resource %s/%s does not have TestReportName set", dig.Namespace, dig.Name)
	}

	// 创建新的 TestReport
	testReport := testtoolsv1.TestReport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dig.Status.TestReportName,
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
			Target:       fmt.Sprintf("%s DNS解析测试", dig.Spec.Domain),
			TestDuration: 3600, // 默认60分钟
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

	// 创建 TestReport
	if err := r.Create(ctx, &testReport); err != nil {
		return err
	}

	logger.Info("Created new TestReport for Dig",
		"dig", dig.Name,
		"namespace", dig.Namespace,
		"reportName", dig.Status.TestReportName)

	return nil
}

// findReportsForPing 查找与 Ping 资源关联的所有 TestReport
func (r *TestReportReconciler) findReportsForPing(ctx context.Context, ping client.Object) []reconcile.Request {
	// 如果 Ping 资源没有设置 TestReportName，则无法关联
	pingObj, ok := ping.(*testtoolsv1.Ping)
	if !ok || pingObj.Status.TestReportName == "" {
		return nil
	}

	logger := log.FromContext(ctx)

	// 检查是否需要创建 TestReport
	var testReport testtoolsv1.TestReport
	err := r.Get(ctx, types.NamespacedName{
		Name:      pingObj.Status.TestReportName,
		Namespace: pingObj.Namespace,
	}, &testReport)

	// 如果 TestReport 不存在，创建它
	if err != nil && apierrors.IsNotFound(err) {
		if err := r.CreatePingReport(ctx, pingObj); err != nil {
			logger.Error(err, "Failed to create TestReport for Ping",
				"ping", pingObj.Name,
				"namespace", pingObj.Namespace)
		}
	}

	// 返回关联的 TestReport 请求
	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      pingObj.Status.TestReportName,
				Namespace: pingObj.Namespace,
			},
		},
	}
}

// CreatePingReport 为 Ping 资源创建一个 TestReport
func (r *TestReportReconciler) CreatePingReport(ctx context.Context, ping *testtoolsv1.Ping) error {
	logger := log.FromContext(ctx)

	// 确保 TestReportName 已设置
	if ping.Status.TestReportName == "" {
		return fmt.Errorf("ping resource %s/%s does not have TestReportName set", ping.Namespace, ping.Name)
	}

	// 创建新的 TestReport
	testReport := testtoolsv1.TestReport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ping.Status.TestReportName,
			Namespace: ping.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: ping.APIVersion,
					Kind:       ping.Kind,
					Name:       ping.Name,
					UID:        ping.UID,
					Controller: func() *bool { b := true; return &b }(),
				},
			},
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

	// 创建 TestReport
	if err := r.Create(ctx, &testReport); err != nil {
		return err
	}

	logger.Info("Created new TestReport for Ping",
		"ping", ping.Name,
		"namespace", ping.Namespace,
		"reportName", ping.Status.TestReportName)

	return nil
}
