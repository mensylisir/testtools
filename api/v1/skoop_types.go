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

// SkoopSpec 定义了Skoop资源的期望状态
// +kubebuilder:validation:Required
type SkoopSpec struct {
	// 诊断任务配置
	Task SkoopTaskSpec `json:"task"`

	// 集群配置
	// +optional
	Cluster *SkoopClusterSpec `json:"cluster,omitempty"`

	// UI配置
	// +optional
	UI *SkoopUISpec `json:"ui,omitempty"`

	// 插件配置
	// +optional
	Plugins *SkoopPluginsSpec `json:"plugins,omitempty"`

	// 采集器配置
	// +optional
	Collector *SkoopCollectorSpec `json:"collector,omitempty"`

	// 日志配置
	// +optional
	Logging *SkoopLoggingSpec `json:"logging,omitempty"`

	// 是否定时执行诊断
	// +kubebuilder:validation:Pattern=^[0-9]+$
	Schedule string `json:"schedule,omitempty"`

	// 超时时间（秒）
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3600
	// +kubebuilder:default=300
	Timeout int32 `json:"timeout,omitempty"`

	// 采集器使用的镜像
	// +kubebuilder:default="kubeskoop/kubeskoop:v0.1.0"
	Image string `json:"image,omitempty"`

	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// +optional
	// +mapType=atomic
	NodeSelector map[string]string `json:"nodeSelector,omitempty" protobuf:"bytes,7,rep,name=nodeSelector"`

	// NodeName indicates in which node this pod is scheduled.
	// +optional
	NodeName string `json:"nodeName,omitempty" protobuf:"bytes,10,opt,name=nodeName"`
}

// SkoopTaskSpec 定义诊断任务相关配置
type SkoopTaskSpec struct {
	// 源地址
	// +optional
	SourceAddress string `json:"sourceAddress,omitempty"`

	// 目标地址，必填
	// +kubebuilder:validation:Required
	DestinationAddress string `json:"destinationAddress"`

	// 目标端口，必填
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	DestinationPort int32 `json:"destinationPort"`

	// 协议类型，默认为tcp
	// +kubebuilder:validation:Enum=tcp;udp;icmp
	// +kubebuilder:default=tcp
	Protocol string `json:"protocol,omitempty"`
}

// SkoopClusterSpec 定义集群相关配置
type SkoopClusterSpec struct {
	// 云厂商，默认为generic
	// +kubebuilder:default=generic
	CloudProvider string `json:"cloudProvider,omitempty"`

	// 集群Pod CIDR，如不设置将尝试自动检测
	ClusterCIDR string `json:"clusterCIDR,omitempty"`

	// 网络插件，如不设置将尝试自动检测
	NetworkPlugin string `json:"networkPlugin,omitempty"`

	// 代理模式，如不设置将尝试自动检测
	ProxyMode string `json:"proxyMode,omitempty"`

	// 阿里云配置
	// +optional
	Aliyun *AliyunProviderSpec `json:"aliyun,omitempty"`
}

// AliyunProviderSpec 定义阿里云配置
type AliyunProviderSpec struct {
	// 阿里云访问密钥ID
	AccessKeyID string `json:"accessKeyID,omitempty"`

	// 阿里云访问密钥Secret
	AccessKeySecret string `json:"accessKeySecret,omitempty"`

	// 阿里云安全Token（可选）
	SecurityToken string `json:"securityToken,omitempty"`
}

// SkoopUISpec 定义UI相关配置
type SkoopUISpec struct {
	// 输出格式，支持d2/svg/json
	// +kubebuilder:validation:Enum=d2;svg;json
	Format string `json:"format,omitempty"`

	// 是否启用HTTP服务器展示诊断结果
	// +kubebuilder:default=false
	EnableHTTP bool `json:"enableHTTP,omitempty"`

	// HTTP服务器监听地址
	// +kubebuilder:default="127.0.0.1:8080"
	HTTPAddress string `json:"httpAddress,omitempty"`

	// 输出文件名，默认为工作目录下的output.d2/svg/json
	OutputFile string `json:"outputFile,omitempty"`
}

// SkoopPluginsSpec 定义插件相关配置
type SkoopPluginsSpec struct {
	// Calico插件配置
	// +optional
	Calico *CalicoPluginSpec `json:"calico,omitempty"`

	// Flannel插件配置
	// +optional
	Flannel *FlannelPluginSpec `json:"flannel,omitempty"`
}

// CalicoPluginSpec 定义Calico插件配置
type CalicoPluginSpec struct {
	// 主机网络接口
	HostInterface string `json:"hostInterface,omitempty"`

	// 主机MTU
	// +kubebuilder:default=1500
	HostMTU int32 `json:"hostMTU,omitempty"`

	// Pod MTU (BGP模式)
	// +kubebuilder:default=1500
	PodMTU int32 `json:"podMTU,omitempty"`

	// Pod MTU (IPIP模式)
	// +kubebuilder:default=1480
	IPIPPodMTU int32 `json:"ipipPodMTU,omitempty"`
}

