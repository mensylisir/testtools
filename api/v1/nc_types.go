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

// NcSpec 定义了Nc资源的期望状态
// +kubebuilder:validation:Required
type NcSpec struct {
	// 目标主机，必填
	// +kubebuilder:validation:Required
	Host string `json:"host"`

	// 目标端口，必填
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// 协议类型，支持tcp和udp，默认为tcp
	// +kubebuilder:validation:Enum=tcp;udp
	// +kubebuilder:default=tcp
	Protocol string `json:"protocol,omitempty"`

	// 超时时间（秒），默认为5秒
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=300
	// +kubebuilder:default=5
	Timeout int32 `json:"timeout,omitempty"`

	// 是否开启详细日志输出
	// +kubebuilder:default=true
	Verbose bool `json:"verbose,omitempty"`

	// 测试间隔（秒），若设置则定时执行
	// +kubebuilder:validation:Pattern=^[0-9]+$
	Schedule string `json:"schedule,omitempty"`

	// 是否等待连接关闭
	// +kubebuilder:default=false
	Wait bool `json:"wait,omitempty"`

	// 是否只进行连接测试不发送数据
	// +kubebuilder:default=false
	ZeroInput bool `json:"zeroInput,omitempty"`

	// 是否开启IPv4模式
	// +kubebuilder:default=false
	UseIPv4Only bool `json:"useIPv4Only,omitempty"`

	// 是否开启IPv6模式
	// +kubebuilder:default=false
	UseIPv6Only bool `json:"useIPv6Only,omitempty"`

	// Image specifies the container image used to run the Dig test
	// +optional
	// +kubebuilder:default="registry.dev.rdev.tech:18093/testtools-nc:v3"
	Image string `json:"image,omitempty"`

	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// +optional
	// +mapType=atomic
	NodeSelector map[string]string `json:"nodeSelector,omitempty" protobuf:"bytes,7,rep,name=nodeSelector"`

	// NodeName indicates in which node this pod is scheduled.
	// +optional
	NodeName string `json:"nodeName,omitempty" protobuf:"bytes,10,opt,name=nodeName"`
}

// NcStatus 定义了Nc资源的观测状态
type NcStatus struct {
	// 测试状态：Running, Succeeded, Failed
	Status string `json:"status,omitempty"`

	// 最后执行时间
	LastExecutionTime *metav1.Time `json:"lastExecutionTime,omitempty"`

	// 执行的命令
	ExecutedCommand string `json:"executedCommand,omitempty"`

	// 最后测试结果
	LastResult string `json:"lastResult,omitempty"`

	// 连接延迟（毫秒）
	ConnectionLatency string `json:"connectionLatency,omitempty"`

	// 连接是否成功
	ConnectionSuccess bool `json:"connectionSuccess,omitempty"`

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
//+kubebuilder:printcolumn:name="ConnectionSuccess",type="boolean",JSONPath=".status.connectionSuccess",description="连接是否成功"
//+kubebuilder:printcolumn:name="Latency",type="string",JSONPath=".status.connectionLatency",description="连接延迟(ms)"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Nc 是网络连接测试资源的定义
type Nc struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NcSpec   `json:"spec,omitempty"`
	Status NcStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NcList 包含Nc资源的列表
type NcList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Nc `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Nc{}, &NcList{})
}
