package utils

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// FioOutput represents the structure of fio JSON output
type FioOutput struct {
	Jobs []struct {
		JobName string `json:"jobname"`
		Read    struct {
			IOPS        float64 `json:"iops"`
			BW          float64 `json:"bw"`
			Latency     float64 `json:"lat_ns,omitempty"`
			LatencyUsec float64 `json:"lat,omitempty"`
			LatencyNs   struct {
				Min        float64 `json:"min"`
				Max        float64 `json:"max"`
				Mean       float64 `json:"mean"`
				Stddev     float64 `json:"stddev"`
				Percentile map[string]float64
			} `json:"lat_ns"`
		} `json:"read"`
		Write struct {
			IOPS        float64 `json:"iops"`
			BW          float64 `json:"bw"`
			Latency     float64 `json:"lat_ns,omitempty"`
			LatencyUsec float64 `json:"lat,omitempty"`
			LatencyNs   struct {
				Min        float64 `json:"min"`
				Max        float64 `json:"max"`
				Mean       float64 `json:"mean"`
				Stddev     float64 `json:"stddev"`
				Percentile map[string]float64
			} `json:"lat_ns"`
		} `json:"write"`
	} `json:"jobs"`
	Status string `json:"status"` // 测试状态 (如 "SUCCESS", "FAILED")
}

// DigOutput 表示 dig 命令输出的结构化表示
type DigOutput struct {
	// 查询信息
	Command     string  `json:"command"`     // 执行的 dig 命令
	Status      string  `json:"status"`      // 查询状态 (NOERROR, NXDOMAIN 等)
	QueryTime   float64 `json:"queryTime"`   // 查询耗时（毫秒）
	Server      string  `json:"server"`      // 查询的 DNS 服务器
	When        string  `json:"when"`        // 查询时间
	MessageSize int     `json:"messageSize"` // 消息大小（字节）

	// 查询的域名信息
	QuestionSection []string `json:"questionSection"` // 查询内容
	AnswerSection   []string `json:"answerSection"`   // 应答内容

	// 解析的 IP 地址
	IpAddresses []string `json:"ipAddresses"` // 解析出的 IP 地址列表
}

// PingOutput represents the parsed output of a ping command
type PingOutput struct {
	Host            string    `json:"host"`            // 目标主机
	Transmitted     int       `json:"transmitted"`     // 已发送的数据包数
	Received        int       `json:"received"`        // 已接收的数据包数
	PacketLoss      float64   `json:"packetLoss"`      // 数据包丢失率 (%)
	MinRtt          float64   `json:"minRtt"`          // 最小往返时间 (ms)
	AvgRtt          float64   `json:"avgRtt"`          // 平均往返时间 (ms)
	MaxRtt          float64   `json:"maxRtt"`          // 最大往返时间 (ms)
	StdDevRtt       float64   `json:"stdDevRtt"`       // 往返时间标准差 (ms)
	SuccessfulPings []float64 `json:"successfulPings"` // 所有成功的ping往返时间
	Status          string    `json:"status"`          // 测试状态 (如 "SUCCESS", "FAILED")
}

// NCOutput 表示 nc 命令输出的结构化表示
type NCOutput struct {
	Host              string  `json:"host"`              // 目标主机
	Port              int32   `json:"port"`              // 目标端口
	Protocol          string  `json:"protocol"`          // 协议类型
	ConnectionSuccess bool    `json:"connectionSuccess"` // 连接是否成功
	ConnectionLatency float64 `json:"connectionLatency"` // 连接延迟 (ms)
	ErrorMessage      string  `json:"errorMessage"`      // 错误信息（如果有）
	ExtraInfo         string  `json:"extraInfo"`         // 额外信息（如服务器banner等）
	Status            string  `json:"status"`            // 测试状态 (如 "SUCCESS", "FAILED")
}

// TcpPingOutput 表示 TCP Ping 输出的结构化表示
type TcpPingOutput struct {
	Host          string    `json:"host"`          // 目标主机
	Port          int32     `json:"port"`          // 目标端口
	Transmitted   int32     `json:"transmitted"`   // 发送的包数
	Received      int32     `json:"received"`      // 接收的包数
	PacketLoss    float64   `json:"packetLoss"`    // 丢包率 (%)
	MinLatency    float64   `json:"minLatency"`    // 最小延迟 (ms)
	AvgLatency    float64   `json:"avgLatency"`    // 平均延迟 (ms)
	MaxLatency    float64   `json:"maxLatency"`    // 最大延迟 (ms)
	StdDevLatency float64   `json:"stdDevLatency"` // 延迟标准差 (ms)
	RttValues     []float64 `json:"rttValues"`     // 所有RTT值列表
	Status        string    `json:"status"`        // 测试状态 (如 "SUCCESS", "FAILED")
}

// IperfOutput 表示 iperf 命令输出的结构化表示
type IperfOutput struct {
	Host             string                `json:"host"`             // 目标主机
	Port             int32                 `json:"port"`             // 目标端口
	Protocol         string                `json:"protocol"`         // 协议类型
	Duration         float64               `json:"duration"`         // 测试持续时间 (s)
	SentBytes        int64                 `json:"sentBytes"`        // 发送的字节数
	ReceivedBytes    int64                 `json:"receivedBytes"`    // 接收的字节数
	SendBandwidth    float64               `json:"sendBandwidth"`    // 发送带宽 (bits/s)
	ReceiveBandwidth float64               `json:"receiveBandwidth"` // 接收带宽 (bits/s)
	Jitter           float64               `json:"jitter"`           // 抖动 (ms)
	LostPackets      int32                 `json:"lostPackets"`      // 丢失的数据包数
	LostPercent      float64               `json:"lostPercent"`      // 丢包率 (%)
	Retransmits      int32                 `json:"retransmits"`      // 重传次数
	RttMs            float64               `json:"rttMs"`            // 往返时间 (ms)
	CpuUtilization   float64               `json:"cpuUtilization"`   // CPU利用率 (%)
	IntervalReports  []IperfIntervalReport `json:"intervalReports"`  // 间隔报告
	Status           string                `json:"status"`           // 测试状态 (如 "SUCCESS", "FAILED")
}

// IperfIntervalReport 表示 iperf 间隔报告
type IperfIntervalReport struct {
	Interval         string  `json:"interval"`         // 间隔时间范围
	SentBytes        int64   `json:"sentBytes"`        // 此间隔发送的字节数
	ReceivedBytes    int64   `json:"receivedBytes"`    // 此间隔接收的字节数
	SendBandwidth    float64 `json:"sendBandwidth"`    // 此间隔发送带宽 (bits/s)
	ReceiveBandwidth float64 `json:"receiveBandwidth"` // 此间隔接收带宽 (bits/s)
}

// SkoopOutput 表示 skoop 命令输出的结构化表示
type SkoopOutput struct {
	SourceAddress      string       `json:"sourceAddress"`      // 源地址
	DestinationAddress string       `json:"destinationAddress"` // 目标地址
	DestinationPort    int32        `json:"destinationPort"`    // 目标端口
	Protocol           string       `json:"protocol"`           // 协议类型
	Status             string       `json:"status"`             // 诊断状态
	Path               []SkoopNode  `json:"path"`               // 诊断路径
	Issues             []SkoopIssue `json:"issues"`             // 诊断问题
	Summary            string       `json:"summary"`            // 诊断概要
}

// SkoopNode 表示网络路径上的一个节点
type SkoopNode struct {
	Type      string  `json:"type"`      // 节点类型
	Name      string  `json:"name"`      // 节点名称
	IP        string  `json:"ip"`        // 节点IP
	Namespace string  `json:"namespace"` // 命名空间
	NodeName  string  `json:"nodeName"`  // 所属节点
	Interface string  `json:"interface"` // 网络接口
	Protocol  string  `json:"protocol"`  // 协议
	LatencyMs float64 `json:"latencyMs"` // 延迟(ms)
}

// SkoopIssue 表示诊断发现的问题
type SkoopIssue struct {
	Type     string `json:"type"`     // 问题类型
	Level    string `json:"level"`    // 问题级别
	Message  string `json:"message"`  // 问题描述
	Location string `json:"location"` // 问题位置
	Solution string `json:"solution"` // 解决方案
}

// PrepareFioJob 准备执行Fio测试的Job
func PrepareFioJob(ctx context.Context, k8sClient client.Client, fio *testtoolsv1.Fio) (string, error) {
	logger := log.FromContext(ctx)

	// 构建命令参数
	args := BuildFioArgs(fio)

	// 创建带有标签的Job
	labels := map[string]string{
		"app":                          "testtools",
		"type":                         "fio",
		"test-name":                    fio.Name,
		"app.kubernetes.io/created-by": "testtools-controller",
		"app.kubernetes.io/managed-by": "fio-controller",
		"app.kubernetes.io/name":       "fio-job",
		"app.kubernetes.io/instance":   fio.Name,
		"app.kubernetes.io/part-of":    "testtools",
		"app.kubernetes.io/component":  "job",
	}

	// 使用固定格式的job名称，避免创建多个job
	baseName := strings.TrimSuffix(fio.Name, "-job") // 移除可能存在的-job后缀
	jobName := fmt.Sprintf("fio-%s-job", baseName)
	if fio.Spec.Schedule != "" {
		jobName = fmt.Sprintf("%s-%s-job", baseName, GenerateShortUID())
	}

	// 检查Job是否已存在
	var existingJob batchv1.Job
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: fio.Namespace,
		Name:      jobName,
	}, &existingJob)

	if err == nil {
		// Job已存在，检查其状态
		if existingJob.Status.Succeeded > 0 {
			// Job已成功完成，可以删除并创建新Job
			logger.Info("已存在已完成的Fio Job",
				"job", jobName,
				"namespace", fio.Namespace)

			//if err := k8sClient.Delete(ctx, &existingJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			//	logger.Error(err, "删除已完成的Fio Job失败")
			//	return "", err
			//}
			// 等待Job被删除
			//time.Sleep(2 * time.Second)
			return jobName, nil
		} else if existingJob.Status.Failed > 0 {
			// Job已失败，可以删除并创建新Job
			logger.Info("已存在失败的Fio Job",
				"job", jobName,
				"namespace", fio.Namespace)

			//if err := k8sClient.Delete(ctx, &existingJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			//	logger.Error(err, "删除失败的Fio Job失败")
			//	return "", err
			//}
			// 等待Job被删除
			//time.Sleep(2 * time.Second)
			return jobName, nil
		} else {
			// Job正在运行，直接返回Job
			logger.Info("Fio Job正在运行中，不需要重新创建",
				"job", jobName,
				"namespace", fio.Namespace)
			return jobName, nil
		}
	} else if !errors.IsNotFound(err) {
		// 发生了除"未找到"之外的错误
		logger.Error(err, "检查Fio Job是否存在时出错")
		return "", err
	}

	// 创建Job
	job, err := CreateJobForCommand(ctx, k8sClient, fio.Namespace, jobName, "fio", args, labels, fio.Spec.Image, fio.Spec.NodeName, fio.Spec.NodeSelector)
	if err != nil {
		logger.Error(err, "创建Job失败")
		return "", err
	}

	// 设置所有者引用，使Job在Fio资源被删除时自动清理
	job.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: fio.APIVersion,
			Kind:       fio.Kind,
			Name:       fio.Name,
			UID:        fio.UID,
			Controller: pointer.Bool(true),
		},
	}

	// 更新Job添加所有者引用
	if err := k8sClient.Update(ctx, job); err != nil {
		logger.Error(err, "更新Fio Job的所有者引用失败")
		return "", err
	}

	logger.Info("成功创建并更新Fio Job", "name", jobName)
	return jobName, nil
}

