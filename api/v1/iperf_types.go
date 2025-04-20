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

// IperfSpec 定义了Iperf资源的期望状态
// +kubebuilder:validation:Required
type IperfSpec struct {
	// 目标主机，必填
	// +optional
	Host string `json:"host"`

	// 目标端口，默认为5201
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default=5201
	Port int32 `json:"port,omitempty"`

	// 测试模式，client或server，默认为client
	// +kubebuilder:validation:Enum=client;server
	// +optional
	Mode string `json:"mode,omitempty"`

	// 测试时长（秒），默认为10秒
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3600
	// +kubebuilder:default=10
	Duration int32 `json:"duration,omitempty"`

	// 测试协议，tcp或udp，默认为tcp
	// +kubebuilder:validation:Enum=tcp;udp
	// +kubebuilder:default=tcp
	Protocol string `json:"protocol,omitempty"`

	// 并发流数量，默认为1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=1
	Parallel int32 `json:"parallel,omitempty"`

	// 发送缓冲区大小（字节）
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=104857600
	SendBuffer int32 `json:"sendBuffer,omitempty"`

	// 接收缓冲区大小（字节）
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=104857600
	ReceiveBuffer int32 `json:"receiveBuffer,omitempty"`

	// 测试带宽限制（bits/sec）
	// +kubebuilder:validation:Minimum=1
	Bandwidth int64 `json:"bandwidth,omitempty"`

	// 是否启用反向模式测试（服务器向客户端传输）
	// +kubebuilder:default=false
	Reverse bool `json:"reverse,omitempty"`

	// 是否启用双向模式测试（同时进行发送和接收）
	// +kubebuilder:default=false
	Bidirectional bool `json:"bidirectional,omitempty"`

	// 报告间隔时间（秒）
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=60
	// +kubebuilder:default=1
	ReportInterval int64 `json:"reportInterval,omitempty"`

	// 测试间隔（秒），若设置则定时执行
	// +kubebuilder:validation:Pattern=^[0-9]+$
	Schedule string `json:"schedule,omitempty"`

	// 是否开启详细日志输出
	// +kubebuilder:default=false
	Verbose bool `json:"verbose,omitempty"`

	// 是否开启JSON输出
	// +kubebuilder:default=true
	JsonOutput bool `json:"jsonOutput,omitempty"`

	// 是否开启IPv4模式
	// +kubebuilder:default=false
	UseIPv4Only bool `json:"useIPv4Only,omitempty"`

	// 是否开启IPv6模式
	// +kubebuilder:default=false
	UseIPv6Only bool `json:"useIPv6Only,omitempty"`

	// Image specifies the container image used to run the Dig test
	// +optional
	// +kubebuilder:default="172.30.1.13:18093/testtools-iperf:v1"
	Image string `json:"image,omitempty"`

	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// +optional
	// +mapType=atomic
	NodeSelector map[string]string `json:"nodeSelector,omitempty" protobuf:"bytes,7,rep,name=nodeSelector"`

	// NodeName indicates in which node this pod is scheduled.
	// +optional
	NodeName string `json:"nodeName,omitempty" protobuf:"bytes,10,opt,name=nodeName"`
}

// IperfStats 定义了Iperf测试的性能统计信息
type IperfStats struct {
	// 测试开始时间
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// 测试结束时间
	EndTime *metav1.Time `json:"endTime,omitempty"`

	// 总发送数据量（字节）
	SentBytes int64 `json:"sentBytes,omitempty"`

	// 总接收数据量（字节）
	ReceivedBytes int64 `json:"receivedBytes,omitempty"`

	// 发送平均带宽（bits/sec）
	SendBandwidth string `json:"sendBandwidth,omitempty"`

	// 接收平均带宽（bits/sec）
	ReceiveBandwidth string `json:"receiveBandwidth,omitempty"`

	// 往返延迟（毫秒）
	RttMs string `json:"rttMs,omitempty"`

	// 重传计数
	Retransmits int32 `json:"retransmits,omitempty"`

	// 平均抖动（毫秒）
	Jitter string `json:"jitter,omitempty"`

	// 丢包数
	LostPackets int32 `json:"lostPackets,omitempty"`

	// 丢包率（百分比）
	LostPercent string `json:"lostPercent,omitempty"`

	// CPU利用率（百分比）
	CpuUtilization string `json:"cpuUtilization,omitempty"`
}

// IperfStatus 定义了Iperf资源的观测状态
type IperfStatus struct {
	// 测试状态：Running, Succeeded, Failed
	Status string `json:"status,omitempty"`

	// 最后执行时间
	LastExecutionTime *metav1.Time `json:"lastExecutionTime,omitempty"`

	// 执行的命令
	ExecutedCommand string `json:"executedCommand,omitempty"`

	// 最后测试结果
	LastResult string `json:"lastResult,omitempty"`

	// 性能统计信息
	Stats IperfStats `json:"stats,omitempty"`

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
//+kubebuilder:printcolumn:name="Protocol",type="string",JSONPath=".spec.protocol",description="协议类型"
//+kubebuilder:printcolumn:name="SendBW(Mbps)",type="string",JSONPath=".status.stats.sendBandwidth",description="发送带宽(Mbps)"
//+kubebuilder:printcolumn:name="RecvBW(Mbps)",type="string",JSONPath=".status.stats.receiveBandwidth",description="接收带宽(Mbps)"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Iperf 是网络性能测试资源的定义
type Iperf struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IperfSpec   `json:"spec,omitempty"`
	Status IperfStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IperfList 包含Iperf资源的列表
type IperfList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Iperf `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Iperf{}, &IperfList{})
}
