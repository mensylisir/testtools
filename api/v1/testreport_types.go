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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestReportSpec defines the desired state of TestReport
type TestReportSpec struct {
	// TestName is the name of the test
	// +kubebuilder:validation:Required
	TestName string `json:"testName"`

	// TestType is the type of test
	// +kubebuilder:validation:Enum=DNS;Network;Performance;PING
	// +kubebuilder:validation:Required
	TestType string `json:"testType"`

	// Target is the target of the test (e.g., domain, IP, service)
	// +kubebuilder:validation:Required
	Target string `json:"target"`

	// TestDuration is the duration of the test in seconds
	// +optional
	TestDuration int `json:"testDuration,omitempty"`

	// Interval is the interval between test runs in seconds
	// +optional
	Interval int `json:"interval,omitempty"`

	// Parameters contains additional test parameters
	// +optional
	Parameters map[string]string `json:"parameters,omitempty"`

	// ResourceSelectors defines which resources to collect results from
	// +optional
	ResourceSelectors []ResourceSelector `json:"resourceSelectors,omitempty"`

	// AnalyticsConfig配置分析选项
	// +optional
	AnalyticsConfig *AnalyticsConfig `json:"analyticsConfig,omitempty"`

	// AlertConfig配置告警规则
	// +optional
	AlertConfig *AlertConfig `json:"alertConfig,omitempty"`

	// ExportConfig配置数据导出选项
	// +optional
	ExportConfig *ExportConfig `json:"exportConfig,omitempty"`
}

// ResourceSelector selects resources to include in the test report
type ResourceSelector struct {
	// APIVersion of the resource
	// +kubebuilder:validation:Required
	APIVersion string `json:"apiVersion"`

	// Kind of the resource
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`

	// Name of the resource
	// +optional
	Name string `json:"name,omitempty"`

	// Namespace of the resource
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// LabelSelector for the resources
	// +optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`
}

// TestResult contains a single test result
type TestResult struct {
	// ResourceName is the name of the resource
	ResourceName string `json:"resourceName"`

	// ResourceNamespace is the namespace of the resource
	ResourceNamespace string `json:"resourceNamespace"`

	// ResourceKind is the kind of the resource
	ResourceKind string `json:"resourceKind"`

	// ExecutionTime is when the test was executed
	ExecutionTime metav1.Time `json:"executionTime"`

	// Success indicates if the test was successful
	Success bool `json:"success"`

	// ResponseTime is the response time in milliseconds
	// +optional
	ResponseTime float64 `json:"responseTime,omitempty"`

	// Output contains the formatted test output
	// +optional
	Output string `json:"output,omitempty"`

	// RawOutput contains the original unformatted output
	// +optional
	RawOutput string `json:"rawOutput,omitempty"`

	// Error contains the error message if test failed
	// +optional
	Error string `json:"error,omitempty"`

	// MetricValues contains test metrics
	// +optional
	MetricValues map[string]string `json:"metricValues,omitempty"`
}