// BuildFioArgs 构建FIO命令行参数
func BuildFioArgs(fio *testtoolsv1.Fio) []string {
	args := []string{
		"--filename=" + fio.Spec.FilePath,
		"--name=" + fio.Spec.JobName,
		"--rw=" + fio.Spec.ReadWrite,
		"--bs=" + fio.Spec.BlockSize,
		"--iodepth=" + fmt.Sprintf("%d", fio.Spec.IODepth),
		"--size=" + fio.Spec.Size,
		"--ioengine=" + fio.Spec.IOEngine,
		"--numjobs=" + fmt.Sprintf("%d", fio.Spec.NumJobs),
		"--output-format=json",
		"--time_based",
	}

	if fio.Spec.DirectIO {
		args = append(args, "--direct=1")
	} else {
		args = append(args, "--direct=0")
	}

	if fio.Spec.Group {
		args = append(args, "--group_reporting")
	}

	if fio.Spec.KernelFilesystemBufferCache {
		args = append(args, "--buffered=1")
	} else {
		args = append(args, "--buffered=0")
	}

	if fio.Spec.Runtime > 0 {
		args = append(args, fmt.Sprintf("--runtime=%d", fio.Spec.Runtime))
	} else {
		args = append(args, "--runtime=30") // 默认运行30秒
	}

	// 添加额外的自定义参数
	for k, v := range fio.Spec.ExtraParams {
		args = append(args, fmt.Sprintf("--%s=%s", k, v))
	}

	return args
}

// ParseFioOutput 解析FIO输出并提取性能指标
func ParseFioOutput(outputStr string) (testtoolsv1.FioStats, *FioOutput, error) {
	stats := testtoolsv1.FioStats{
		LatencyPercentiles: make(map[string]string),
	}

	// 记录原始输出长度，便于调试
	outputLength := len(outputStr)
	if outputLength == 0 {
		return stats, nil, fmt.Errorf("FIO输出为空")
	}

	// 尝试定位JSON部分
	// 找到第一个左大括号
	jsonStart := strings.Index(outputStr, "{")
	if jsonStart < 0 {
		return stats, nil, fmt.Errorf("FIO输出中未找到JSON开始标记 '{': %s", truncateString(outputStr, 200))
	}

	// 找到最后一个右大括号
	jsonEnd := strings.LastIndex(outputStr, "}")
	if jsonEnd < 0 {
		return stats, nil, fmt.Errorf("FIO输出中未找到JSON结束标记 '}': %s", truncateString(outputStr, 200))
	}

	if jsonEnd <= jsonStart {
		return stats, nil, fmt.Errorf("FIO输出中JSON格式无效，结束标记位于开始标记之前: %s", truncateString(outputStr, 200))
	}

	// 提取可能的JSON部分
	jsonStr := outputStr[jsonStart : jsonEnd+1]

	// 尝试解析JSON
	var fioOutput FioOutput
	err := json.Unmarshal([]byte(jsonStr), &fioOutput)
	if err != nil {
		// 尝试清理JSON字符串
		cleanJsonStr := cleanJsonString(jsonStr)
		// 再次尝试解析
		err = json.Unmarshal([]byte(cleanJsonStr), &fioOutput)
		if err != nil {
			return stats, nil, fmt.Errorf("无法解析FIO输出: %v, 提取的JSON字符串: %s", err, truncateString(jsonStr, 200))
		}
	}

	// 提取性能指标
	if len(fioOutput.Jobs) == 0 {
		return stats, nil, fmt.Errorf("FIO输出中未找到任何作业数据")
	}

	// 使用第一个作业的结果，或者如果使用了group_reporting则是合并的结果
	job := fioOutput.Jobs[0]

	// 读取IOPS和带宽
	stats.ReadIOPS = fmt.Sprintf("%f", job.Read.IOPS)
	stats.WriteIOPS = fmt.Sprintf("%f", job.Write.IOPS)
	stats.ReadBW = fmt.Sprintf("%f", job.Read.BW)
	stats.WriteBW = fmt.Sprintf("%f", job.Write.BW)

	// 读取延迟，处理可能的单位差异 (纳秒、微秒)
	readLatency := job.Read.LatencyNs.Mean
	if readLatency == 0 && job.Read.LatencyUsec > 0 {
		readLatency = job.Read.LatencyUsec * 1000 // 转换为纳秒
	}
	if readLatency > 0 {
		// 转换为微秒 (1纳秒 = 0.001微秒)
		stats.ReadLatency = fmt.Sprintf("%.2f", readLatency/1000)
	} else {
		// 如果没有读取延迟数据，设置为0，避免空值
		stats.ReadLatency = "0.00"
	}

	writeLatency := job.Write.LatencyNs.Mean
	if writeLatency == 0 && job.Write.LatencyUsec > 0 {
		writeLatency = job.Write.LatencyUsec * 1000 // 转换为纳秒
	}
	if writeLatency > 0 {
		// 转换为微秒 (1纳秒 = 0.001微秒)
		stats.WriteLatency = fmt.Sprintf("%.2f", writeLatency/1000)
	} else {
		// 如果没有写入延迟数据，设置为0，避免空值
		stats.WriteLatency = "0.00"
	}

	// 提取百分位延迟
	stats.LatencyPercentiles = make(map[string]string)
	for percentile, value := range job.Read.LatencyNs.Percentile {
		stats.LatencyPercentiles["read_"+percentile] = fmt.Sprintf("%f", value/1000000) // 转换为毫秒
	}
	for percentile, value := range job.Write.LatencyNs.Percentile {
		stats.LatencyPercentiles["write_"+percentile] = fmt.Sprintf("%f", value/1000000) // 转换为毫秒
	}
	if len(fioOutput.Jobs) > 0 {
		fioOutput.Status = "SUCCESS"
	} else {
		fioOutput.Status = "FAILED"
	}

	return stats, &fioOutput, nil
}

// truncateString 截断字符串，用于日志记录
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(已截断)"
}

// cleanJsonString 清理JSON字符串，删除可能导致解析问题的字符
func cleanJsonString(jsonStr string) string {
	// 替换常见的非标准JSON格式问题
	// 1. 删除控制字符
	re := regexp.MustCompile("[\x00-\x1F\x7F]")
	cleanStr := re.ReplaceAllString(jsonStr, "")

	// 2. 修复常见的JSON语法错误
	cleanStr = strings.ReplaceAll(cleanStr, ",,", ",") // 删除连续逗号
	cleanStr = strings.ReplaceAll(cleanStr, ",]", "]") // 删除最后逗号
	cleanStr = strings.ReplaceAll(cleanStr, ",}", "}") // 删除最后逗号

	return cleanStr
}

// PrepareDigJob 准备执行Dig测试的Job
func PrepareDigJob(ctx context.Context, k8sClient client.Client, dig *testtoolsv1.Dig) (string, error) {
	logger := log.FromContext(ctx)

	// 构建Dig命令参数
	args := BuildDigArgs(dig)

	// 创建带有标签的Job
	labels := map[string]string{
		"app":                          "testtools",
		"type":                         "dig",
		"test-name":                    dig.Name,
		"app.kubernetes.io/created-by": "testtools-controller",
		"app.kubernetes.io/managed-by": "dig-controller",
		"app.kubernetes.io/name":       "dig-job",
		"app.kubernetes.io/instance":   dig.Name,
		"app.kubernetes.io/part-of":    "testtools",
		"app.kubernetes.io/component":  "job",
	}

	// 使用固定格式的job名称，避免创建多个job
	baseName := strings.TrimSuffix(dig.Name, "-job") // 移除可能存在的-job后缀
	jobName := fmt.Sprintf("dig-%s-job", baseName)
	if dig.Spec.Schedule != "" {
		jobName = fmt.Sprintf("%s-%s-job", baseName, GenerateShortUID())
	}

	// 检查Job是否已存在
	var existingJob batchv1.Job
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: dig.Namespace,
		Name:      jobName,
	}, &existingJob)

	if err == nil {
		// Job已存在，检查其状态
		if existingJob.Status.Succeeded > 0 {
			// Job已成功完成，可以删除并创建新Job
			logger.Info("已存在已完成的Dig Job",
				"job", jobName,
				"namespace", dig.Namespace)
			//if err := k8sClient.Delete(ctx, &existingJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			//	logger.Error(err, "删除已完成的Dig Job失败")
			//	return "", err
			//}
			// 等待Job被删除
			//time.Sleep(2 * time.Second)
			return jobName, nil
		} else if existingJob.Status.Failed > 0 {
			// Job已失败，可以删除并创建新Job
			logger.Info("已存在失败的Dig Job",
				"job", jobName,
				"namespace", dig.Namespace)
			//if err := k8sClient.Delete(ctx, &existingJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			//	logger.Error(err, "删除失败的Dig Job失败")
			//	return "", err
			//}
			// 等待Job被删除
			//time.Sleep(2 * time.Second)
			return jobName, nil
		} else {
			// Job正在运行，直接返回Job名称
			logger.Info("Dig Job正在运行中，不需要重新创建",
				"job", jobName,
				"namespace", dig.Namespace)
			return jobName, nil
		}
	} else if !errors.IsNotFound(err) {
		// 发生了除"未找到"之外的错误
		logger.Error(err, "检查Dig Job是否存在时出错")
		return "", err
	}

	// 创建新Job - 传递作业名称时确保没有重复的"-job"后缀
	job, err := CreateJobForCommand(ctx, k8sClient, dig.Namespace, jobName, "dig", args, labels, dig.Spec.Image, dig.Spec.NodeName, dig.Spec.NodeSelector)
	if err != nil {
		logger.Error(err, "创建Dig Job失败")
		return "", err
	}

	// 设置所有者引用
	job.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: dig.APIVersion,
			Kind:       dig.Kind,
			Name:       dig.Name,
			UID:        dig.UID,
			Controller: pointer.Bool(true),
		},
	}

	// 更新Job
	if err := k8sClient.Update(ctx, job); err != nil {
		logger.Error(err, "更新Dig Job的所有者引用失败")
		return "", err
	}

	return jobName, nil
}

// BuildDigArgs 构建Dig命令行参数
func BuildDigArgs(dig *testtoolsv1.Dig) []string {
	var args []string

	// Add the server if specified
	if dig.Spec.Server != "" {
		args = append(args, "@"+dig.Spec.Server)
	}

	// Add source IP and port if specified
	if dig.Spec.SourceIP != "" {
		sourceArg := "-b " + dig.Spec.SourceIP
		if dig.Spec.SourcePort > 0 {
			sourceArg += "#" + strconv.Itoa(int(dig.Spec.SourcePort))
		}
		args = append(args, sourceArg)
	}

	// Add query class if specified
	if dig.Spec.QueryClass != "" {
		args = append(args, "-c", dig.Spec.QueryClass)
	}

	// Add query file if specified
	if dig.Spec.QueryFile != "" {
		args = append(args, "-f", dig.Spec.QueryFile)
	}

	// Add key file if specified
	if dig.Spec.KeyFile != "" {
		args = append(args, "-k", dig.Spec.KeyFile)
	}

	// Add port if specified
	if dig.Spec.Port > 0 {
		args = append(args, "-p", strconv.Itoa(int(dig.Spec.Port)))
	}

	// Add query name if specified
	if dig.Spec.QueryName != "" {
		args = append(args, "-q", dig.Spec.QueryName)
	}

	// Add query type if specified
	if dig.Spec.QueryType != "" {
		args = append(args, "-t", dig.Spec.QueryType)
	}

	// Add microseconds flag if specified
	if dig.Spec.UseMicroseconds {
		args = append(args, "-u")
	}

	// Add reverse query if specified
	if dig.Spec.ReverseQuery != "" {
		args = append(args, "-x", dig.Spec.ReverseQuery)
	}

	// Add TSIG key if specified
	if dig.Spec.TSIGKey != "" {
		args = append(args, "-y", dig.Spec.TSIGKey)
	}

	// Add IPv4/IPv6 flags if specified
	if dig.Spec.UseIPv4Only {
		args = append(args, "-4")
	}
	if dig.Spec.UseIPv6Only {
		args = append(args, "-6")
	}

	// Add TCP flag if specified
	if dig.Spec.UseTCP {
		args = append(args, "+tcp")
	}

	// Add timeout if specified
	if dig.Spec.Timeout > 0 {
		args = append(args, "+time="+strconv.Itoa(int(dig.Spec.Timeout)))
	}

	// Add the domain (required)
	args = append(args, dig.Spec.Domain)

	return args
}

