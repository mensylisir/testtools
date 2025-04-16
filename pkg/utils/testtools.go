package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
}

// PrepareFioJob 为 FIO 测试创建一个 Kubernetes Job
func PrepareFioJob(ctx context.Context, c client.Client, fio *testtoolsv1.Fio) (string, error) {
	logger := log.FromContext(ctx)

	// 修复: 避免重复的"-job"后缀
	// 使用不带后缀的基本名称
	baseName := fmt.Sprintf("fio-%s", fio.Name)
	// 完整的作业名称
	jobName := fmt.Sprintf("%s-job", baseName)

	// 检查是否已存在同名作业
	existingJob := &batchv1.Job{}
	err := c.Get(ctx, types.NamespacedName{Namespace: fio.Namespace, Name: jobName}, existingJob)
	if err == nil {
		// 已存在同名作业，需要处理冲突
		logger.Info("发现已存在的FIO作业", "jobName", jobName, "status", existingJob.Status)

		// 检查作业状态
		var completed bool
		var failed bool

		for _, condition := range existingJob.Status.Conditions {
			if condition.Type == batchv1.JobComplete && condition.Status == v1.ConditionTrue {
				completed = true
			}
			if condition.Type == batchv1.JobFailed && condition.Status == v1.ConditionTrue {
				failed = true
			}
		}

		// 如果作业已完成或失败，尝试删除它
		if completed || failed {
			logger.Info("作业已经完成或失败，将删除它", "jobName", jobName, "completed", completed, "failed", failed)

			// 使用 DeletePropagationForeground 确保作业及其相关资源被完全删除
			deletePolicy := metav1.DeletePropagationForeground
			deleteOptions := &client.DeleteOptions{
				PropagationPolicy: &deletePolicy,
			}

			if err := c.Delete(ctx, existingJob, deleteOptions); err != nil {
				logger.Error(err, "删除已存在的作业失败", "jobName", jobName)
				return "", fmt.Errorf("删除已存在的作业失败: %w", err)
			}

			// 等待作业被删除
			logger.Info("等待作业被删除", "jobName", jobName)
			time.Sleep(5 * time.Second)

			// 再次检查作业是否已被删除
			err = c.Get(ctx, types.NamespacedName{Namespace: fio.Namespace, Name: jobName}, &batchv1.Job{})
			if err == nil {
				// 作业仍然存在，尝试强制删除
				logger.Info("作业仍然存在，尝试强制删除", "jobName", jobName)

				// 强制删除所有相关的 Pod
				podList := &v1.PodList{}
				if err := c.List(ctx, podList, client.InNamespace(fio.Namespace), client.MatchingLabels{"job-name": jobName}); err != nil {
					logger.Error(err, "获取作业相关的 Pod 列表失败")
				} else {
					for _, pod := range podList.Items {
						logger.Info("强制删除 Pod", "podName", pod.Name)
						var zeroGracePeriod int64 = 0
						if err := c.Delete(ctx, &pod, &client.DeleteOptions{
							GracePeriodSeconds: &zeroGracePeriod,
							PropagationPolicy:  &deletePolicy,
						}); err != nil {
							logger.Error(err, "强制删除 Pod 失败", "podName", pod.Name)
						}
					}
				}

				// 再次尝试删除作业
				var zeroGracePeriod int64 = 0
				if err := c.Delete(ctx, existingJob, &client.DeleteOptions{
					GracePeriodSeconds: &zeroGracePeriod,
					PropagationPolicy:  &deletePolicy,
				}); err != nil {
					logger.Error(err, "强制删除作业失败", "jobName", jobName)
					return "", fmt.Errorf("强制删除作业失败: %w", err)
				}

				// 再次等待
				logger.Info("等待强制删除完成", "jobName", jobName)
				time.Sleep(5 * time.Second)
			}
		} else {
			// 作业仍在运行中，返回作业名称
			logger.Info("FIO作业仍在运行中", "jobName", jobName)
			return jobName, nil
		}
	} else if !errors.IsNotFound(err) {
		// 获取作业时发生非 NotFound 错误
		logger.Error(err, "检查已存在的作业时出错")
		return "", fmt.Errorf("检查已存在的作业时出错: %w", err)
	}

	// 获取参数并构建 FIO 命令
	args := BuildFioArgs(fio)

	// 设置作业标签
	labels := map[string]string{
		"app":              "fio",
		"test-resource":    fio.Name,
		"test-namespace":   fio.Namespace,
		"test-type":        "fio",
		"test-controller":  "true",
		"test-report-name": fmt.Sprintf("fio-%s-report", fio.Name),
	}

	// 使用通用函数创建作业 - 传递作业名称时确保没有重复的"-job"后缀
	_, err = CreateJobForCommand(ctx, c, fio.Namespace, jobName, "fio", args, labels, fio.Spec.Image)
	if err != nil {
		logger.Error(err, "创建FIO作业失败")
		return "", err
	}

	// 短暂延迟确保Job被创建
	time.Sleep(2 * time.Second)

	// 验证Job确实已经创建
	var newJob batchv1.Job
	err = c.Get(ctx, types.NamespacedName{Namespace: fio.Namespace, Name: jobName}, &newJob)
	if err != nil {
		logger.Error(err, "验证新创建的Job失败")
		return "", fmt.Errorf("创建FIO作业后验证失败: %w", err)
	}

	logger.Info("成功创建FIO作业", "jobName", jobName, "args", args)
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
func ParseFioOutput(outputStr string) (testtoolsv1.FioStats, error) {
	stats := testtoolsv1.FioStats{
		LatencyPercentiles: make(map[string]float64),
	}

	// 记录原始输出长度，便于调试
	outputLength := len(outputStr)
	if outputLength == 0 {
		return stats, fmt.Errorf("FIO输出为空")
	}

	// 尝试定位JSON部分
	// 找到第一个左大括号
	jsonStart := strings.Index(outputStr, "{")
	if jsonStart < 0 {
		return stats, fmt.Errorf("FIO输出中未找到JSON开始标记 '{': %s", truncateString(outputStr, 200))
	}

	// 找到最后一个右大括号
	jsonEnd := strings.LastIndex(outputStr, "}")
	if jsonEnd < 0 {
		return stats, fmt.Errorf("FIO输出中未找到JSON结束标记 '}': %s", truncateString(outputStr, 200))
	}

	if jsonEnd <= jsonStart {
		return stats, fmt.Errorf("FIO输出中JSON格式无效，结束标记位于开始标记之前: %s", truncateString(outputStr, 200))
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
			return stats, fmt.Errorf("无法解析FIO输出: %v, 提取的JSON字符串: %s", err, truncateString(jsonStr, 200))
		}
	}

	// 提取性能指标
	if len(fioOutput.Jobs) == 0 {
		return stats, fmt.Errorf("FIO输出中未找到任何作业数据")
	}

	// 使用第一个作业的结果，或者如果使用了group_reporting则是合并的结果
	job := fioOutput.Jobs[0]

	// 读取IOPS和带宽
	stats.ReadIOPS = job.Read.IOPS
	stats.WriteIOPS = job.Write.IOPS
	stats.ReadBW = job.Read.BW
	stats.WriteBW = job.Write.BW

	// 读取延迟，处理可能的单位差异 (纳秒、微秒)
	readLatency := job.Read.LatencyNs.Mean
	if readLatency == 0 && job.Read.LatencyUsec > 0 {
		readLatency = job.Read.LatencyUsec * 1000 // 转换为纳秒
	}
	if readLatency > 0 {
		stats.ReadLatency = readLatency / 1000000 // 转换为毫秒
	}

	writeLatency := job.Write.LatencyNs.Mean
	if writeLatency == 0 && job.Write.LatencyUsec > 0 {
		writeLatency = job.Write.LatencyUsec * 1000 // 转换为纳秒
	}
	if writeLatency > 0 {
		stats.WriteLatency = writeLatency / 1000000 // 转换为毫秒
	}

	// 提取百分位延迟
	for percentile, value := range job.Read.LatencyNs.Percentile {
		stats.LatencyPercentiles["read_"+percentile] = value / 1000000 // 转换为毫秒
	}
	for percentile, value := range job.Write.LatencyNs.Percentile {
		stats.LatencyPercentiles["write_"+percentile] = value / 1000000 // 转换为毫秒
	}

	return stats, nil
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
		"app":       "testtools",
		"type":      "dig",
		"test-name": dig.Name,
	}

	// 修复：避免重复的"-job"后缀
	// 使用不带后缀的基本名称
	baseName := fmt.Sprintf("dig-%s", dig.Name)
	// 完整的作业名称
	jobName := fmt.Sprintf("%s-job", baseName)

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
			logger.Info("已存在已完成的Dig Job，将删除后重新创建",
				"job", jobName,
				"namespace", dig.Namespace)
			if err := k8sClient.Delete(ctx, &existingJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				logger.Error(err, "删除已完成的Dig Job失败")
				return "", err
			}
			// 等待Job被删除
			time.Sleep(2 * time.Second)
		} else if existingJob.Status.Failed > 0 {
			// Job已失败，可以删除并创建新Job
			logger.Info("已存在失败的Dig Job，将删除后重新创建",
				"job", jobName,
				"namespace", dig.Namespace)
			if err := k8sClient.Delete(ctx, &existingJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				logger.Error(err, "删除失败的Dig Job失败")
				return "", err
			}
			// 等待Job被删除
			time.Sleep(2 * time.Second)
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
	_, err = CreateJobForCommand(ctx, k8sClient, dig.Namespace, jobName, "dig", args, labels, dig.Spec.Image)
	if err != nil {
		logger.Error(err, "创建Dig Job失败")
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

// PreparePingJob 准备执行Ping测试的Job
func PreparePingJob(ctx context.Context, k8sClient client.Client, ping *testtoolsv1.Ping) (string, error) {
	logger := log.FromContext(ctx)

	// 构建Ping命令参数
	args := BuildPingArgs(ping)

	// 创建带有标签的Job
	labels := map[string]string{
		"app":       "testtools",
		"type":      "ping",
		"test-name": ping.Name,
	}

	// 修复：避免重复的"-job"后缀
	// 使用不带后缀的基本名称
	baseName := fmt.Sprintf("ping-%s", ping.Name)
	// 完整的作业名称
	jobName := fmt.Sprintf("%s-job", baseName)

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
			logger.Info("已存在已完成的Ping Job，将删除后重新创建",
				"job", jobName,
				"namespace", ping.Namespace)
			if err := k8sClient.Delete(ctx, &existingJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				logger.Error(err, "删除已完成的Ping Job失败")
				return "", err
			}
			// 等待Job被删除
			time.Sleep(2 * time.Second)
		} else if existingJob.Status.Failed > 0 {
			// Job已失败，可以删除并创建新Job
			logger.Info("已存在失败的Ping Job，将删除后重新创建",
				"job", jobName,
				"namespace", ping.Namespace)
			if err := k8sClient.Delete(ctx, &existingJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				logger.Error(err, "删除失败的Ping Job失败")
				return "", err
			}
			// 等待Job被删除
			time.Sleep(2 * time.Second)
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

	// 创建新Job - 传递作业名称时确保没有重复的"-job"后缀
	_, err = CreateJobForCommand(ctx, k8sClient, ping.Namespace, jobName, "ping", args, labels, ping.Spec.Image)
	if err != nil {
		logger.Error(err, "创建Ping Job失败")
		return "", err
	}

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

	return pingOutput
}

// ParsePingStatistics 解析Ping统计信息
func ParsePingStatistics(output string, pingOutput *PingOutput) {
	stats := ExtractSection(output, "--- ping statistics ---", "")
	if stats == "" {
		return
	}

	lines := strings.Split(stats, "\n")
	for _, line := range lines {
		// 解析发送/接收/丢失的数据包
		if strings.Contains(line, "packets transmitted") {
			fields := strings.Fields(line)
			if len(fields) >= 10 {
				if val, err := strconv.Atoi(fields[0]); err == nil {
					pingOutput.Transmitted = val
				}
				if val, err := strconv.Atoi(fields[3]); err == nil {
					pingOutput.Received = val
				}
				// 解析丢包率
				if strings.Contains(line, "% packet loss") {
					lossStr := ""
					for _, field := range fields {
						if strings.HasSuffix(field, "%") {
							lossStr = strings.TrimSuffix(field, "%")
							break
						}
					}
					if val, err := strconv.ParseFloat(lossStr, 64); err == nil {
						pingOutput.PacketLoss = val
					}
				}
			}
		}

		// 解析延迟统计
		if strings.Contains(line, "min/avg/max") {
			parts := strings.Split(line, "=")
			if len(parts) > 1 {
				stats := strings.Split(strings.TrimSpace(parts[1]), "/")
				if len(stats) >= 4 {
					if val, err := strconv.ParseFloat(stats[0], 64); err == nil {
						pingOutput.MinRtt = val
					}
					if val, err := strconv.ParseFloat(stats[1], 64); err == nil {
						pingOutput.AvgRtt = val
					}
					if val, err := strconv.ParseFloat(stats[2], 64); err == nil {
						pingOutput.MaxRtt = val
					}
					if val, err := strconv.ParseFloat(stats[3], 64); err == nil {
						pingOutput.StdDevRtt = val
					}
				}
			}
		}
	}
}

// ExtractPingStats 从 PingOutput 提取 PingStatus
func ExtractPingStats(output *PingOutput) testtoolsv1.PingStatus {
	status := testtoolsv1.PingStatus{
		PacketLoss: output.PacketLoss,
		MinRtt:     output.MinRtt,
		AvgRtt:     output.AvgRtt,
		MaxRtt:     output.MaxRtt,
	}
	return status
}
