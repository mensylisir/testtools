package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SkoopSpec defines the desired state of a kubeskoop command
type SkoopSpec struct {
	// SourcePod is the pod where the probe starts
	// +optional
	SourcePod string `json:"sourcePod,omitempty"`

	// SourceNamespace of the pod (default is same as resource namespace)
	// +optional
	SourceNamespace string `json:"sourceNamespace,omitempty"`

	// Destination, can be pod name, IP, or service FQDN
	// +optional
	Destination string `json:"destination,omitempty"`

	// DestinationNamespace of the target pod/service
	// +optional
	DestinationNamespace string `json:"destinationNamespace,omitempty"`

	// Type of check: tcp, udp, icmp, http, etc.
	// +kubebuilder:validation:Enum=tcp;udp;icmp;http;all
	// +kubebuilder:default=all
	Type string `json:"type,omitempty"`

	// Duration of the test, in seconds
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=600
	Duration int32 `json:"duration,omitempty"`

	// Image used to run the probe
	// +kubebuilder:default="registry.cn-hangzhou.aliyuncs.com/kubeskoop/skoop:latest"
	Image string `json:"image,omitempty"`

	// Schedule is a cron expression for periodic run
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// Concurrency defines how many jobs to run in parallel
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	Concurrency int32 `json:"concurrency,omitempty"`
}

// SkoopStatus defines the observed state of Skoop
type SkoopStatus struct {
	// LastExecutionTime is the time when last test was executed
	// +optional
	LastExecutionTime *metav1.Time `json:"lastExecutionTime,omitempty"`

	// LastResult stores the output or summary of the test
	// +optional
	LastResult string `json:"lastResult,omitempty"`

	// ExecutedCommand shows the actual executed command
	// +optional
	ExecutedCommand string `json:"executedCommand,omitempty"`

	// TestReportName links to the report resource
	// +optional
	TestReportName string `json:"testReportName,omitempty"`

	// Conditions represent the current state of the Skoop test
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
// +kubebuilder:printcolumn:name="Source",type="string",JSONPath=".spec.sourcePod"
// +kubebuilder:printcolumn:name="Dest",type="string",JSONPath=".spec.destination"
// +kubebuilder:printcolumn:name="LastRun",type="date",JSONPath=".status.lastExecutionTime"
type Skoop struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SkoopSpec   `json:"spec,omitempty"`
	Status SkoopStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type SkoopList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Skoop `json:"items"`
}

//func init() {
//	SchemeBuilder.Register(&Skoop{}, &SkoopList{})
//}