// ParseDigOutput 解析Dig输出
func ParseDigOutput(output string) *DigOutput {
	digOutput := &DigOutput{
		QuestionSection: []string{},
		AnswerSection:   []string{},
		IpAddresses:     []string{},
	}

	// 提取命令信息
	commandLine := ExtractSection(output, "; <<>> DiG", "\n")
	digOutput.Command = commandLine

	// 提取状态信息
	headerLine := ExtractSection(output, ";; ->>HEADER<<-", "\n")
	if headerLine != "" {
		if strings.Contains(headerLine, "status: NOERROR") {
			digOutput.Status = "NOERROR"
		} else if strings.Contains(headerLine, "status: NXDOMAIN") {
			digOutput.Status = "NXDOMAIN"
		} else if strings.Contains(headerLine, "status:") {
			parts := strings.Split(headerLine, "status:")
			if len(parts) > 1 {
				statusPart := strings.TrimSpace(parts[1])
				statusParts := strings.Split(statusPart, ",")
				if len(statusParts) > 0 {
					digOutput.Status = strings.TrimSpace(statusParts[0])
				}
			}
		}
	}

	// 提取问题部分
	questionSection := ExtractMultilineSection(output, ";; QUESTION SECTION:", ";; ")
	if questionSection != "" {
		lines := strings.Split(questionSection, "\n")
		for _, line := range lines {
			trimmedLine := strings.TrimSpace(line)
			if trimmedLine != "" {
				digOutput.QuestionSection = append(digOutput.QuestionSection, trimmedLine)
			}
		}
	}

	// 提取回答部分
	answerSection := ExtractMultilineSection(output, ";; ANSWER SECTION:", ";; ")
	if answerSection != "" {
		lines := strings.Split(answerSection, "\n")
		for _, line := range lines {
			trimmedLine := strings.TrimSpace(line)
			if trimmedLine != "" {
				digOutput.AnswerSection = append(digOutput.AnswerSection, trimmedLine)

				// 尝试提取IP地址
				fields := strings.Fields(trimmedLine)
				if len(fields) >= 5 {
					recordType := fields[3]
					if recordType == "A" || recordType == "AAAA" {
						ip := fields[4]
						digOutput.IpAddresses = append(digOutput.IpAddresses, ip)
					}
				}
			}
		}
	}

	// 提取查询时间
	queryTimeLine := ExtractSection(output, ";; Query time:", "\n")
	if queryTimeLine != "" {
		digOutput.QueryTime = ParseResponseTime(queryTimeLine)
	}

	// 提取服务器信息
	serverLine := ExtractSection(output, ";; SERVER:", "\n")
	if serverLine != "" {
		parts := strings.SplitN(serverLine, ":", 2)
		if len(parts) > 1 {
			digOutput.Server = strings.TrimSpace(parts[1])
		}
	}

	// 提取查询时间
	whenLine := ExtractSection(output, ";; WHEN:", "\n")
	if whenLine != "" {
		parts := strings.SplitN(whenLine, ":", 2)
		if len(parts) > 1 {
			digOutput.When = strings.TrimSpace(parts[1])
		}
	}

	// 提取消息大小
	msgSizeLine := ExtractSection(output, ";; MSG SIZE", "\n")
	if msgSizeLine != "" {
		parts := strings.Split(msgSizeLine, "rcvd:")
		if len(parts) > 1 {
			sizeStr := strings.TrimSpace(parts[1])
			size, err := strconv.Atoi(sizeStr)
			if err == nil {
				digOutput.MessageSize = size
			}
		}
	}

	if digOutput.Status == "NOERROR" {
		digOutput.Status = "SUCCESS"
	} else {
		digOutput.Status = "FAILED"
	}

	return digOutput
}

// ParseResponseTime 解析响应时间
func ParseResponseTime(output string) float64 {
	// Sample dig output line: ";; Query time: 23 msec"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Query time") {
			parts := strings.Split(line, ":")
			if len(parts) < 2 {
				continue
			}
			timePart := strings.TrimSpace(parts[1])
			msecParts := strings.Split(timePart, " ")
			if len(msecParts) < 2 {
				continue
			}
			if val, err := strconv.ParseFloat(msecParts[0], 64); err == nil {
				return val
			}
		}
	}
	return 0
}

// GenerateShortUID 生成一个短的唯一标识符
func GenerateShortUID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// 如果出错，使用时间戳作为备选
		return fmt.Sprintf("%x", time.Now().UnixNano()%0xFFFFFFFF)
	}
	return hex.EncodeToString(b)
}

// PreparePingJob 准备并执行Ping测试Job
func PreparePingJob(ctx context.Context, k8sClient client.Client, ping *testtoolsv1.Ping) (string, error) {
	logger := log.FromContext(ctx)
	logger.Info("准备执行Ping测试", "ping", ping.Name, "host", ping.Spec.Host)

	// 构建ping命令参数
	args := BuildPingArgs(ping)

	// 为Job创建标签
	labels := map[string]string{
		"app":                          "testtools",
		"type":                         "ping",
		"test-name":                    ping.Name,
		"app.kubernetes.io/created-by": "testtools-controller",
		"app.kubernetes.io/managed-by": "ping-controller",
		"app.kubernetes.io/name":       "ping-job",
		"app.kubernetes.io/instance":   ping.Name,
		"app.kubernetes.io/part-of":    "testtools",
		"app.kubernetes.io/component":  "job",
	}

	// 创建Job名称 - 避免重复"-job"后缀
	baseName := strings.TrimSuffix(ping.Name, "-job")
	jobName := fmt.Sprintf("%s-job", baseName)
	if ping.Spec.Schedule != "" {
		jobName = fmt.Sprintf("%s-%s-job", baseName, GenerateShortUID())
	}

	// 检查Job是否已存在
	var existingJob batchv1.Job
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: ping.Namespace,
		Name:      jobName,
	}, &existingJob)

	if err == nil {
		// Job已存在，检查其状态
		if existingJob.Status.Succeeded > 0 {
			// Job已成功完成，可以删除并创建新Job
			logger.Info("已存在已完成的Ping Job",
				"job", jobName,
				"namespace", ping.Namespace)
			//if err := k8sClient.Delete(ctx, &existingJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			//	logger.Error(err, "删除已完成的Ping Job失败")
			//	return "", err
			//}
			// 等待Job被删除
			//time.Sleep(2 * time.Second)
			return jobName, nil
		} else if existingJob.Status.Failed > 0 {
			// Job已失败，可以删除并创建新Job
			logger.Info("已存在失败的Ping Job",
				"job", jobName,
				"namespace", ping.Namespace)
			//if err := k8sClient.Delete(ctx, &existingJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			//	logger.Error(err, "删除失败的Ping Job失败")
			//	return "", err
			//}
			// 等待Job被删除
			//time.Sleep(2 * time.Second)
			return jobName, nil
		} else {
			// Job正在运行，直接返回Job名称
			logger.Info("Ping Job正在运行中，不需要重新创建",
				"job", jobName,
				"namespace", ping.Namespace)
			return jobName, nil
		}
	} else if !errors.IsNotFound(err) {
		// 发生了除"未找到"之外的错误
		logger.Error(err, "检查Ping Job是否存在时出错")
		return "", err
	}

	logger.Info("创建新的Ping Job", "name", jobName, "args", args)

	// 创建新Job - 传递作业名称时确保没有重复的"-job"后缀
	job, err := CreateJobForCommand(ctx, k8sClient, ping.Namespace, jobName, "ping", args, labels, ping.Spec.Image, ping.Spec.NodeName, ping.Spec.NodeSelector)
	if err != nil {
		logger.Error(err, "创建Ping Job失败")
		return "", err
	}

	// 设置所有者引用
	job.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: ping.APIVersion,
			Kind:       ping.Kind,
			Name:       ping.Name,
			UID:        ping.UID,
			Controller: pointer.Bool(true),
		},
	}

	// 更新Job
	if err := k8sClient.Update(ctx, job); err != nil {
		logger.Error(err, "更新Ping Job的所有者引用失败")
		return "", err
	}

	logger.Info("成功创建并更新Ping Job", "name", jobName)
	return jobName, nil
}

// BuildPingArgs 构建Ping命令行参数
func BuildPingArgs(ping *testtoolsv1.Ping) []string {
	var args []string

	// 计数参数
	args = append(args, "-c", fmt.Sprintf("%d", ping.Spec.Count))

	// 间隔参数
	if ping.Spec.Interval > 0 {
		args = append(args, "-i", fmt.Sprintf("%d", ping.Spec.Interval))
	}

	// 包大小参数
	if ping.Spec.PacketSize > 0 {
		args = append(args, "-s", fmt.Sprintf("%d", ping.Spec.PacketSize))
	}

	// 超时参数
	if ping.Spec.Timeout > 0 {
		args = append(args, "-W", fmt.Sprintf("%d", ping.Spec.Timeout))
	}

	// TTL参数
	if ping.Spec.TTL > 0 {
		args = append(args, "-t", fmt.Sprintf("%d", ping.Spec.TTL))
	}

	// 添加主机参数（必须）
	args = append(args, ping.Spec.Host)

	return args
}

// ParsePingOutput 解析Ping输出
func ParsePingOutput(output string, host string) *PingOutput {
	pingOutput := &PingOutput{
		Host:            host,
		SuccessfulPings: []float64{},
	}

	// 解析每个ping的往返时间
	// 不使用正则表达式来提取时间，直接使用字符串处理
	matches := strings.Split(output, "\n")
	for _, line := range matches {
		if strings.Contains(line, "time=") {
			parts := strings.Split(line, "time=")
			if len(parts) > 1 {
				timePart := strings.Split(parts[1], " ")[0]
				if val, err := strconv.ParseFloat(timePart, 64); err == nil {
					pingOutput.SuccessfulPings = append(pingOutput.SuccessfulPings, val)
				}
			}
		}
	}

	// 解析统计部分
	ParsePingStatistics(output, pingOutput)
	if pingOutput.Received > 0 {
		pingOutput.Status = "SUCCESS"
	} else {
		pingOutput.Status = "FAILED"
	}

	return pingOutput
}

