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

// PingSpec defines the desired state of Ping
type PingSpec struct {
	// Host is the host to ping
	// +kubebuilder:validation:Required
	Host string `json:"host"`

	// Count is the number of ping requests to send
	// +optional
	// +kubebuilder:default=4
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	Count int32 `json:"count,omitempty"`

	// Interval is the interval between ping requests in seconds
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=60
	Interval int32 `json:"interval,omitempty"`

	// Timeout is the timeout for each ping request in seconds
	// +optional
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=60
	Timeout int32 `json:"timeout,omitempty"`

	// PacketSize is the size of the ICMP packet to send
	// +optional
	// +kubebuilder:default=56
	// +kubebuilder:validation:Minimum=8
	// +kubebuilder:validation:Maximum=65507
	PacketSize int32 `json:"packetSize,omitempty"`

	// TTL is the IP Time to Live
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=255
	TTL int32 `json:"ttl,omitempty"`

	// DoNotFragment sets the "don't fragment" bit in the IP header
	// +optional
	DoNotFragment bool `json:"doNotFragment,omitempty"`

	// UseIPv4Only forces IPv4 usage
	// +optional
	UseIPv4Only bool `json:"useIPv4Only,omitempty"`

	// UseIPv6Only forces IPv6 usage
	// +optional
	UseIPv6Only bool `json:"useIPv6Only,omitempty"`

	// Schedule is the cron schedule for running the ping command
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// MaxRetries is the maximum number of retries for failed pings
	// +optional
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	MaxRetries int32 `json:"maxRetries,omitempty"`

	// Concurrency is the number of concurrent pings to run
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	Concurrency int32 `json:"concurrency,omitempty"`
}

// PingStatus defines the observed state of Ping
type PingStatus struct {
	// QueryCount is the number of pings executed
	// +optional
	QueryCount int64 `json:"queryCount,omitempty"`

	// LastExecutionTime is the time of the last execution
	// +optional
	LastExecutionTime *metav1.Time `json:"lastExecutionTime,omitempty"`

	// Status is the status of the last ping operation
	// +optional
	Status string `json:"status,omitempty"`

	// LastResult is the result of the last ping operation
	// +optional
	LastResult string `json:"lastResult,omitempty"`

	// SuccessCount is the number of successful pings
	// +optional
	SuccessCount int64 `json:"successCount,omitempty"`

	// FailureCount is the number of failed pings
	// +optional
	FailureCount int64 `json:"failureCount,omitempty"`

	// PacketLoss is the percentage of packet loss
	// +optional
	PacketLoss float64 `json:"packetLoss,omitempty"`

	// MinRtt is the minimum round trip time in milliseconds
	// +optional
	MinRtt float64 `json:"minRtt,omitempty"`

	// AvgRtt is the average round trip time in milliseconds
	// +optional
	AvgRtt float64 `json:"avgRtt,omitempty"`

	// MaxRtt is the maximum round trip time in milliseconds
	// +optional
	MaxRtt float64 `json:"maxRtt,omitempty"`

	// TestReportName is the name of the associated TestReport resource
	// +optional
	TestReportName string `json:"testReportName,omitempty"`

	// Conditions represents the latest available observations of the ping's current state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="QueryCount",type="integer",JSONPath=".status.queryCount"
// +kubebuilder:printcolumn:name="PacketLoss",type="number",JSONPath=".status.packetLoss"
// +kubebuilder:printcolumn:name="MinRtt",type="number",JSONPath=".status.minRtt"
// +kubebuilder:printcolumn:name="AvgRtt",type="number",JSONPath=".status.avgRtt"
// +kubebuilder:printcolumn:name="MaxRtt",type="number",JSONPath=".status.maxRtt"
// +kubebuilder:printcolumn:name="LastRun",type="date",JSONPath=".status.lastExecutionTime"
// +kubebuilder:printcolumn:name="TestReport",type="string",JSONPath=".status.testReportName",priority=1
// Ping is the Schema for the pings API
type Ping struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PingSpec   `json:"spec,omitempty"`
	Status PingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// PingList contains a list of Ping
type PingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Ping `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Ping{}, &PingList{})
}
