package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Iperf3Mode represents client or server mode
// +kubebuilder:validation:Enum=client;server
type Iperf3Mode string

const (
	Iperf3ClientMode Iperf3Mode = "client"
	Iperf3ServerMode Iperf3Mode = "server"
)

// Iperf3Spec defines the desired state of Iperf3
type Iperf3Spec struct {
	// Mode specifies whether this is an iperf3 client or server
	// +kubebuilder:default=client
	Mode Iperf3Mode `json:"mode"`

	// Server is the target host to connect to (client mode only)
	// +optional
	Server string `json:"server,omitempty"`

	// Port is the port iperf3 server listens on or client connects to
	// +kubebuilder:default=5201
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// Duration of the test in seconds
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3600
	Duration int32 `json:"duration,omitempty"`

	// Parallel streams to use (client mode only)
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=128
	Parallel int32 `json:"parallel,omitempty"`

	// Bandwidth limit (client mode only), e.g. "10M", "1G"
	// +optional
	Bandwidth string `json:"bandwidth,omitempty"`

	// Protocol defines the protocol to be used (TCP or UDP)
	// +kubebuilder:validation:Enum=TCP;UDP
	// +kubebuilder:default=TCP
	// +optional
	Protocol string `json:"protocol,omitempty"`

	// BufferSize
	// +kubebuilder:default=TCP
	// +optional
	BufferSize string `json:"BufferSize,omitempty"`

	// Reverse mode (client only): test upload instead of download
	// +optional
	// +kubebuilder:default=false
	Reverse bool `json:"reverse,omitempty"`

	// Verbose
	// +optional
	// +kubebuilder:default=false
	Verbose bool `json:"Verbose,omitempty"`

	// Image to use for running iperf3
	// +optional
	// +kubebuilder:default="172.30.1.13:18093/testtools-iperf3:v1"
	Image string `json:"image,omitempty"`

	// Schedule defines cron expression for periodic test
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// Concurrency defines how many client jobs to run in parallel
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	Concurrency int32 `json:"concurrency,omitempty"`
}

// Iperf3Status defines the observed state of Iperf3
type Iperf3Status struct {
	// LastExecutionTime indicates the last time the test was executed
	// +optional
	LastExecutionTime *metav1.Time `json:"lastExecutionTime,omitempty"`

	// LastResult contains the result summary of the last iperf3 run
	// +optional
	LastResult string `json:"lastResult,omitempty"`

	// ExecutedCommand is the actual command that was run
	// +optional
	ExecutedCommand string `json:"executedCommand,omitempty"`

	// TestReportName links to the associated TestReport
	// +optional
	TestReportName string `json:"testReportName,omitempty"`

	// Conditions reflect the state of the test resource
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Mode",type="string",JSONPath=".spec.mode"
// +kubebuilder:printcolumn:name="Server",type="string",JSONPath=".spec.server"
// +kubebuilder:printcolumn:name="Duration",type="integer",JSONPath=".spec.duration"
// +kubebuilder:printcolumn:name="LastRun",type="date",JSONPath=".status.lastExecutionTime"
// +kubebuilder:printcolumn:name="Report",type="string",JSONPath=".status.testReportName"
type Iperf3 struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   Iperf3Spec   `json:"spec,omitempty"`
	Status Iperf3Status `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type Iperf3List struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Iperf3 `json:"items"`
}

//func init() {
//	SchemeBuilder.Register(&Iperf3{}, &Iperf3List{})
//}