// ParsePingStatistics 解析Ping统计信息
func ParsePingStatistics(output string, pingOutput *PingOutput) {
	stats := ExtractSection(output, "--- ping statistics ---", "")
	if stats == "" {
		// 尝试其他可能的分割标记
		stats = ExtractSection(output, "Ping statistics for", "")
		if stats == "" {
			// 如果仍然找不到统计部分，检查是否有成功的ping响应
			if len(pingOutput.SuccessfulPings) > 0 {
				// 有成功的ping响应，手动设置接收包数
				pingOutput.Received = len(pingOutput.SuccessfulPings)
				if pingOutput.Transmitted == 0 {
					// 如果发送包数未设置，假设等于或略大于接收包数
					pingOutput.Transmitted = pingOutput.Received
				}
				// 计算统计数据
				calculatePingStats(pingOutput)
			}
			return
		}
	}

	receivedFound := false
	transmittedFound := false

	lines := strings.Split(stats, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 更全面的发送/接收包数解析
		if strings.Contains(line, "packets transmitted") ||
			strings.Contains(line, "Packets: Sent") ||
			strings.Contains(line, "transmitted") {

			// 检查多种格式的transmitted
			transmittedRegex := regexp.MustCompile(`(\d+)(?:\s+packets)?\s+transmitted`)
			if matches := transmittedRegex.FindStringSubmatch(line); len(matches) > 1 {
				if val, err := strconv.Atoi(matches[1]); err == nil {
					pingOutput.Transmitted = val
					transmittedFound = true
				}
			}

			// 检查多种格式的received
			receivedRegex := regexp.MustCompile(`(\d+)(?:\s+received|\s+packets\s+received)`)
			if matches := receivedRegex.FindStringSubmatch(line); len(matches) > 1 {
				if val, err := strconv.Atoi(matches[1]); err == nil {
					pingOutput.Received = val
					receivedFound = true
				}
			}

			// 检查丢包率
			lossRegex := regexp.MustCompile(`(\d+(?:\.\d+)?)%\s+(?:packet\s+)?loss`)
			if matches := lossRegex.FindStringSubmatch(line); len(matches) > 1 {
				if val, err := strconv.ParseFloat(matches[1], 64); err == nil {
					pingOutput.PacketLoss = val
				}
			}
		}

		if strings.Contains(line, "min/avg/max") ||
			strings.Contains(line, "Minimum") ||
			strings.Contains(line, "rtt min/avg/max/mdev") {

			rttRegex := regexp.MustCompile(`(?:min/avg/max(?:/[a-z]+)?\s*=\s*|rtt min/avg/max(?:/[a-z]+)?\s*=\s*)([0-9.]+)/([0-9.]+)/([0-9.]+)(?:/([0-9.]+))?`)
			if matches := rttRegex.FindStringSubmatch(line); len(matches) > 3 {
				if val, err := strconv.ParseFloat(matches[1], 64); err == nil {
					pingOutput.MinRtt = val
				}
				if val, err := strconv.ParseFloat(matches[2], 64); err == nil {
					pingOutput.AvgRtt = val
				}
				if val, err := strconv.ParseFloat(matches[3], 64); err == nil {
					pingOutput.MaxRtt = val
				}
				if len(matches) > 4 && matches[4] != "" {
					if val, err := strconv.ParseFloat(matches[4], 64); err == nil {
						pingOutput.StdDevRtt = val
					}
				}
			}
		}

		// Windows格式的单独RTT值解析
		if strings.Contains(line, "Minimum") {
			minRegex := regexp.MustCompile(`Minimum\s*[=:]\s*([0-9.]+)\s*ms`)
			if matches := minRegex.FindStringSubmatch(line); len(matches) > 1 {
				if val, err := strconv.ParseFloat(matches[1], 64); err == nil {
					pingOutput.MinRtt = val
				}
			}
		}
		if strings.Contains(line, "Maximum") {
			maxRegex := regexp.MustCompile(`Maximum\s*[=:]\s*([0-9.]+)\s*ms`)
			if matches := maxRegex.FindStringSubmatch(line); len(matches) > 1 {
				if val, err := strconv.ParseFloat(matches[1], 64); err == nil {
					pingOutput.MaxRtt = val
				}
			}
		}
		if strings.Contains(line, "Average") {
			avgRegex := regexp.MustCompile(`Average\s*[=:]\s*([0-9.]+)\s*ms`)
			if matches := avgRegex.FindStringSubmatch(line); len(matches) > 1 {
				if val, err := strconv.ParseFloat(matches[1], 64); err == nil {
					pingOutput.AvgRtt = val
				}
			}
		}
	}

	// 如果收到的包或发送的包信息没有找到，但有成功的ping记录
	if (!receivedFound || !transmittedFound) && len(pingOutput.SuccessfulPings) > 0 {
		if !receivedFound {
			pingOutput.Received = len(pingOutput.SuccessfulPings)
		}
		if !transmittedFound {
			// 如果发送包数未设置，假设等于或略大于接收包数
			pingOutput.Transmitted = pingOutput.Received
		}
	}

	// 确保如果有成功ping但统计为0，则计算统计值
	calculatePingStats(pingOutput)
}

// 辅助函数：计算ping统计数据
func calculatePingStats(pingOutput *PingOutput) {
	// 如果计算不出丢包率，则基于传输和接收的包数计算
	if pingOutput.PacketLoss == 0 && pingOutput.Transmitted > 0 {
		if pingOutput.Received < pingOutput.Transmitted {
			pingOutput.PacketLoss = float64(pingOutput.Transmitted-pingOutput.Received) * 100.0 / float64(pingOutput.Transmitted)
		} else if pingOutput.Received == pingOutput.Transmitted {
			pingOutput.PacketLoss = 0.0
		}
	}

	// 确保有RTT值 - 如果没有从统计中获取，可以从成功的ping中计算
	if len(pingOutput.SuccessfulPings) > 0 {
		if pingOutput.MinRtt == 0 {
			min := pingOutput.SuccessfulPings[0]
			for _, v := range pingOutput.SuccessfulPings {
				if v < min {
					min = v
				}
			}
			pingOutput.MinRtt = min
		}

		if pingOutput.MaxRtt == 0 {
			max := pingOutput.SuccessfulPings[0]
			for _, v := range pingOutput.SuccessfulPings {
				if v > max {
					max = v
				}
			}
			pingOutput.MaxRtt = max
		}

		if pingOutput.AvgRtt == 0 {
			sum := 0.0
			for _, v := range pingOutput.SuccessfulPings {
				sum += v
			}
			pingOutput.AvgRtt = sum / float64(len(pingOutput.SuccessfulPings))
		}
	}
}

// ExtractPingStats 从 PingOutput 提取 PingStatus
//func ExtractPingStats(output *PingOutput) testtoolsv1.PingStatus {
//	status := testtoolsv1.PingStatus{
//		PacketLoss: output.PacketLoss,
//		MinRtt:     output.MinRtt,
//		AvgRtt:     output.AvgRtt,
//		MaxRtt:     output.MaxRtt,
//	}
//	return status
//}

// GenerateTestReportName 生成测试报告名称，避免重复前缀
// 控制器可以共享这个函数以保持命名一致性
func GenerateTestReportName(resourceKind string, resourceName string) string {
	kindLower := strings.ToLower(resourceKind)
	// 检查资源名称是否已包含类型前缀
	if strings.HasPrefix(resourceName, kindLower+"-") {
		return fmt.Sprintf("%s-report", resourceName)
	}
	return fmt.Sprintf("%s-%s-report", kindLower, resourceName)
}

// BuildNCArgs 构建nc命令行参数
func BuildNCArgs(nc *testtoolsv1.Nc) []string {
	var args []string

	// 添加协议选项
	if nc.Spec.Protocol == "udp" {
		args = append(args, "-u")
	}

	// 添加超时选项
	if nc.Spec.Timeout > 0 {
		args = append(args, "-w", fmt.Sprintf("%d", nc.Spec.Timeout))
	}

	// 是否开启详细日志
	if nc.Spec.Verbose {
		args = append(args, "-v")
	}

	// 是否等待连接关闭
	if nc.Spec.Wait {
		args = append(args, "-q", "1") // 在EOF后等待1秒关闭
	}

	// 仅连接测试不发送数据
	if nc.Spec.ZeroInput {
		args = append(args, "-z")
	}

	// IPv4/IPv6选项
	if nc.Spec.UseIPv4Only {
		args = append(args, "-4")
	} else if nc.Spec.UseIPv6Only {
		args = append(args, "-6")
	}

	// 添加主机和端口参数（必须）
	args = append(args, nc.Spec.Host, fmt.Sprintf("%d", nc.Spec.Port))

	return args
}

func BuildTcpPingArgs(tcpPing *testtoolsv1.TcpPing) []string {
	var args []string

	// 添加结果格式参数
	if tcpPing.Spec.Verbose {
		args = append(args, "-d") // 显示时间戳
		args = append(args, "-c") // 以列格式输出
	}

	// 添加超时时间
	if tcpPing.Spec.Timeout > 0 {
		args = append(args, "-w", fmt.Sprintf("%d", tcpPing.Spec.Timeout))
	}

	// 添加间隔时间
	if tcpPing.Spec.Interval > 0 {
		args = append(args, "-r", fmt.Sprintf("%d", tcpPing.Spec.Interval))
	}

	// 添加测试次数
	if tcpPing.Spec.Count > 0 {
		args = append(args, "-x", fmt.Sprintf("%d", tcpPing.Spec.Count))
	}

	// 添加目标主机和端口
	args = append(args, tcpPing.Spec.Host)
	if tcpPing.Spec.Port > 0 {
		args = append(args, fmt.Sprintf("%d", tcpPing.Spec.Port))
	}

	return args
}

// BuildIperfClientArgs 为Iperf客户端模式构建命令行参数
func BuildIperfClientArgs(iperf *testtoolsv1.Iperf) []string {
	var args []string

	// 添加客户端标志和目标主机
	args = append(args, "-c", iperf.Spec.Host)

	// 添加端口号
	if iperf.Spec.Port > 0 {
		args = append(args, "-p", fmt.Sprintf("%d", iperf.Spec.Port))
	} else {
		args = append(args, "-p", "5201") // 默认端口
	}

	// 添加测试时长
	if iperf.Spec.Duration > 0 {
		args = append(args, "-t", fmt.Sprintf("%d", iperf.Spec.Duration))
	} else {
		args = append(args, "-t", "10") // 默认10秒
	}

	// 添加协议选项
	if iperf.Spec.Protocol == "udp" {
		args = append(args, "-u")
	}

	// 添加并发流数量
	if iperf.Spec.Parallel > 1 {
		args = append(args, "-P", fmt.Sprintf("%d", iperf.Spec.Parallel))
	}

	// 添加缓冲区大小
	if iperf.Spec.SendBuffer > 0 {
		args = append(args, "-w", fmt.Sprintf("%d", iperf.Spec.SendBuffer))
	}

	if iperf.Spec.ReceiveBuffer > 0 {
		args = append(args, "-l", fmt.Sprintf("%d", iperf.Spec.ReceiveBuffer))
	}

	// 添加带宽限制
	if iperf.Spec.Bandwidth > 0 {
		args = append(args, "-b", fmt.Sprintf("%d", iperf.Spec.Bandwidth))
	}

	// 添加反向模式
	if iperf.Spec.Reverse {
		args = append(args, "-R")
	}

	// 添加双向模式
	if iperf.Spec.Bidirectional {
		args = append(args, "--bidir")
	}

	// 添加报告间隔
	if iperf.Spec.ReportInterval > 0 {
		args = append(args, "-i", fmt.Sprintf("%d", iperf.Spec.ReportInterval))
	}

	// 添加输出格式
	if iperf.Spec.JsonOutput {
		args = append(args, "-J")
	}

	// 添加IPv4/IPv6选项
	if iperf.Spec.UseIPv4Only {
		args = append(args, "-4")
	} else if iperf.Spec.UseIPv6Only {
		args = append(args, "-6")
	}

	// 添加详细日志
	if iperf.Spec.Verbose {
		args = append(args, "-V")
	}

	return args
}

