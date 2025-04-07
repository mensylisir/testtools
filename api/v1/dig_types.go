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

// DigSpec defines the desired state of Dig
type DigSpec struct {
	// Domain is the domain name to query
	// +kubebuilder:validation:Required
	Domain string `json:"domain"`

	// Server is the name server to query (@server parameter)
	// +optional
	Server string `json:"server,omitempty"`

	// SourceIP sets the source IP address for the query (-b parameter)
	// +optional
	SourceIP string `json:"sourceIP,omitempty"`

	// SourcePort sets the source port for the query (with -b parameter)
	// +optional
	SourcePort int32 `json:"sourcePort,omitempty"`

	// QueryClass overrides the default query class (-c parameter)
	// +kubebuilder:validation:Enum=IN;HS;CH
	// +optional
	QueryClass string `json:"queryClass,omitempty"`

	// QueryFile specifies a file with query requests (-f parameter)
	// +optional
	QueryFile string `json:"queryFile,omitempty"`

	// KeyFile specifies the TSIG key file (-k parameter)
	// +optional
	KeyFile string `json:"keyFile,omitempty"`

	// Port specifies a non-standard port number (-p parameter)
	// +optional
	Port int32 `json:"port,omitempty"`

	// QueryName sets the query name (-q parameter)
	// +optional
	QueryName string `json:"queryName,omitempty"`

	// QueryType sets the query type (-t parameter)
	// +kubebuilder:validation:Enum=A;AAAA;NS;MX;TXT;CNAME;SOA;PTR;SRV;CAA
	// +optional
	// +kubebuilder:default=A
	QueryType string `json:"queryType,omitempty"`

	// UseMicroseconds indicates that query time should be in microseconds (-u parameter)
	// +optional
	UseMicroseconds bool `json:"useMicroseconds,omitempty"`

	// ReverseQuery simplifies reverse lookup (-x parameter)
	// +optional
	ReverseQuery string `json:"reverseQuery,omitempty"`

	// TSIGKey specifies TSIG key on command line (-y parameter)
	// +optional
	TSIGKey string `json:"tsigKey,omitempty"`

	// UseIPv4Only forces IPv4 query transport (-4 parameter)
	// +optional
	UseIPv4Only bool `json:"useIPv4Only,omitempty"`

	// UseIPv6Only forces IPv6 query transport (-6 parameter)
	// +optional
	UseIPv6Only bool `json:"useIPv6Only,omitempty"`

	// UseTCP specifies whether to use TCP for queries (+tcp option)
	// +optional
	UseTCP bool `json:"useTCP,omitempty"`

	// Timeout sets the query timeout in seconds (+time option)
	// +optional
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=60
	Timeout int32 `json:"timeout,omitempty"`

	// Schedule is the cron schedule for running the dig command
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// MaxRetries is the maximum number of retries for failed queries
	// +optional
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	MaxRetries int32 `json:"maxRetries,omitempty"`

	// Concurrency is the number of concurrent queries to run
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	Concurrency int32 `json:"concurrency,omitempty"`
}

// DigStatus defines the observed state of Dig
type DigStatus struct {
	// QueryCount is the number of queries executed
	// +optional
	QueryCount int64 `json:"queryCount,omitempty"`

	// LastExecutionTime is the time of the last execution
	// +optional
	LastExecutionTime *metav1.Time `json:"lastExecutionTime,omitempty"`

	// Status is the status of the last dig operation
	// +optional
	Status string `json:"status,omitempty"`

	// LastResult is the result of the last dig operation
	// +optional
	LastResult string `json:"lastResult,omitempty"`

	// SuccessCount is the number of successful queries
	// +optional
	SuccessCount int64 `json:"successCount,omitempty"`

	// FailureCount is the number of failed queries
	// +optional
	FailureCount int64 `json:"failureCount,omitempty"`

	// AverageResponseTime is the average response time in milliseconds
	// +optional
	AverageResponseTime float64 `json:"averageResponseTime,omitempty"`

	// TestReportName is the name of the associated TestReport resource
	// +optional
	TestReportName string `json:"testReportName,omitempty"`

	// Conditions represents the latest available observations of the dig's current state
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
// +kubebuilder:printcolumn:name="Success",type="integer",JSONPath=".status.successCount"
// +kubebuilder:printcolumn:name="Failed",type="integer",JSONPath=".status.failureCount"
// +kubebuilder:printcolumn:name="AvgResponse",type="number",JSONPath=".status.averageResponseTime",priority=1
// +kubebuilder:printcolumn:name="LastRun",type="date",JSONPath=".status.lastExecutionTime"
// +kubebuilder:printcolumn:name="TestReport",type="string",JSONPath=".status.testReportName"
// Dig is the Schema for the digs API
type Dig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DigSpec   `json:"spec,omitempty"`
	Status DigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// DigList contains a list of Dig
type DigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Dig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Dig{}, &DigList{})
}
