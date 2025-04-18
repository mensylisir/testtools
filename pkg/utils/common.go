package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net"
	"os/exec"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

// SetCondition sets or updates a condition in condition list
func SetCondition(conditions *[]metav1.Condition, condition metav1.Condition) {
	// If conditions is nil, create a new slice
	if *conditions == nil {
		*conditions = []metav1.Condition{}
	}

	// Find the condition
	var existingCondition *metav1.Condition
	for i := range *conditions {
		if (*conditions)[i].Type == condition.Type {
			existingCondition = &(*conditions)[i]
			break
		}
	}

	// If condition doesn't exist, add it
	if existingCondition == nil {
		*conditions = append(*conditions, condition)
		return
	}

	// Update existing condition
	if existingCondition.Status != condition.Status {
		existingCondition.LastTransitionTime = condition.LastTransitionTime
	}
	existingCondition.Status = condition.Status
	existingCondition.Reason = condition.Reason
	existingCondition.Message = condition.Message
}

// ExtractSection extracts content from text between specified markers
func ExtractSection(text, startMarker, endMarker string) string {
	startIndex := 0
	if startMarker != "" {
		startIndex = indexOf(text, startMarker)
		if startIndex == -1 {
			return ""
		}
		startIndex += len(startMarker)
	}

	endIndex := len(text)
	if endMarker != "" {
		tempIndex := indexOf(text[startIndex:], endMarker)
		if tempIndex != -1 {
			endIndex = startIndex + tempIndex
		}
	}

	if startIndex >= endIndex {
		return ""
	}
	return text[startIndex:endIndex]
}

// ExtractMultilineSection extracts a multiline section from text
func ExtractMultilineSection(text, startMarker, nextSectionMarker string) string {
	startIndex := 0
	if startMarker != "" {
		startIndex = indexOf(text, startMarker)
		if startIndex == -1 {
			return ""
		}
		startIndex += len(startMarker)
	}

	endIndex := len(text)
	if nextSectionMarker != "" {
		tempIndex := indexOf(text[startIndex:], nextSectionMarker)
		if tempIndex != -1 {
			endIndex = startIndex + tempIndex
		}
	}

	if startIndex >= endIndex {
		return ""
	}
	return text[startIndex:endIndex]
}

// indexOf is a helper function to find the index of a substring
func indexOf(text, substring string) int {
	for i := 0; i <= len(text)-len(substring); i++ {
		if text[i:i+len(substring)] == substring {
			return i
		}
	}
	return -1
}

// CreateJobForCommand creates a Kubernetes Job to execute a command
func CreateJobForCommand(ctx context.Context, k8sClient client.Client, namespace, name, command string, args []string, labels map[string]string, image string) (*batchv1.Job, error) {
	logger := log.FromContext(ctx)
	logger.Info("Creating job to execute command", "namespace", namespace, "name", name, "command", command, "image", image)

	// 如果没有提供镜像，使用默认镜像
	if image == "" {
		image = "debian:bullseye-slim"
	}

	// 创建基本的job对象
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:            "executor",
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{command},
							Args:            args,
						},
					},
				},
			},
			BackoffLimit: func() *int32 { i := int32(2); return &i }(),
		},
	}

	// 创建job
	if err := k8sClient.Create(ctx, job); err != nil {
		logger.Error(err, "Failed to create job")
		return nil, err
	}

	return job, nil
}