// BuildIperfServerArgs 为Iperf服务器模式构建命令行参数
func BuildIperfServerArgs(iperf *testtoolsv1.Iperf) []string {
	var args []string

	// 添加服务器标志
	args = append(args, "-s")

	// 添加端口号
	if iperf.Spec.Port > 0 {
		args = append(args, "-p", fmt.Sprintf("%d", iperf.Spec.Port))
	} else {
		args = append(args, "-p", "5201") // 默认端口
	}

	// 添加协议选项
	if iperf.Spec.Protocol == "udp" {
		args = append(args, "-u")
	}

	// 添加报告间隔
	if iperf.Spec.ReportInterval > 0 {
		args = append(args, "-i", fmt.Sprintf("%d", iperf.Spec.ReportInterval))
	}

	// 添加缓冲区大小
	if iperf.Spec.ReceiveBuffer > 0 {
		args = append(args, "-l", fmt.Sprintf("%d", iperf.Spec.ReceiveBuffer))
	}

	// 添加输出格式
	if iperf.Spec.JsonOutput {
		args = append(args, "-J")
	}

	// 添加IPv4/IPv6选项
	if iperf.Spec.UseIPv4Only {
		args = append(args, "-4")
	} else if iperf.Spec.UseIPv6Only {
		args = append(args, "-6")
	}

	// 添加详细日志
	if iperf.Spec.Verbose {
		args = append(args, "-V")
	}

	return args
}

// BuildSkoopArgs 为Skoop构建命令行参数
func BuildSkoopArgs(skoop *testtoolsv1.Skoop) []string {
	var args []string

	// 基本命令
	args = append(args, "diagnose", "connection")

	// 添加源地址
	if skoop.Spec.Task.SourceAddress != "" {
		args = append(args, "-s", skoop.Spec.Task.SourceAddress)
	}

	// 添加目标地址和端口
	args = append(args, "-d", skoop.Spec.Task.DestinationAddress)
	args = append(args, "-p", fmt.Sprintf("%d", skoop.Spec.Task.DestinationPort))

	// 添加协议
	if skoop.Spec.Task.Protocol != "" {
		args = append(args, "--protocol", skoop.Spec.Task.Protocol)
	}

	// 添加UI相关配置
	if skoop.Spec.UI != nil {
		if skoop.Spec.UI.Format != "" {
			args = append(args, "--format", skoop.Spec.UI.Format)
		}
		if skoop.Spec.UI.OutputFile != "" {
			args = append(args, "--output", skoop.Spec.UI.OutputFile)
		}
		if skoop.Spec.UI.EnableHTTP {
			args = append(args, "--http")
			if skoop.Spec.UI.HTTPAddress != "" {
				args = append(args, "--http-address", skoop.Spec.UI.HTTPAddress)
			}
		}
	}

	// 添加日志相关配置
	if skoop.Spec.Logging != nil {
		if skoop.Spec.Logging.Verbosity > 0 {
			args = append(args, "-v", fmt.Sprintf("%d", skoop.Spec.Logging.Verbosity))
		}
		if skoop.Spec.Logging.LogFile != "" {
			args = append(args, "--log-file", skoop.Spec.Logging.LogFile)
		}
		if skoop.Spec.Logging.LogDir != "" {
			args = append(args, "--log-dir", skoop.Spec.Logging.LogDir)
		}
	}

	// 添加集群配置
	if skoop.Spec.Cluster != nil {
		if skoop.Spec.Cluster.ClusterCIDR != "" {
			args = append(args, "--cluster-cidr", skoop.Spec.Cluster.ClusterCIDR)
		}
		if skoop.Spec.Cluster.NetworkPlugin != "" {
			args = append(args, "--network-plugin", skoop.Spec.Cluster.NetworkPlugin)
		}
		if skoop.Spec.Cluster.ProxyMode != "" {
			args = append(args, "--proxy-mode", skoop.Spec.Cluster.ProxyMode)
		}
	}

	// 添加采集器配置
	if skoop.Spec.Collector != nil {
		if skoop.Spec.Collector.Image != "" {
			args = append(args, "--collector-image", skoop.Spec.Collector.Image)
		}
		if skoop.Spec.Collector.Namespace != "" {
			args = append(args, "--collector-namespace", skoop.Spec.Collector.Namespace)
		}
		if skoop.Spec.Collector.PreserveCollectorPod {
			args = append(args, "--preserve-collector-pod")
		}
	}

	return args
}

// ParseNCOutput 解析nc命令输出
func ParseNCOutput(output string, nc *testtoolsv1.Nc) *NCOutput {
	ncOutput := &NCOutput{
		Host:     nc.Spec.Host,
		Port:     nc.Spec.Port,
		Protocol: nc.Spec.Protocol,
	}

	// 检查是否有连接成功的标志
	if strings.Contains(output, "succeeded") || strings.Contains(output, "connected") {
		ncOutput.ConnectionSuccess = true
	} else if strings.Contains(output, "refused") || strings.Contains(output, "failed") || strings.Contains(output, "timed out") {
		ncOutput.ConnectionSuccess = false
	} else {
		// 如果没有明确的成功或失败标志，但输出内容不为空，通常表示连接成功
		ncOutput.ConnectionSuccess = output != "" && !strings.Contains(output, "error")
	}

	// 提取错误信息
	if !ncOutput.ConnectionSuccess {
		errorLines := []string{}
		for _, line := range strings.Split(output, "\n") {
			if strings.Contains(line, "error") || strings.Contains(line, "refused") ||
				strings.Contains(line, "failed") || strings.Contains(line, "timed out") {
				errorLines = append(errorLines, strings.TrimSpace(line))
			}
		}
		ncOutput.ErrorMessage = strings.Join(errorLines, "; ")
	}

	// 提取连接延迟，通常nc命令本身不直接提供连接延迟
	// 在实际使用时可能需要使用time命令包装nc命令来测量延迟
	if re := regexp.MustCompile(`connect time: ([0-9.]+) ms`); re.MatchString(output) {
		if matches := re.FindStringSubmatch(output); len(matches) > 1 {
			if latency, err := strconv.ParseFloat(matches[1], 64); err == nil {
				ncOutput.ConnectionLatency = latency
			}
		}
	}

	// 提取额外信息，如服务banner等
	if ncOutput.ConnectionSuccess && ncOutput.ErrorMessage == "" {
		// 去除可能的调试输出
		lines := strings.Split(output, "\n")
		infoLines := []string{}
		for _, line := range lines {
			if !strings.Contains(line, "succeeded") && !strings.Contains(line, "connected") &&
				!strings.HasPrefix(line, "nc:") && strings.TrimSpace(line) != "" {
				infoLines = append(infoLines, strings.TrimSpace(line))
			}
		}
		ncOutput.ExtraInfo = strings.Join(infoLines, "\n")
	}

	if ncOutput.ConnectionSuccess {
		ncOutput.Status = "SUCCESS"
	} else {
		ncOutput.Status = "FAILED"
	}

	return ncOutput
}

// ParseTcpPingOutput 解析TCP Ping命令输出
func ParseTcpPingOutput(output string, tcpPing *testtoolsv1.TcpPing) *TcpPingOutput {
	tcpPingOutput := &TcpPingOutput{
		Host:      tcpPing.Spec.Host,
		Port:      tcpPing.Spec.Port,
		RttValues: []float64{},
	}

	// 解析每行记录的RTT值
	var receivedCount int32 = 0
	var maxSeq int32 = -1

	for _, line := range strings.Split(output, "\n") {
		// 解析实际输出格式: "seq 0: tcp response from 147.75.40.148 [open]  214.942 ms"
		if matches := regexp.MustCompile(`seq\s+(\d+):\s+tcp\s+response\s+from\s+.*\s+([0-9.]+)\s+ms`).FindStringSubmatch(line); len(matches) > 2 {
			// 提取序列号
			seqNum, _ := strconv.ParseInt(matches[1], 10, 32)
			if int32(seqNum) > maxSeq {
				maxSeq = int32(seqNum)
			}

			// 提取RTT值
			if val, err := strconv.ParseFloat(matches[2], 64); err == nil {
				tcpPingOutput.RttValues = append(tcpPingOutput.RttValues, val)
				receivedCount++
			}
		}
	}

	// 设置发送和接收的包数
	// 发送的包数至少是最大序列号+1（因为序列号从0开始）
	if maxSeq >= 0 {
		tcpPingOutput.Transmitted = maxSeq + 1
	} else {
		// 如果无法确定，则使用收到的响应数量
		tcpPingOutput.Transmitted = receivedCount
	}

	tcpPingOutput.Received = receivedCount

	// 计算丢包率
	if tcpPingOutput.Transmitted > 0 {
		tcpPingOutput.PacketLoss = float64(tcpPingOutput.Transmitted-tcpPingOutput.Received) * 100.0 / float64(tcpPingOutput.Transmitted)
	}

	// 计算延迟统计
	if len(tcpPingOutput.RttValues) > 0 {
		// 最小延迟
		minVal := tcpPingOutput.RttValues[0]
		for _, val := range tcpPingOutput.RttValues {
			if val < minVal {
				minVal = val
			}
		}
		tcpPingOutput.MinLatency = minVal

		// 最大延迟
		maxVal := tcpPingOutput.RttValues[0]
		for _, val := range tcpPingOutput.RttValues {
			if val > maxVal {
				maxVal = val
			}
		}
		tcpPingOutput.MaxLatency = maxVal

		// 平均延迟
		var sum float64
		for _, val := range tcpPingOutput.RttValues {
			sum += val
		}
		tcpPingOutput.AvgLatency = sum / float64(len(tcpPingOutput.RttValues))

		// 标准差
		if len(tcpPingOutput.RttValues) > 1 {
			var sumSquares float64
			for _, val := range tcpPingOutput.RttValues {
				diff := val - tcpPingOutput.AvgLatency
				sumSquares += diff * diff
			}
			tcpPingOutput.StdDevLatency = math.Sqrt(sumSquares / float64(len(tcpPingOutput.RttValues)-1))
		}
	}

	// 设置状态字段
	if tcpPingOutput.Received > 0 {
		tcpPingOutput.Status = "SUCCESS"
	} else {
		tcpPingOutput.Status = "FAILED"
	}

	return tcpPingOutput
}

// ParseIperfOutput 解析iperf命令输出
func ParseIperfOutput(output string, iperf *testtoolsv1.Iperf) (*IperfOutput, error) {
	//iperfOutput := &IperfOutput{
	//	Host:            iperf.Spec.Host,
	//	Port:            iperf.Spec.Port,
	//	Protocol:        iperf.Spec.Protocol,
	//	IntervalReports: []IperfIntervalReport{},
	//}

	// 检查输出是否是JSON格式
	if strings.HasPrefix(strings.TrimSpace(output), "{") && strings.Contains(output, "\"start\"") {
		// 解析JSON格式的iperf输出
		return parseIperfJsonOutput(output, iperf)
	}

	// 如果不是JSON格式，解析文本格式输出
	return parseIperfTextOutput(output, iperf)
}

