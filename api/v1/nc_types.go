package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NcSpec defines the desired state of Nc
type NcSpec struct {
	// Host is the target hostname or IP address
	// +kubebuilder:validation:Required
	Host string `json:"host"`

	// Port is the target port to connect to
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:validation:Required
	Port int32 `json:"port"`

	// UDP sets UDP mode (-u)
	// +optional
	UDP bool `json:"udp,omitempty"`

	// Verbose enables verbose output (-v)
	// +optional
	Verbose bool `json:"verbose,omitempty"`

	// ZeroIO mode just checks connectivity without sending data (-z)
	// +optional
	ZeroIO bool `json:"zeroIO,omitempty"`

	// NoDNS disables DNS resolution (-n)
	// +optional
	NoDNS bool `json:"noDNS,omitempty"`

	// Timeout in seconds (-w)
	// +optional
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=60
	Timeout int32 `json:"timeout,omitempty"`

	// SourceIP specifies the source IP address (-s)
	// +optional
	SourceIP string `json:"sourceIP,omitempty"`

	// SourcePort sets the source port number (-p)
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	SourcePort int32 `json:"sourcePort,omitempty"`

	// RepeatCount specifies how many times to run the test
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	RepeatCount int32 `json:"repeatCount,omitempty"`

	// Schedule is the cron schedule to run this test
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// Concurrency is how many parallel nc operations to run
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	Concurrency int32 `json:"concurrency,omitempty"`

	// Image specifies the container image used to run nc
	// +optional
	// +kubebuilder:default="172.30.1.13:18093/testtools-nc:v1"
	Image string `json:"image,omitempty"`
}

// NcStatus defines the observed state of Nc
type NcStatus struct {
	// SuccessCount is the number of successful tests
	// +optional
	SuccessCount int64 `json:"successCount,omitempty"`

	// FailureCount is the number of failed tests
	// +optional
	FailureCount int64 `json:"failureCount,omitempty"`

	// LastExecutionTime is the time the test was last run
	// +optional
	LastExecutionTime *metav1.Time `json:"lastExecutionTime,omitempty"`

	// LastResult is the result of the last nc command
	// +optional
	LastResult string `json:"lastResult,omitempty"`

	// ExecutedCommand is the actual command run (for debug)
	// +optional
	ExecutedCommand string `json:"executedCommand,omitempty"`

	// AverageResponseTime in milliseconds
	// +optional
	AverageResponseTime float64 `json:"averageResponseTime,omitempty"`

	// TestReportName links to the related TestReport resource
	// +optional
	TestReportName string `json:"testReportName,omitempty"`

	// Conditions reflects the current state of the resource
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
type Nc struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NcSpec   `json:"spec,omitempty"`
	Status NcStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type NcList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Nc `json:"items"`
}

//func init() {
//	SchemeBuilder.Register(&Nc{}, &NcList{})
//}