// TestReportStatus defines the observed state of TestReport
type TestReportStatus struct {
	// Phase is the current phase of the test report
	// +optional
	Phase string `json:"phase,omitempty"`

	// StartTime is the time when test collection started
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is the time when test collection completed
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Results contains the test results
	// +optional
	Results []TestResult `json:"results,omitempty"`

	// Summary contains summary metrics
	// +optional
	Summary TestSummary `json:"summary,omitempty"`

	// Analytics包含性能分析结果
	// +optional
	Analytics *Analytics `json:"analytics,omitempty"`

	// Anomalies包含检测到的异常
	// +optional
	Anomalies []Anomaly `json:"anomalies,omitempty"`

	// Conditions represents the latest available observations of the test report's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// TestSummary contains summary metrics for the test report
type TestSummary struct {
	// Total number of tests run
	Total int `json:"total"`

	// Number of successful tests
	Succeeded int `json:"succeeded"`

	// Number of failed tests
	Failed int `json:"failed"`

	// Average response time across all tests in milliseconds
	// +optional
	AverageResponseTime float64 `json:"averageResponseTime,omitempty"`

	// Min response time across all tests in milliseconds
	// +optional
	MinResponseTime float64 `json:"minResponseTime,omitempty"`

	// Max response time across all tests in milliseconds
	// +optional
	MaxResponseTime float64 `json:"maxResponseTime,omitempty"`

	// Additional metrics
	// +optional
	Metrics map[string]string `json:"metrics,omitempty"`
}

// AnalyticsConfig配置分析选项
type AnalyticsConfig struct {
	// EnableTrendAnalysis启用趋势分析
	// +optional
	EnableTrendAnalysis bool `json:"enableTrendAnalysis,omitempty"`

	// EnableAnomalyDetection启用异常检测
	// +optional
	EnableAnomalyDetection bool `json:"enableAnomalyDetection,omitempty"`

	// ComparisonBaseline指定比较基准
	// +optional
	ComparisonBaseline string `json:"comparisonBaseline,omitempty"`

	// HistoryLimit设置保留的历史记录数量
	// +optional
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	HistoryLimit int `json:"historyLimit,omitempty"`
}

// AlertConfig配置告警规则
type AlertConfig struct {
	// Thresholds设置各指标的告警阈值
	// +optional
	Thresholds map[string]float64 `json:"thresholds,omitempty"`

	// AlertChannels设置告警通道
	// +optional
	AlertChannels []string `json:"alertChannels,omitempty"`

	// ErrorThreshold设置连续错误阈值
	// +optional
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	ErrorThreshold int `json:"errorThreshold,omitempty"`
}

// ExportConfig配置数据导出选项
type ExportConfig struct {
	// Format指定导出格式
	// +optional
	// +kubebuilder:validation:Enum=json;csv;prometheus
	Format string `json:"format,omitempty"`

	// Destination指定导出目标
	// +optional
	Destination string `json:"destination,omitempty"`

	// Schedule指定导出计划
	// +optional
	Schedule string `json:"schedule,omitempty"`
}

// Analytics 包含性能分析结果
type Analytics struct {
	// PerformanceTrend 包含性能趋势分析结果
	// +optional
	PerformanceTrend map[string]string `json:"performanceTrend,omitempty"`

	// ChangeRate 包含各指标的变化率
	// +optional
	ChangeRate map[string]float64 `json:"changeRate,omitempty"`

	// BaselineComparison 与基准比较的结果
	// +optional
	BaselineComparison map[string]float64 `json:"baselineComparison,omitempty"`
}

// Anomaly 表示检测到的异常
type Anomaly struct {
	// 异常指标的名称
	Metric string `json:"metric"`

	// 异常指标的值
	Value float64 `json:"value"`

	// 异常检测到的时间
	DetectionTime metav1.Time `json:"detectionTime"`

	// 异常执行的时间
	ExecutionTime metav1.Time `json:"executionTime"`

	// 异常的严重程度 (低、中、高)
	Severity string `json:"severity"`

	// 异常的描述
	Description string `json:"description"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.testType"
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".spec.target"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Total",type="integer",JSONPath=".status.summary.total"
// +kubebuilder:printcolumn:name="Succeeded",type="integer",JSONPath=".status.summary.succeeded"
// +kubebuilder:printcolumn:name="Failed",type="integer",JSONPath=".status.summary.failed"
// +kubebuilder:printcolumn:name="AvgResponse",type="number",JSONPath=".status.summary.averageResponseTime"
// +kubebuilder:printcolumn:name="MinResponse",type="number",JSONPath=".status.summary.minResponseTime",priority=1
// +kubebuilder:printcolumn:name="MaxResponse",type="number",JSONPath=".status.summary.maxResponseTime",priority=1
// +kubebuilder:printcolumn:name="Duration",type="integer",JSONPath=".spec.testDuration",priority=1
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// TestReport is the Schema for the testreports API
type TestReport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TestReportSpec   `json:"spec,omitempty"`
	Status TestReportStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// TestReportList contains a list of TestReport
type TestReportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TestReport `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TestReport{}, &TestReportList{})
}