// parseIperfJsonOutput 解析iperf的JSON格式输出
func parseIperfJsonOutput(output string, iperf *testtoolsv1.Iperf) (*IperfOutput, error) {
	var jsonData map[string]interface{}
	iperfOutput := &IperfOutput{
		Host:            iperf.Spec.Host,
		Port:            iperf.Spec.Port,
		Protocol:        iperf.Spec.Protocol,
		IntervalReports: []IperfIntervalReport{},
	}

	err := json.Unmarshal([]byte(output), &jsonData)
	if err != nil {
		return iperfOutput, fmt.Errorf("解析JSON输出失败: %v", err)
	}

	// 获取起始信息
	if start, ok := jsonData["start"].(map[string]interface{}); ok {
		// 获取版本和测试配置信息
		if test, ok := start["test_start"].(map[string]interface{}); ok {
			if protocol, ok := test["protocol"].(string); ok {
				iperfOutput.Protocol = protocol
			}
			if duration, ok := test["duration"].(float64); ok {
				iperfOutput.Duration = duration
			}
		}
	}

	// 获取interval数据
	if intervals, ok := jsonData["intervals"].([]interface{}); ok {
		for _, idata := range intervals {
			if interval, ok := idata.(map[string]interface{}); ok {
				if streams, ok := interval["streams"].([]interface{}); ok && len(streams) > 0 {
					for _, s := range streams {
						if stream, ok := s.(map[string]interface{}); ok {
							ir := IperfIntervalReport{}

							if interval, ok := stream["interval"].(string); ok {
								ir.Interval = interval
							} else if start, ok := stream["start"].(float64); ok {
								if end, ok := stream["end"].(float64); ok {
									ir.Interval = fmt.Sprintf("%.1f-%.1f", start, end)
								}
							}

							// 存储带宽和字节数
							if bytes, ok := stream["bytes"].(float64); ok {
								ir.SentBytes = int64(bytes)
							}
							if bitsPerSecond, ok := stream["bits_per_second"].(float64); ok {
								ir.SendBandwidth = bitsPerSecond
							}

							iperfOutput.IntervalReports = append(iperfOutput.IntervalReports, ir)
						}
					}
				}
			}
		}
	}

	// 获取汇总数据
	if end, ok := jsonData["end"].(map[string]interface{}); ok {
		if sumSent, ok := end["sum_sent"].(map[string]interface{}); ok {
			if bytes, ok := sumSent["bytes"].(float64); ok {
				iperfOutput.SentBytes = int64(bytes)
			}
			if bitsPerSecond, ok := sumSent["bits_per_second"].(float64); ok {
				iperfOutput.SendBandwidth = bitsPerSecond
			}
		}

		if sumReceived, ok := end["sum_received"].(map[string]interface{}); ok {
			if bytes, ok := sumReceived["bytes"].(float64); ok {
				iperfOutput.ReceivedBytes = int64(bytes)
			}
			if bitsPerSecond, ok := sumReceived["bits_per_second"].(float64); ok {
				iperfOutput.ReceiveBandwidth = bitsPerSecond
			}
		}

		// 获取CPU利用率
		if cpuUtil, ok := end["cpu_utilization_percent"].(map[string]interface{}); ok {
			if hostTotal, ok := cpuUtil["host_total"].(float64); ok {
				iperfOutput.CpuUtilization = hostTotal
			}
		}

		// 获取UDP特定的信息
		if streams, ok := end["streams"].([]interface{}); ok && len(streams) > 0 {
			for _, s := range streams {
				if stream, ok := s.(map[string]interface{}); ok {
					if udp, ok := stream["udp"].(map[string]interface{}); ok {
						if lost, ok := udp["lost_packets"].(float64); ok {
							iperfOutput.LostPackets = int32(lost)
						}
						if lostPercent, ok := udp["lost_percent"].(float64); ok {
							iperfOutput.LostPercent = lostPercent
						}
						if jitter, ok := udp["jitter_ms"].(float64); ok {
							iperfOutput.Jitter = jitter
						}
					}

					// 获取TCP特定的信息(重传)
					if retrans, ok := stream["retransmits"].(float64); ok {
						iperfOutput.Retransmits = int32(retrans)
					}
				}
			}
		}
	}
	if iperfOutput.ReceiveBandwidth > 0 {
		iperfOutput.Status = "SUCCESS"
	} else {
		iperfOutput.Status = "FAILED"
	}

	return iperfOutput, nil
}

// parseIperfTextOutput 解析iperf的文本格式输出
func parseIperfTextOutput(output string, iperf *testtoolsv1.Iperf) (*IperfOutput, error) {
	iperfOutput := &IperfOutput{
		Host:            iperf.Spec.Host,
		Port:            iperf.Spec.Port,
		Protocol:        iperf.Spec.Protocol,
		IntervalReports: []IperfIntervalReport{},
	}

	lines := strings.Split(output, "\n")
	var inSummary bool
	var inClientSummary bool

	// 解析每一行输出
	for _, line := range lines {
		// 忽略空行
		if strings.TrimSpace(line) == "" {
			continue
		}

		// 检测客户端/服务器信息
		if strings.Contains(line, "Client connecting to") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				iperfOutput.Host = parts[3]

				// 解析端口
				if len(parts) >= 6 && strings.HasPrefix(parts[5], "port") {
					portStr := strings.TrimPrefix(parts[5], "port")
					if port, err := strconv.ParseInt(portStr, 10, 32); err == nil {
						iperfOutput.Port = int32(port)
					}
				}
			}
		}

		// 检测协议
		if strings.Contains(line, "TCP") {
			iperfOutput.Protocol = "tcp"
		} else if strings.Contains(line, "UDP") {
			iperfOutput.Protocol = "udp"
		}

		// 检查是否是间隔报告行
		if !inSummary && (strings.Contains(line, "sec") && strings.Contains(line, "Bytes") && strings.Contains(line, "bits/sec")) {
			fields := strings.Fields(line)
			if len(fields) >= 8 {
				// 解析间隔
				interval := ""
				if len(fields) >= 3 && strings.Contains(fields[2], "-") {
					interval = fields[2]
				}

				// 解析字节数
				bytesStr := fields[4]
				sentBytes := int64(0)
				if val, err := strconv.ParseFloat(bytesStr, 64); err == nil {
					// 可能需要根据单位进行转换
					unit := fields[5]
					multiplier := 1.0
					if strings.HasPrefix(unit, "K") {
						multiplier = 1000.0
					} else if strings.HasPrefix(unit, "M") {
						multiplier = 1000000.0
					} else if strings.HasPrefix(unit, "G") {
						multiplier = 1000000000.0
					}
					sentBytes = int64(val * multiplier)
				}

				// 解析带宽
				bandwidthStr := fields[6]
				bandwidth := 0.0
				if val, err := strconv.ParseFloat(bandwidthStr, 64); err == nil {
					// 可能需要根据单位进行转换
					unit := fields[7]
					multiplier := 1.0
					if strings.HasPrefix(unit, "K") {
						multiplier = 1000.0
					} else if strings.HasPrefix(unit, "M") {
						multiplier = 1000000.0
					} else if strings.HasPrefix(unit, "G") {
						multiplier = 1000000000.0
					}
					bandwidth = val * multiplier
				}

				// 添加间隔报告
				iperfOutput.IntervalReports = append(iperfOutput.IntervalReports, IperfIntervalReport{
					Interval:      interval,
					SentBytes:     sentBytes,
					SendBandwidth: bandwidth,
				})

				// 累计字节数
				iperfOutput.SentBytes += sentBytes
			}
		}

		// 检查是否进入总结部分
		if strings.Contains(line, "[ ID]") && strings.Contains(line, "Transfer") && strings.Contains(line, "Bandwidth") {
			inSummary = true
			continue
		}

		// 解析总结部分
		if inSummary && strings.Contains(line, "[SUM]") {
			fields := strings.Fields(line)
			if len(fields) >= 8 {
				// 解析总传输字节数
				if sentBytesStr := fields[4]; sentBytesStr != "" {
					if val, err := strconv.ParseFloat(sentBytesStr, 64); err == nil {
						// 可能需要根据单位进行转换
						unit := fields[5]
						multiplier := 1.0
						if strings.HasPrefix(unit, "K") {
							multiplier = 1000.0
						} else if strings.HasPrefix(unit, "M") {
							multiplier = 1000000.0
						} else if strings.HasPrefix(unit, "G") {
							multiplier = 1000000000.0
						}
						iperfOutput.SentBytes = int64(val * multiplier)
					}
				}

				// 解析总带宽
				if bandwidthStr := fields[6]; bandwidthStr != "" {
					if val, err := strconv.ParseFloat(bandwidthStr, 64); err == nil {
						// 可能需要根据单位进行转换
						unit := fields[7]
						multiplier := 1.0
						if strings.HasPrefix(unit, "K") {
							multiplier = 1000.0
						} else if strings.HasPrefix(unit, "M") {
							multiplier = 1000000.0
						} else if strings.HasPrefix(unit, "G") {
							multiplier = 1000000000.0
						}
						iperfOutput.SendBandwidth = val * multiplier
					}
				}
			}
		}

		// 解析客户端摘要（UDP特有）
		if strings.Contains(line, "Client Report:") {
			inClientSummary = true
			continue
		}

		// 解析UDP丢包信息
		if inClientSummary && strings.Contains(line, "Lost/Total Datagrams") {
			parts := strings.Split(line, "Lost/Total Datagrams")
			if len(parts) >= 2 {
				datagramInfo := strings.TrimSpace(parts[1])
				dataParts := strings.Split(datagramInfo, "/")
				if len(dataParts) >= 2 {
					// 解析丢失的数据包
					if lost, err := strconv.ParseInt(strings.TrimSpace(dataParts[0]), 10, 32); err == nil {
						iperfOutput.LostPackets = int32(lost)
					}

					// 计算丢包率
					if total, err := strconv.ParseInt(strings.TrimSpace(dataParts[1]), 10, 32); err == nil && total > 0 {
						iperfOutput.LostPercent = float64(iperfOutput.LostPackets) * 100.0 / float64(total)
					}
				}
			}
		}

		// 解析抖动信息
		if strings.Contains(line, "Jitter") {
			parts := strings.Split(line, "Jitter")
			if len(parts) >= 2 {
				jitterInfo := strings.TrimSpace(parts[1])
				jitterParts := strings.Fields(jitterInfo)
				if len(jitterParts) >= 1 {
					if jitter, err := strconv.ParseFloat(jitterParts[0], 64); err == nil {
						iperfOutput.Jitter = jitter
					}
				}
			}
		}

		// 提取测试时长
		if strings.Contains(line, "Duration") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				durationStr := strings.TrimSpace(parts[1])
				if duration, err := strconv.ParseFloat(durationStr, 64); err == nil {
					iperfOutput.Duration = duration
				}
			}
		}
	}

	if iperfOutput.ReceiveBandwidth > 0 {
		iperfOutput.Status = "SUCCESS"
	} else {
		iperfOutput.Status = "FAILED"
	}

	return iperfOutput, nil
}

