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