// GetJobResults retrieves the results of a completed job
func GetJobResults(ctx context.Context, k8sClient client.Client, namespace, jobName string) (string, error) {
	logger := log.FromContext(ctx)

	// 获取job
	var job batchv1.Job
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: jobName}, &job); err != nil {
		if errors.IsNotFound(err) {
			logger.Error(err, "获取作业失败 - 作业不存在", "jobName", jobName)
			return "", fmt.Errorf("作业不存在: %s/%s", namespace, jobName)
		}
		logger.Error(err, "获取作业失败", "jobName", jobName)
		return "", err
	}

	// 检查job是否完成
	completed := false
	failed := false
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
			completed = true
			break
		}
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			failed = true
			break
		}
	}

	if !completed && !failed {
		return "", fmt.Errorf("作业尚未完成: %s/%s", namespace, jobName)
	}

	// 获取pod
	var pods corev1.PodList
	if err := k8sClient.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabels(job.Spec.Template.Labels)); err != nil {
		logger.Error(err, "获取作业相关的Pod列表失败", "jobName", jobName)
		return "", err
	}

	if len(pods.Items) == 0 {
		logger.Info("未找到作业相关的Pod", "jobName", jobName, "namespace", namespace)
		return "", fmt.Errorf("未找到作业相关的Pod: %s/%s", namespace, jobName)
	}

	// 创建Kubernetes客户端来获取Pod日志
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Error(err, "获取集群配置失败")
		return "", fmt.Errorf("获取集群配置失败: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Error(err, "创建Kubernetes客户端失败")
		return "", fmt.Errorf("创建Kubernetes客户端失败: %v", err)
	}

	// 尝试从所有Pod获取日志
	var combinedLogs strings.Builder
	logsFound := false

	for i, pod := range pods.Items {
		podName := pod.Name
		logger.Info("尝试获取Pod日志", "podName", podName, "index", i+1, "total", len(pods.Items))

		// 创建获取日志的请求
		req := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{})
		podLogs, err := req.Stream(ctx)
		if err != nil {
			logger.Error(err, "获取Pod日志流失败", "podName", podName)
			// 继续尝试下一个Pod
			continue
		}

		// 读取日志内容
		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, podLogs)
		podLogs.Close()

		if err != nil {
			logger.Error(err, "读取Pod日志失败", "podName", podName)
			// 继续尝试下一个Pod
			continue
		}

		logs := buf.String()
		if logs != "" {
			if logsFound {
				// 如果已经有之前Pod的日志，添加分隔符
				combinedLogs.WriteString("\n----------\n")
			}
			combinedLogs.WriteString(fmt.Sprintf("Pod %s logs:\n", podName))
			combinedLogs.WriteString(logs)
			logsFound = true
		}
	}

	if !logsFound {
		// 尝试从已删除的Pod获取日志（如果有）
		for _, pod := range pods.Items {
			// 尝试获取Pod状态和退出原因
			combinedLogs.WriteString(fmt.Sprintf("Pod %s status: %s\n", pod.Name, string(pod.Status.Phase)))

			for _, containerStatus := range pod.Status.ContainerStatuses {
				if containerStatus.State.Terminated != nil {
					combinedLogs.WriteString(fmt.Sprintf("Container %s exit code: %d\n",
						containerStatus.Name, containerStatus.State.Terminated.ExitCode))
					combinedLogs.WriteString(fmt.Sprintf("Container %s reason: %s\n",
						containerStatus.Name, containerStatus.State.Terminated.Reason))
					if containerStatus.State.Terminated.Message != "" {
						combinedLogs.WriteString(fmt.Sprintf("Container %s message: %s\n",
							containerStatus.Name, containerStatus.State.Terminated.Message))
					}
				}
			}
		}

		if combinedLogs.Len() == 0 {
			return "", fmt.Errorf("未能获取任何Pod的日志: %s/%s", namespace, jobName)
		}
	}

	return combinedLogs.String(), nil
}

// ExecuteCommand executes a command locally and returns the output
// 注意：这个函数只应该在开发/测试环境中使用，生产环境应该使用Job
func ExecuteCommand(ctx context.Context, command string, args []string) (string, error) {
	logger := log.FromContext(ctx)
	logger.Info("Executing command locally", "command", command, "args", args)

	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error(err, "Command execution failed", "output", string(output))
		return string(output), err
	}

	return string(output), nil
}