// PrepareNcJob 准备执行NC测试的Job
func PrepareNcJob(ctx context.Context, k8sClient client.Client, nc *testtoolsv1.Nc) (string, error) {
	logger := log.FromContext(ctx)

	// 构建NC命令参数
	args := BuildNCArgs(nc)

	// 创建带有标签的Job
	labels := map[string]string{
		"app":                          "testtools",
		"type":                         "nc",
		"test-name":                    nc.Name,
		"app.kubernetes.io/created-by": "testtools-controller",
		"app.kubernetes.io/managed-by": "nc-controller",
		"app.kubernetes.io/name":       "nc-job",
		"app.kubernetes.io/instance":   nc.Name,
		"app.kubernetes.io/part-of":    "testtools",
		"app.kubernetes.io/component":  "job",
	}

	// 使用固定格式的job名称，避免创建多个job
	baseName := strings.TrimSuffix(nc.Name, "-job") // 移除可能存在的-job后缀
	jobName := fmt.Sprintf("nc-%s-job", baseName)
	if nc.Spec.Schedule != "" {
		jobName = fmt.Sprintf("%s-%s-job", baseName, GenerateShortUID())
	}

	// 检查Job是否已存在
	var existingJob batchv1.Job
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: nc.Namespace,
		Name:      jobName,
	}, &existingJob)

	if err == nil {
		// Job已存在，检查其状态
		if existingJob.Status.Succeeded > 0 {
			// Job已成功完成，可以删除并创建新Job
			logger.Info("已存在已完成的NC Job",
				"job", jobName,
				"namespace", nc.Namespace)
			//if err := k8sClient.Delete(ctx, &existingJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			//	logger.Error(err, "删除已完成的Dig Job失败")
			//	return "", err
			//}
			// 等待Job被删除
			//time.Sleep(2 * time.Second)
			return jobName, nil
		} else if existingJob.Status.Failed > 0 {
			// Job已失败，可以删除并创建新Job
			logger.Info("已存在失败的NC Job",
				"job", jobName,
				"namespace", nc.Namespace)
			//if err := k8sClient.Delete(ctx, &existingJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			//	logger.Error(err, "删除失败的Dig Job失败")
			//	return "", err
			//}
			// 等待Job被删除
			//time.Sleep(2 * time.Second)
			return jobName, nil
		} else {
			// Job正在运行，直接返回Job名称
			logger.Info("NC Job正在运行中，不需要重新创建",
				"job", jobName,
				"namespace", nc.Namespace)
			return jobName, nil
		}
	} else if !errors.IsNotFound(err) {
		// 发生了除"未找到"之外的错误
		logger.Error(err, "检查NC Job是否存在时出错")
		return "", err
	}

	// 创建新Job - 传递作业名称时确保没有重复的"-job"后缀
	job, err := CreateJobForCommand(ctx, k8sClient, nc.Namespace, jobName, "nc", args, labels, nc.Spec.Image, nc.Spec.NodeName, nc.Spec.NodeSelector)
	if err != nil {
		logger.Error(err, "创建NC Job失败")
		return "", err
	}

	// 设置所有者引用
	job.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: nc.APIVersion,
			Kind:       nc.Kind,
			Name:       nc.Name,
			UID:        nc.UID,
			Controller: pointer.Bool(true),
		},
	}

	// 更新Job
	if err := k8sClient.Update(ctx, job); err != nil {
		logger.Error(err, "更新NC Job的所有者引用失败")
		return "", err
	}

	return jobName, nil
}

// PrepareTcpPingJob 准备执行TcpPing测试的Job
func PrepareTcpPingJob(ctx context.Context, k8sClient client.Client, tcpping *testtoolsv1.TcpPing) (string, error) {
	logger := log.FromContext(ctx)

	// 构建TcpPing命令参数
	args := BuildTcpPingArgs(tcpping)

	// 创建带有标签的Job
	labels := map[string]string{
		"app":                          "testtools",
		"type":                         "tcpping",
		"test-name":                    tcpping.Name,
		"app.kubernetes.io/created-by": "testtools-controller",
		"app.kubernetes.io/managed-by": "tcpping-controller",
		"app.kubernetes.io/name":       "tcpping-job",
		"app.kubernetes.io/instance":   tcpping.Name,
		"app.kubernetes.io/part-of":    "testtools",
		"app.kubernetes.io/component":  "job",
	}

	// 使用固定格式的job名称，避免创建多个job
	baseName := strings.TrimSuffix(tcpping.Name, "-job") // 移除可能存在的-job后缀
	jobName := fmt.Sprintf("tcpping-%s-job", baseName)
	if tcpping.Spec.Schedule != "" {
		jobName = fmt.Sprintf("%s-%s-job", baseName, GenerateShortUID())
	}

	// 检查Job是否已存在
	var existingJob batchv1.Job
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: tcpping.Namespace,
		Name:      jobName,
	}, &existingJob)

	if err == nil {
		// Job已存在，检查其状态
		if existingJob.Status.Succeeded > 0 {
			// Job已成功完成，可以删除并创建新Job
			logger.Info("已存在已完成的TcpPing Job",
				"job", jobName,
				"namespace", tcpping.Namespace)
			//if err := k8sClient.Delete(ctx, &existingJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			//	logger.Error(err, "删除已完成的Dig Job失败")
			//	return "", err
			//}
			// 等待Job被删除
			//time.Sleep(2 * time.Second)
			return jobName, nil
		} else if existingJob.Status.Failed > 0 {
			// Job已失败，可以删除并创建新Job
			logger.Info("已存在失败的TcpPing Job",
				"job", jobName,
				"namespace", tcpping.Namespace)
			//if err := k8sClient.Delete(ctx, &existingJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			//	logger.Error(err, "删除失败的Dig Job失败")
			//	return "", err
			//}
			// 等待Job被删除
			//time.Sleep(2 * time.Second)
			return jobName, nil
		} else {
			// Job正在运行，直接返回Job名称
			logger.Info("TcpPing Job正在运行中，不需要重新创建",
				"job", jobName,
				"namespace", tcpping.Namespace)
			return jobName, nil
		}
	} else if !errors.IsNotFound(err) {
		// 发生了除"未找到"之外的错误
		logger.Error(err, "检查TcpPing Job是否存在时出错")
		return "", err
	}

	// 创建新Job - 传递作业名称时确保没有重复的"-job"后缀
	job, err := CreateJobForCommand(ctx, k8sClient, tcpping.Namespace, jobName, "tcpping", args, labels, tcpping.Spec.Image, tcpping.Spec.NodeName, tcpping.Spec.NodeSelector)
	if err != nil {
		logger.Error(err, "创建TcpPing Job失败")
		return "", err
	}

	// 设置所有者引用
	job.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: tcpping.APIVersion,
			Kind:       tcpping.Kind,
			Name:       tcpping.Name,
			UID:        tcpping.UID,
			Controller: pointer.Bool(true),
		},
	}

	// 更新Job
	if err := k8sClient.Update(ctx, job); err != nil {
		logger.Error(err, "更新TcpPing Job的所有者引用失败")
		return "", err
	}

	return jobName, nil
}

// PrepareSkoopJob 准备执行Skoop测试的Job
func PrepareSkoopJob(ctx context.Context, k8sClient client.Client, skoop *testtoolsv1.Skoop) (string, error) {
	logger := log.FromContext(ctx)

	// 构建Skoop命令参数
	args := BuildSkoopArgs(skoop)

	// 创建带有标签的Job
	labels := map[string]string{
		"app":                          "testtools",
		"type":                         "skoop",
		"test-name":                    skoop.Name,
		"app.kubernetes.io/created-by": "testtools-controller",
		"app.kubernetes.io/managed-by": "skoop-controller",
		"app.kubernetes.io/name":       "skoop-job",
		"app.kubernetes.io/instance":   skoop.Name,
		"app.kubernetes.io/part-of":    "testtools",
		"app.kubernetes.io/component":  "job",
	}

	// 使用固定格式的job名称，避免创建多个job
	baseName := strings.TrimSuffix(skoop.Name, "-job") // 移除可能存在的-job后缀
	jobName := fmt.Sprintf("skoop-%s-job", baseName)
	if skoop.Spec.Schedule != "" {
		jobName = fmt.Sprintf("%s-%s-job", baseName, GenerateShortUID())
	}

	// 检查Job是否已存在
	var existingJob batchv1.Job
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: skoop.Namespace,
		Name:      jobName,
	}, &existingJob)

	if err == nil {
		// Job已存在，检查其状态
		if existingJob.Status.Succeeded > 0 {
			// Job已成功完成，可以删除并创建新Job
			logger.Info("已存在已完成的Skoop Job",
				"job", jobName,
				"namespace", skoop.Namespace)
			//if err := k8sClient.Delete(ctx, &existingJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			//	logger.Error(err, "删除已完成的Dig Job失败")
			//	return "", err
			//}
			// 等待Job被删除
			//time.Sleep(2 * time.Second)
			return jobName, nil
		} else if existingJob.Status.Failed > 0 {
			// Job已失败，可以删除并创建新Job
			logger.Info("已存在失败的SkoopJob",
				"job", jobName,
				"namespace", skoop.Namespace)
			//if err := k8sClient.Delete(ctx, &existingJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			//	logger.Error(err, "删除失败的Dig Job失败")
			//	return "", err
			//}
			// 等待Job被删除
			//time.Sleep(2 * time.Second)
			return jobName, nil
		} else {
			// Job正在运行，直接返回Job名称
			logger.Info("Skoop Job正在运行中，不需要重新创建",
				"job", jobName,
				"namespace", skoop.Namespace)
			return jobName, nil
		}
	} else if !errors.IsNotFound(err) {
		// 发生了除"未找到"之外的错误
		logger.Error(err, "检查TcpPing Job是否存在时出错")
		return "", err
	}

	// 创建新Job - 传递作业名称时确保没有重复的"-job"后缀
	job, err := CreateJobForCommand(ctx, k8sClient, skoop.Namespace, jobName, "skoop", args, labels, skoop.Spec.Image, skoop.Spec.NodeName, skoop.Spec.NodeSelector)
	if err != nil {
		logger.Error(err, "创建Skoop Job失败")
		return "", err
	}

	// 设置所有者引用
	job.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: skoop.APIVersion,
			Kind:       skoop.Kind,
			Name:       skoop.Name,
			UID:        skoop.UID,
			Controller: pointer.Bool(true),
		},
	}

	// 更新Job
	if err := k8sClient.Update(ctx, job); err != nil {
		logger.Error(err, "更新Skoop Job的所有者引用失败")
		return "", err
	}

	return jobName, nil
}

