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

// TcpPingSpec 定义了TcpPing资源的期望状态
// +kubebuilder:validation:Required
type TcpPingSpec struct {
	// 目标主机，必填
	// +kubebuilder:validation:Required
	Host string `json:"host"`

	// 目标端口，必填
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// 发送的包数，默认为5
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1000
	// +kubebuilder:default=5
	Count int32 `json:"count,omitempty"`

	// 等待超时时间（秒），默认为2秒
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=60
	// +kubebuilder:default=2
	Timeout int32 `json:"timeout,omitempty"`

	// 包间隔时间（秒），默认为1秒
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=60
	// +kubebuilder:default=1
	Interval int64 `json:"interval,omitempty"`

	// 源端口，可选
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=65535
	SourcePort int32 `json:"sourcePort,omitempty"`

	// 源地址，可选
	SourceAddress string `json:"sourceAddress,omitempty"`

	// 测试间隔（秒），若设置则定时执行
	// +kubebuilder:validation:Pattern=^[0-9]+$
	Schedule string `json:"schedule,omitempty"`

	// 是否开启详细日志输出
	// +kubebuilder:default=false
	Verbose bool `json:"verbose,omitempty"`

	// 是否使用TCP SYN包而不是完整连接
	// +kubebuilder:default=true
	UseSynOnly bool `json:"useSynOnly,omitempty"`

	// 是否开启IPv4模式
	// +kubebuilder:default=false
	UseIPv4Only bool `json:"useIPv4Only,omitempty"`

	// 是否开启IPv6模式
	// +kubebuilder:default=false
	UseIPv6Only bool `json:"useIPv6Only,omitempty"`

	// Image specifies the container image used to run the Dig test
	// +optional
	// +kubebuilder:default="172.30.1.13:18093/testtools-tcpping:v1"
	Image string `json:"image,omitempty"`

	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// +optional
	// +mapType=atomic
	NodeSelector map[string]string `json:"nodeSelector,omitempty" protobuf:"bytes,7,rep,name=nodeSelector"`

	// NodeName indicates in which node this pod is scheduled.
	// +optional
	NodeName string `json:"nodeName,omitempty" protobuf:"bytes,10,opt,name=nodeName"`
}

// TcpPingStats 记录TCP连接测试的统计信息
type TcpPingStats struct {
	// 发送的包数
	Transmitted int32 `json:"transmitted,omitempty"`

	// 接收的包数
	Received int32 `json:"received,omitempty"`

	// 丢包率
	PacketLoss string `json:"packetLoss,omitempty"`

	// 最小延迟（毫秒）
	MinLatency string `json:"minLatency,omitempty"`

	// 平均延迟（毫秒）
	AvgLatency string `json:"avgLatency,omitempty"`

	// 最大延迟（毫秒）
	MaxLatency string `json:"maxLatency,omitempty"`

	// 延迟标准差（毫秒）
	StdDevLatency string `json:"stdDevLatency,omitempty"`
}

// TcpPingStatus 定义了TcpPing资源的观测状态
type TcpPingStatus struct {
	// 测试状态：Running, Succeeded, Failed
	Status string `json:"status,omitempty"`

	// 最后执行时间
	LastExecutionTime *metav1.Time `json:"lastExecutionTime,omitempty"`

	// 执行的命令
	ExecutedCommand string `json:"executedCommand,omitempty"`

	// TCP Ping统计信息
	Stats TcpPingStats `json:"stats,omitempty"`

	// 最后测试结果原始输出
	LastResult string `json:"lastResult,omitempty"`

	// 查询次数
	QueryCount int32 `json:"queryCount,omitempty"`

	// 成功次数
	SuccessCount int32 `json:"successCount,omitempty"`

	// 失败次数
	FailureCount int32 `json:"failureCount,omitempty"`

	// 关联的测试报告名称
	TestReportName string `json:"testReportName,omitempty"`

	// 测试的Job名称
	JobName string `json:"jobName,omitempty"`

	// 状态条件
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status",description="测试状态"
//+kubebuilder:printcolumn:name="Host",type="string",JSONPath=".spec.host",description="目标主机"
//+kubebuilder:printcolumn:name="Port",type="integer",JSONPath=".spec.port",description="目标端口"
//+kubebuilder:printcolumn:name="Transmitted",type="integer",JSONPath=".status.stats.transmitted",description="发送包数"
//+kubebuilder:printcolumn:name="Received",type="integer",JSONPath=".status.stats.received",description="接收包数"
//+kubebuilder:printcolumn:name="Loss",type="string",JSONPath=".status.stats.packetLoss",description="丢包率(%)"
//+kubebuilder:printcolumn:name="AvgLatency",type="string",JSONPath=".status.stats.avgLatency",description="平均延迟(ms)"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// TcpPing 是TCP端口连通性和延迟测试资源的定义
type TcpPing struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TcpPingSpec   `json:"spec,omitempty"`
	Status TcpPingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TcpPingList 包含TcpPing资源的列表
type TcpPingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TcpPing `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TcpPing{}, &TcpPingList{})
}
