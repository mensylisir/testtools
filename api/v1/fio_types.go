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

// FioSpec defines the desired state of Fio
type FioSpec struct {
	// FilePath is the path where the test file will be created
	// +kubebuilder:validation:Required
	FilePath string `json:"filePath"`

	// Name is the name of the fio job
	// +optional
	// +kubebuilder:default="fio-test"
	JobName string `json:"jobName,omitempty"`

	// ReadWrite specifies the I/O pattern (e.g., read, write, randread, randwrite, readwrite, randrw)
	// +kubebuilder:validation:Enum=read;write;randread;randwrite;readwrite;randrw
	// +optional
	// +kubebuilder:default=randread
	ReadWrite string `json:"readWrite,omitempty"`

	// BlockSize specifies the block size in bytes (e.g., 4k, 1m)
	// +optional
	// +kubebuilder:default="4k"
	BlockSize string `json:"blockSize,omitempty"`

	// IODepth specifies the number of I/O units to keep in flight against the file
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1024
	IODepth int32 `json:"ioDepth,omitempty"`

	// Size specifies the total size of I/O (e.g., 1g, 100m)
	// +optional
	// +kubebuilder:default="100m"
	Size string `json:"size,omitempty"`

	// DirectIO enables O_DIRECT I/O mode (bypasses page cache)
	// +optional
	// +kubebuilder:default=true
	DirectIO bool `json:"directIO,omitempty"`

	// NumJobs specifies the number of clones of this job
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=64
	NumJobs int32 `json:"numJobs,omitempty"`

	// Runtime specifies the runtime in seconds
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3600
	Runtime int32 `json:"runtime,omitempty"`

	// IOEngine specifies the I/O engine to use
	// +kubebuilder:validation:Enum=libaio;io_uring;sync;psync;vsync;posixaio
	// +optional
	// +kubebuilder:default="libaio"
	IOEngine string `json:"ioEngine,omitempty"`

	// Group enables grouping of results across jobs
	// +optional
	Group bool `json:"group,omitempty"`

	// KernelFilesystemBufferCache determines if buffered I/O uses the kernel's buffer cache
	// +optional
	KernelFilesystemBufferCache bool `json:"kernelFilesystemBufferCache,omitempty"`

	// ExtraParams allows specifying additional fio parameters
	// +optional
	ExtraParams map[string]string `json:"extraParams,omitempty"`

	// Schedule is the interval in seconds for running the fio command
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// MaxRetries is the maximum number of retries for failed tests
	// +optional
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	MaxRetries int32 `json:"maxRetries,omitempty"`

	// Image specifies the container image used to run the FIO test
	// +optional
	// +kubebuilder:default="172.30.1.13:18093/testtools-fio:v1"
	Image string `json:"image,omitempty"`

	// RetainJobPods controls whether to retain the completed Job and Pods
	// +kubebuilder:default=false
	RetainJobPods bool `json:"retainJobPods,omitempty"`

	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// +optional
	// +mapType=atomic
	NodeSelector map[string]string `json:"nodeSelector,omitempty" protobuf:"bytes,7,rep,name=nodeSelector"`

	// NodeName indicates in which node this pod is scheduled.
	// +optional
	NodeName string `json:"nodeName,omitempty" protobuf:"bytes,10,opt,name=nodeName"`
}

// FioStats contains performance statistics for an FIO test
type FioStats struct {
	// ReadIOPS is the read I/O operations per second
	// +optional
	ReadIOPS string `json:"readIOPS,omitempty"`

	// WriteIOPS is the write I/O operations per second
	// +optional
	WriteIOPS string `json:"writeIOPS,omitempty"`

	// ReadBW is the read bandwidth in KiB/s
	// +optional
	ReadBW string `json:"readBW,omitempty"`

	// WriteBW is the write bandwidth in KiB/s
	// +optional
	WriteBW string `json:"writeBW,omitempty"`

	// ReadLatency is the average read latency in microseconds
	// +optional
	ReadLatency string `json:"readLatency,omitempty"`

	// WriteLatency is the average write latency in microseconds
	// +optional
	WriteLatency string `json:"writeLatency,omitempty"`

	// LatencyPercentiles contains percentile latency values
	// +optional
	LatencyPercentiles map[string]string `json:"latencyPercentiles,omitempty"`
}

// FioStatus defines the observed state of Fio
type FioStatus struct {
	// QueryCount is the number of fio tests executed
	// +optional
	QueryCount int64 `json:"queryCount,omitempty"`

	// LastExecutionTime is the time of the last execution
	// +optional
	LastExecutionTime *metav1.Time `json:"lastExecutionTime,omitempty"`

	// Status is the status of the last fio operation
	// +optional
	Status string `json:"status,omitempty"`

	// LastResult is the result of the last fio operation
	// +optional
	LastResult string `json:"lastResult,omitempty"`

	// ExecutedCommand is the command that was executed for debug purposes
	// +optional
	ExecutedCommand string `json:"executedCommand,omitempty"`

	// SuccessCount is the number of successful fio tests
	// +optional
	SuccessCount int64 `json:"successCount,omitempty"`

	// FailureCount is the number of failed fio tests
	// +optional
	FailureCount int64 `json:"failureCount,omitempty"`

	// Stats contains the performance statistics for the last test
	// +optional
	Stats FioStats `json:"stats,omitempty"`

	// TestReportName is the name of the associated TestReport resource
	// +optional
	TestReportName string `json:"testReportName,omitempty"`

	// Conditions represents the latest available observations of the fio's current state
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
// +kubebuilder:printcolumn:name="Succeeded",type="integer",JSONPath=".status.successCount"
// +kubebuilder:printcolumn:name="Failed",type="integer",JSONPath=".status.failureCount"
// +kubebuilder:printcolumn:name="ReadIOPS",type="string",JSONPath=".status.stats.readIOPS",priority=1
// +kubebuilder:printcolumn:name="WriteIOPS",type="string",JSONPath=".status.stats.writeIOPS",priority=1
// +kubebuilder:printcolumn:name="ReadBW",type="string",JSONPath=".status.stats.readBW",priority=1
// +kubebuilder:printcolumn:name="WriteBW",type="string",JSONPath=".status.stats.writeBW",priority=1
// +kubebuilder:printcolumn:name="ReadLat",type="string",JSONPath=".status.stats.readLatency",priority=1
// +kubebuilder:printcolumn:name="LastRun",type="date",JSONPath=".status.lastExecutionTime"
// +kubebuilder:printcolumn:name="TestReport",type="string",JSONPath=".status.testReportName"
// Fio is the Schema for the fios API
type Fio struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FioSpec   `json:"spec,omitempty"`
	Status FioStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// FioList contains a list of Fio
type FioList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Fio `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Fio{}, &FioList{})
}