// ParseSkoopOutput 解析skoop命令输出
func ParseSkoopOutput(output string, skoop *testtoolsv1.Skoop) (*SkoopOutput, error) {
	skoopOutput := &SkoopOutput{
		SourceAddress:      skoop.Spec.Task.SourceAddress,
		DestinationAddress: skoop.Spec.Task.DestinationAddress,
		DestinationPort:    skoop.Spec.Task.DestinationPort,
		Protocol:           skoop.Spec.Task.Protocol,
		Path:               []SkoopNode{},
		Issues:             []SkoopIssue{},
	}

	// 检查输出是否是JSON格式
	if strings.HasPrefix(strings.TrimSpace(output), "{") {
		// 解析JSON格式输出
		var jsonData map[string]interface{}
		err := json.Unmarshal([]byte(output), &jsonData)
		if err != nil {
			return skoopOutput, fmt.Errorf("解析JSON输出失败: %v", err)
		}

		// 获取诊断状态
		if status, ok := jsonData["status"].(string); ok {
			skoopOutput.Status = status
		}

		// 获取诊断概要
		if summary, ok := jsonData["summary"].(string); ok {
			skoopOutput.Summary = summary
		}

		// 获取诊断路径
		if path, ok := jsonData["path"].([]interface{}); ok {
			for _, node := range path {
				if nodeMap, ok := node.(map[string]interface{}); ok {
					skoopNode := SkoopNode{}

					if nodeType, ok := nodeMap["type"].(string); ok {
						skoopNode.Type = nodeType
					}
					if name, ok := nodeMap["name"].(string); ok {
						skoopNode.Name = name
					}
					if ip, ok := nodeMap["ip"].(string); ok {
						skoopNode.IP = ip
					}
					if namespace, ok := nodeMap["namespace"].(string); ok {
						skoopNode.Namespace = namespace
					}
					if nodeName, ok := nodeMap["nodeName"].(string); ok {
						skoopNode.NodeName = nodeName
					}
					if iface, ok := nodeMap["interface"].(string); ok {
						skoopNode.Interface = iface
					}
					if protocol, ok := nodeMap["protocol"].(string); ok {
						skoopNode.Protocol = protocol
					}
					if latency, ok := nodeMap["latencyMs"].(float64); ok {
						skoopNode.LatencyMs = latency
					}

					skoopOutput.Path = append(skoopOutput.Path, skoopNode)
				}
			}
		}

		// 获取诊断问题
		if issues, ok := jsonData["issues"].([]interface{}); ok {
			for _, issue := range issues {
				if issueMap, ok := issue.(map[string]interface{}); ok {
					skoopIssue := SkoopIssue{}

					if issueType, ok := issueMap["type"].(string); ok {
						skoopIssue.Type = issueType
					}
					if level, ok := issueMap["level"].(string); ok {
						skoopIssue.Level = level
					}
					if message, ok := issueMap["message"].(string); ok {
						skoopIssue.Message = message
					}
					if location, ok := issueMap["location"].(string); ok {
						skoopIssue.Location = location
					}
					if solution, ok := issueMap["solution"].(string); ok {
						skoopIssue.Solution = solution
					}

					skoopOutput.Issues = append(skoopOutput.Issues, skoopIssue)
				}
			}
		}

		return skoopOutput, nil
	}

	// 解析文本格式输出
	lines := strings.Split(output, "\n")
	var inPathSection, inIssuesSection bool

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		// 提取诊断状态
		if strings.Contains(line, "Diagnosis Status:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) > 1 {
				skoopOutput.Status = strings.TrimSpace(parts[1])
			}
			continue
		}

		// 提取诊断概要
		if strings.Contains(line, "Summary:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) > 1 {
				skoopOutput.Summary = strings.TrimSpace(parts[1])
			}
			continue
		}

		// 检查路径部分开始
		if strings.Contains(line, "Network Path:") || strings.Contains(line, "Connection Path:") {
			inPathSection = true
			inIssuesSection = false
			continue
		}

		// 检查问题部分开始
		if strings.Contains(line, "Issues:") || strings.Contains(line, "Detected Issues:") {
			inPathSection = false
			inIssuesSection = true
			continue
		}

		// 解析路径节点
		if inPathSection && !strings.HasPrefix(trimmedLine, "---") {
			fields := strings.Fields(trimmedLine)
			if len(fields) >= 3 { // 至少需要类型、名称和IP
				node := SkoopNode{
					Type: fields[0],
					Name: fields[1],
				}

				// 尝试提取IP地址
				ipRegex := regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
				if ipMatches := ipRegex.FindAllString(trimmedLine, -1); len(ipMatches) > 0 {
					node.IP = ipMatches[0]
				}

				// 尝试提取命名空间
				if strings.Contains(trimmedLine, "namespace:") {
					parts := strings.Split(trimmedLine, "namespace:")
					if len(parts) > 1 {
						namespaceParts := strings.Fields(parts[1])
						if len(namespaceParts) > 0 {
							node.Namespace = namespaceParts[0]
						}
					}
				}

				// 尝试提取节点名称
				if strings.Contains(trimmedLine, "node:") {
					parts := strings.Split(trimmedLine, "node:")
					if len(parts) > 1 {
						nodeParts := strings.Fields(parts[1])
						if len(nodeParts) > 0 {
							node.NodeName = nodeParts[0]
						}
					}
				}

				// 尝试提取接口信息
				if strings.Contains(trimmedLine, "interface:") {
					parts := strings.Split(trimmedLine, "interface:")
					if len(parts) > 1 {
						ifaceParts := strings.Fields(parts[1])
						if len(ifaceParts) > 0 {
							node.Interface = ifaceParts[0]
						}
					}
				}

				// 尝试提取延迟信息
				if strings.Contains(trimmedLine, "latency:") {
					parts := strings.Split(trimmedLine, "latency:")
					if len(parts) > 1 {
						latencyParts := strings.Fields(parts[1])
						if len(latencyParts) > 0 {
							latencyStr := strings.TrimSuffix(latencyParts[0], "ms")
							if latency, err := strconv.ParseFloat(latencyStr, 64); err == nil {
								node.LatencyMs = latency
							}
						}
					}
				}

				skoopOutput.Path = append(skoopOutput.Path, node)
			}
			continue
		}

		// 解析问题
		if inIssuesSection && !strings.HasPrefix(trimmedLine, "---") {
			// 尝试解析问题格式，例如 "[WARNING] Pod network policy blocks traffic: in namespace default"
			levelRegex := regexp.MustCompile(`\[(WARNING|ERROR|INFO)\]`)
			if levelMatches := levelRegex.FindStringSubmatch(trimmedLine); len(levelMatches) > 1 {
				issue := SkoopIssue{
					Level: levelMatches[1],
				}

				// 提取问题描述
				messageParts := strings.SplitN(trimmedLine, "]", 2)
				if len(messageParts) > 1 {
					issue.Message = strings.TrimSpace(messageParts[1])
				}

				// 尝试根据消息内容推断问题类型
				if strings.Contains(issue.Message, "network policy") {
					issue.Type = "NetworkPolicy"
				} else if strings.Contains(issue.Message, "firewall") {
					issue.Type = "Firewall"
				} else if strings.Contains(issue.Message, "DNS") {
					issue.Type = "DNS"
				} else if strings.Contains(issue.Message, "route") {
					issue.Type = "Routing"
				} else if strings.Contains(issue.Message, "MTU") {
					issue.Type = "MTU"
				} else {
					issue.Type = "Other"
				}

				// 尝试提取位置信息
				if strings.Contains(issue.Message, "in namespace") {
					locationParts := strings.Split(issue.Message, "in namespace")
					if len(locationParts) > 1 {
						issue.Location = "namespace:" + strings.TrimSpace(locationParts[1])
					}
				} else if strings.Contains(issue.Message, "on node") {
					locationParts := strings.Split(issue.Message, "on node")
					if len(locationParts) > 1 {
						issue.Location = "node:" + strings.TrimSpace(locationParts[1])
					}
				}

				skoopOutput.Issues = append(skoopOutput.Issues, issue)
			}
			continue
		}
	}

	return skoopOutput, nil
}

// PrepareIperfServerJob
func PrepareIperfServerJob(ctx context.Context, k8sClient client.Client, iperf *testtoolsv1.Iperf) (string, error) {
	logger := log.FromContext(ctx)
	logger.Info("Prepare iperf server Job", "namespace", iperf.Namespace, "name", iperf.Name)

	// 构建Skoop命令参数
	args := BuildIperfServerArgs(iperf)

	// 创建带有标签的Job
	labels := map[string]string{
		"app":                          "testtools",
		"type":                         "iperf",
		"test-name":                    iperf.Name,
		"app.kubernetes.io/created-by": "testtools-controller",
		"app.kubernetes.io/managed-by": "iperf-controller",
		"app.kubernetes.io/name":       "iperf-job",
		"app.kubernetes.io/instance":   iperf.Name,
		"app.kubernetes.io/part-of":    "testtools",
		"app.kubernetes.io/component":  "job",
	}

	// 使用固定格式的job名称，避免创建多个job
	baseName := strings.TrimSuffix(iperf.Name, "-job")
	jobName := fmt.Sprintf("iperf-server-%s-job", baseName)
	if iperf.Spec.Schedule != "" {
		jobName = fmt.Sprintf("iperf-server-%s-%s-job", baseName, GenerateShortUID())
	}

	// 检查Job是否已存在
	var existingJob batchv1.Job
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: iperf.Namespace,
		Name:      jobName,
	}, &existingJob)

	if err == nil {
		// Job已存在，检查其状态
		if existingJob.Status.Succeeded > 0 {
			logger.Info("已存在已完成的Iperf Server Job",
				"job", jobName,
				"namespace", iperf.Namespace)
			return jobName, nil
		} else if existingJob.Status.Failed > 0 {
			// Job已失败，可以删除并创建新Job
			logger.Info("已存在失败的Iperf Server Job",
				"job", jobName,
				"namespace", iperf.Namespace)
			return jobName, nil
		} else {
			// Job正在运行，直接返回Job名称
			logger.Info("Iperf Server Job正在运行中，不需要重新创建",
				"job", jobName,
				"namespace", iperf.Namespace)
			return jobName, nil
		}
	} else if !errors.IsNotFound(err) {
		logger.Error(err, "检查Iperf Server Job是否存在时出错")
		return "", err
	}

	// 创建新Job - 传递作业名称时确保没有重复的"-job"后缀
	job, err := CreateJobForCommand(ctx, k8sClient, iperf.Namespace, jobName, "iperf3", args, labels, iperf.Spec.Image, iperf.Spec.NodeName, iperf.Spec.NodeSelector)
	if err != nil {
		logger.Error(err, "创建Iperf Server Job失败")
		return "", err
	}

	// 设置所有者引用
	job.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: iperf.APIVersion,
			Kind:       iperf.Kind,
			Name:       iperf.Name,
			UID:        iperf.UID,
			Controller: pointer.Bool(true),
		},
	}

	// 更新Job
	if err := k8sClient.Update(ctx, job); err != nil {
		logger.Error(err, "更新Iperf Server Job所有者引用失败")
		return "", err
	}

	logger.Info("成功创建Iperf Server Job", "name", jobName)
	return jobName, nil
}

// PrepareIperfClientJob 准备执行Iperf客户端Job
func PrepareIperfClientJob(ctx context.Context, k8sClient client.Client, iperf *testtoolsv1.Iperf) (string, error) {
	logger := log.FromContext(ctx)
	logger.Info("Prepare iperf client Job", "namespace", iperf.Namespace, "name", iperf.Name)

	// 构建Skoop命令参数
	args := BuildIperfClientArgs(iperf)

	// 创建带有标签的Job
	labels := map[string]string{
		"app":                          "testtools",
		"type":                         "iperf",
		"test-name":                    iperf.Name,
		"app.kubernetes.io/created-by": "testtools-controller",
		"app.kubernetes.io/managed-by": "iperf-controller",
		"app.kubernetes.io/name":       "iperf-job",
		"app.kubernetes.io/instance":   iperf.Name,
		"app.kubernetes.io/part-of":    "testtools",
		"app.kubernetes.io/component":  "job",
	}

	// 使用固定格式的job名称，避免创建多个job
	baseName := strings.TrimSuffix(iperf.Name, "-job")
	jobName := fmt.Sprintf("iperf-client-%s-job", baseName)
	if iperf.Spec.Schedule != "" {
		jobName = fmt.Sprintf("iperf-client-%s-%s-job", baseName, GenerateShortUID())
	}

	// 检查Job是否已存在
	var existingJob batchv1.Job
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: iperf.Namespace,
		Name:      jobName,
	}, &existingJob)

	if err == nil {
		// Job已存在，检查其状态
		if existingJob.Status.Succeeded > 0 {
			logger.Info("已存在已完成的Iperf Client Job",
				"job", jobName,
				"namespace", iperf.Namespace)
			return jobName, nil
		} else if existingJob.Status.Failed > 0 {
			// Job已失败，可以删除并创建新Job
			logger.Info("已存在失败的Iperf Client Job",
				"job", jobName,
				"namespace", iperf.Namespace)
			return jobName, nil
		} else {
			// Job正在运行，直接返回Job名称
			logger.Info("Iperf Client Job正在运行中，不需要重新创建",
				"job", jobName,
				"namespace", iperf.Namespace)
			return jobName, nil
		}
	} else if !errors.IsNotFound(err) {
		logger.Error(err, "检查Iperf Client Job是否存在时出错")
		return "", err
	}

	// 创建新Job - 传递作业名称时确保没有重复的"-job"后缀
	job, err := CreateJobForCommand(ctx, k8sClient, iperf.Namespace, jobName, "iperf3", args, labels, iperf.Spec.Image, iperf.Spec.NodeName, iperf.Spec.NodeSelector)
	if err != nil {
		logger.Error(err, "创建Iperf Client Job失败")
		return "", err
	}

	// 设置所有者引用
	job.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: iperf.APIVersion,
			Kind:       iperf.Kind,
			Name:       iperf.Name,
			UID:        iperf.UID,
			Controller: pointer.Bool(true),
		},
	}

	// 更新Job
	if err := k8sClient.Update(ctx, job); err != nil {
		logger.Error(err, "更新Iperf Client Job所有者引用失败")
		return "", err
	}

	logger.Info("成功创建Iperf Client Job", "name", jobName)
	return jobName, nil
}