// WaitForJob waits for a job to complete with timeout
//func WaitForJob(ctx context.Context, k8sClient client.Client, namespace, name string, timeout time.Duration) error {
//	logger := log.FromContext(ctx)
//	logger.Info("Waiting for job to complete", "namespace", namespace, "name", name, "timeout", timeout)
//
//	deadline := time.Now().Add(timeout)
//	pollInterval := 5 * time.Second
//	consecutiveNotFoundErrors := 0
//	maxConsecutiveNotFoundErrors := 3
//
//	for time.Now().Before(deadline) {
//		var job batchv1.Job
//		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &job); err != nil {
//			if errors.IsNotFound(err) {
//				// 如果Job不存在，可能是被其他进程删除或尚未创建
//				consecutiveNotFoundErrors++
//				logger.Info("Job not found during waiting", "namespace", namespace, "name", name,
//					"consecutiveErrors", consecutiveNotFoundErrors)
//
//				// 如果连续多次找不到Job，认为Job已被删除或永远不会出现
//				if consecutiveNotFoundErrors >= maxConsecutiveNotFoundErrors {
//					logger.Error(err, "Job not found for several consecutive checks, assuming it was deleted or never created",
//						"namespace", namespace, "name", name, "checks", consecutiveNotFoundErrors)
//					return fmt.Errorf("Job.batch \"%s/%s\" not found after %d consecutive checks", namespace, name, consecutiveNotFoundErrors)
//				}
//
//				// 短暂等待后继续下一次检查
//				time.Sleep(2 * time.Second)
//				continue
//			}
//			// 其他错误直接返回
//			logger.Error(err, "Failed to get job")
//			return err
//		}
//
//		// 找到了Job，重置连续错误计数
//		consecutiveNotFoundErrors = 0
//
//		if job.Status.Succeeded > 0 {
//			logger.Info("Job completed successfully", "namespace", namespace, "name", name)
//			return nil
//		}
//
//		if job.Status.Failed > *job.Spec.BackoffLimit {
//			logger.Error(nil, "Job failed", "namespace", namespace, "name", name,
//				"failed", job.Status.Failed, "backoffLimit", *job.Spec.BackoffLimit)
//			return fmt.Errorf("job failed: %s/%s", namespace, name)
//		}
//
//		logger.V(1).Info("Job still running, waiting...", "namespace", namespace, "name", name)
//		time.Sleep(pollInterval)
//	}
//
//	return fmt.Errorf("timeout waiting for job: %s/%s", namespace, name)
//}

// UpdateWithRetry 使用乐观锁重试机制更新资源
// 它会在遇到冲突时重试更新操作，最多重试maxRetries次
//func UpdateWithRetry(ctx context.Context, c client.Client, obj client.Object, updateFn func(), maxRetries int) error {
//	logger := log.FromContext(ctx)
//	if maxRetries <= 0 {
//		maxRetries = 5 // 默认最多重试5次
//	}
//
//	retryDelay := 200 * time.Millisecond
//
//	for i := 0; i < maxRetries; i++ {
//		// 保存资源的当前版本
//		key := types.NamespacedName{
//			Namespace: obj.GetNamespace(),
//			Name:      obj.GetName(),
//		}
//
//		// 只有在重试时才重新获取对象
//		if i > 0 {
//			if err := c.Get(ctx, key, obj); err != nil {
//				return fmt.Errorf("获取资源失败: %w", err)
//			}
//			logger.V(1).Info("重试更新资源", "资源", key, "尝试", i+1, "最大重试次数", maxRetries)
//		}
//
//		// 应用更新函数
//		updateFn()
//
//		// 尝试更新
//		err := c.Status().Update(ctx, obj)
//		if err == nil {
//			// 更新成功
//			logger.V(1).Info("成功更新资源", "资源", key, "尝试次数", i+1)
//			return nil
//		}
//
//		// 检查是否是冲突错误
//		if !errors.IsConflict(err) {
//			// 非冲突错误，直接返回
//			logger.Error(err, "更新资源时发生非冲突错误", "资源", key)
//			return fmt.Errorf("更新资源时发生错误: %w", err)
//		}
//
//		// 发生冲突，等待后重试
//		retryTime := retryDelay * time.Duration(1<<uint(i))
//		logger.V(1).Info("更新发生冲突，将重试", "资源", key, "重试次数", i+1, "等待时间", retryTime)
//		time.Sleep(retryTime)
//	}
//
//	return fmt.Errorf("经过%d次尝试后仍无法更新资源", maxRetries)
//}

// ContainsString 检查slice中是否包含指定字符串
//func ContainsString(slice []string, s string) bool {
//	for _, item := range slice {
//		if item == s {
//			return true
//		}
//	}
//	return false
//}

// RemoveString 从slice中移除指定字符串
//func RemoveString(slice []string, s string) []string {
//	var result []string
//	for _, item := range slice {
//		if item != s {
//			result = append(result, item)
//		}
//	}
//	return result
//}

func IsValidIP(ip string) bool {
	// Check if the string is a valid IP address
	return net.ParseIP(ip) != nil
}