// FlannelPluginSpec 定义Flannel插件配置
type FlannelPluginSpec struct {
	// 后端类型，支持host-gw,vxlan,alloc，不设置则自动检测
	BackendType string `json:"backendType,omitempty"`

	// 桥接名称
	// +kubebuilder:default=cni0
	Bridge string `json:"bridge,omitempty"`

	// 主机网络接口
	HostInterface string `json:"hostInterface,omitempty"`

	// 是否进行IP伪装
	// +kubebuilder:default=true
	IPMasq bool `json:"ipMasq,omitempty"`

	// Pod MTU，不设置则根据CNI模式自动检测
	PodMTU int32 `json:"podMTU,omitempty"`
}

// SkoopCollectorSpec 定义采集器配置
type SkoopCollectorSpec struct {
	// CRI API端点地址
	CRIAddress string `json:"criAddress,omitempty"`

	// 采集器使用的镜像
	// +kubebuilder:default="kubeskoop/kubeskoop:v0.1.0"
	Image string `json:"image,omitempty"`

	// 采集器所在命名空间
	// +kubebuilder:default=skoop
	Namespace string `json:"namespace,omitempty"`

	// 采集器Pod运行检查间隔
	// +kubebuilder:default="2s"
	PodWaitInterval string `json:"podWaitInterval,omitempty"`

	// 采集器Pod运行检查超时
	// +kubebuilder:default="2m"
	PodWaitTimeout string `json:"podWaitTimeout,omitempty"`

	// 诊断完成后是否保留采集器Pod
	// +kubebuilder:default=false
	PreserveCollectorPod bool `json:"preserveCollectorPod,omitempty"`
}

// SkoopLoggingSpec 定义日志配置
type SkoopLoggingSpec struct {
	// 日志级别
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=9
	// +kubebuilder:default=2
	Verbosity int32 `json:"verbosity,omitempty"`

	// 是否输出到标准错误
	// +kubebuilder:default=true
	LogToStderr bool `json:"logToStderr,omitempty"`

	// 日志目录
	LogDir string `json:"logDir,omitempty"`

	// 日志文件
	LogFile string `json:"logFile,omitempty"`

	// 日志文件最大大小（MB）
	// +kubebuilder:default=1800
	LogFileMaxSize int32 `json:"logFileMaxSize,omitempty"`
}

// SkoopStatus 定义了Skoop资源的观测状态
type SkoopStatus struct {
	// 诊断状态：Running, Succeeded, Failed
	Status string `json:"status,omitempty"`

	// 最后执行时间
	LastExecutionTime *metav1.Time `json:"lastExecutionTime,omitempty"`

	// 执行的命令
	ExecutedCommand string `json:"executedCommand,omitempty"`

	// 诊断结果概要
	Summary string `json:"summary,omitempty"`

	// 诊断路径
	Path []SkoopPathNode `json:"path,omitempty"`

	// 诊断问题
	Issues []SkoopIssue `json:"issues,omitempty"`

	// 结果文件位置
	ResultFileURL string `json:"resultFileURL,omitempty"`

	// 查询次数
	QueryCount int32 `json:"queryCount,omitempty"`

	// 成功次数
	SuccessCount int32 `json:"successCount,omitempty"`

	// 失败次数
	FailureCount int32 `json:"failureCount,omitempty"`

	// 关联的测试报告名称
	TestReportName string `json:"testReportName,omitempty"`

	// 诊断的Pod名称
	DiagnosePodName string `json:"diagnosePodName,omitempty"`

	// LastResult is the result of the last ping operation
	// +optional
	LastResult string `json:"lastResult,omitempty"`

	// 状态条件
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// SkoopPathNode 定义网络路径上的节点
type SkoopPathNode struct {
	// 节点类型：Pod, Node, Service, etc.
	Type string `json:"type,omitempty"`

	// 节点名称
	Name string `json:"name,omitempty"`

	// 节点IP
	IP string `json:"ip,omitempty"`

	// 节点命名空间
	Namespace string `json:"namespace,omitempty"`

	// 所属节点
	NodeName string `json:"nodeName,omitempty"`

	// 通过网络接口
	Interface string `json:"interface,omitempty"`

	// 应用协议层信息
	Protocol string `json:"protocol,omitempty"`

	// 延迟（毫秒）
	LatencyMs string `json:"latencyMs,omitempty"`
}

// SkoopIssue 定义诊断发现的问题
type SkoopIssue struct {
	// 问题类型
	Type string `json:"type,omitempty"`

	// 问题级别：Warning, Error, Info
	Level string `json:"level,omitempty"`

	// 问题描述
	Message string `json:"message,omitempty"`

	// 问题位置
	Location string `json:"location,omitempty"`

	// 建议解决方案
	Solution string `json:"solution,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status",description="诊断状态"
//+kubebuilder:printcolumn:name="Source",type="string",JSONPath=".spec.task.sourceAddress",description="源地址"
//+kubebuilder:printcolumn:name="Destination",type="string",JSONPath=".spec.task.destinationAddress",description="目标地址"
//+kubebuilder:printcolumn:name="Port",type="integer",JSONPath=".spec.task.destinationPort",description="目标端口"
//+kubebuilder:printcolumn:name="Protocol",type="string",JSONPath=".spec.task.protocol",description="协议类型"
//+kubebuilder:printcolumn:name="Issues",type="integer",JSONPath=".status.issues",description="发现问题数"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Skoop 是Kubernetes网络诊断资源的定义
type Skoop struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SkoopSpec   `json:"spec,omitempty"`
	Status SkoopStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SkoopList 包含Skoop资源的列表
type SkoopList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Skoop `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Skoop{}, &SkoopList{})
}
