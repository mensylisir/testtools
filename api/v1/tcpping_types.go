package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TcppingSpec defines the desired state of Tcpping
type TcppingSpec struct {
	// Host is the target host to ping via TCP
	// +kubebuilder:validation:Required
	Host string `json:"host"`

	// Port is the target port to connect to
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:validation:Required
	Port int32 `json:"port"`

	// Count is the number of ping attempts
	// +optional
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	Count int32 `json:"count,omitempty"`

	// Timeout in seconds for each ping attempt
	// +optional
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=60
	Timeout int32 `json:"timeout,omitempty"`

	// Interval in seconds between ping attempts
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=60
	Interval int32 `json:"interval,omitempty"`

	// Concurrency defines how many ping probes run in parallel
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	Concurrency int32 `json:"concurrency,omitempty"`

	// Schedule defines cron expression for periodic test
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// Image to use for running tcpping
	// +optional
	// +kubebuilder:default="172.30.1.13:18093/testtools-tcpping:v1"
	Image string `json:"image,omitempty"`
}

// TcppingStatus defines the observed state of Tcpping
type TcppingStatus struct {
	// SuccessCount indicates how many pings succeeded
	// +optional
	SuccessCount int64 `json:"successCount,omitempty"`

	// FailureCount indicates how many pings failed
	// +optional
	FailureCount int64 `json:"failureCount,omitempty"`

	// LastExecutionTime indicates the last time the test was executed
	// +optional
	LastExecutionTime *metav1.Time `json:"lastExecutionTime,omitempty"`

	// LastResult contains the result of the last tcpping
	// +optional
	LastResult string `json:"lastResult,omitempty"`

	// ExecutedCommand shows the actual command executed (debug)
	// +optional
	ExecutedCommand string `json:"executedCommand,omitempty"`

	// AverageResponseTime in milliseconds
	// +optional
	AverageResponseTime float64 `json:"averageResponseTime,omitempty"`

	// TestReportName links to the associated TestReport
	// +optional
	TestReportName string `json:"testReportName,omitempty"`

	// Conditions indicates current state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Success",type="integer",JSONPath=".status.successCount"
// +kubebuilder:printcolumn:name="Failed",type="integer",JSONPath=".status.failureCount"
// +kubebuilder:printcolumn:name="LastRun",type="date",JSONPath=".status.lastExecutionTime"
// +kubebuilder:printcolumn:name="TestReport",type="string",JSONPath=".status.testReportName"
type Tcpping struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TcppingSpec   `json:"spec,omitempty"`
	Status TcppingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type TcppingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tcpping `json:"items"`
}

//func init() {
//	SchemeBuilder.Register(&Tcpping{}, &TcppingList{})
//}
