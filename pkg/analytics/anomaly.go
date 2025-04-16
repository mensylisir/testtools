package analytics

import (
	"sort"
	"strconv"
	"time"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Anomaly 表示检测到的异常
type Anomaly struct {
	// 异常指标的名称
	Metric string `json:"metric"`
	// 异常指标的值
	Value float64 `json:"value"`
	// 异常检测到的时间
	DetectionTime metav1.Time `json:"detectionTime"`
	// 异常执行的时间
	ExecutionTime metav1.Time `json:"executionTime"`
	// 异常的严重程度 (低、中、高)
	Severity string `json:"severity"`
	// 异常的描述
	Description string `json:"description"`
}

// DetectAnomalies 检测异常值
func DetectAnomalies(results []testtoolsv1.TestResult) []Anomaly {
	var anomalies []Anomaly

	// 对每个指标进行异常检测
	metrics := collectMetrics(results)

	for metric, values := range metrics {
		// 使用IQR方法检测异常
		q1, q3 := calculateQuartiles(values)
		iqr := q3 - q1
		lowerBound := q1 - 1.5*iqr
		upperBound := q3 + 1.5*iqr

		// 检查异常值
		for i, value := range values {
			if value < lowerBound || value > upperBound {
				severity := calculateSeverity(value, lowerBound, upperBound)
				description := generateAnomalyDescription(metric, value, lowerBound, upperBound)

				anomalies = append(anomalies, Anomaly{
					Metric:        metric,
					Value:         value,
					DetectionTime: metav1.NewTime(time.Now()),
					ExecutionTime: results[i].ExecutionTime,
					Severity:      severity,
					Description:   description,
				})
			}
		}
	}

	return anomalies
}

// collectMetrics 从测试结果中收集指标
func collectMetrics(results []testtoolsv1.TestResult) map[string][]float64 {
	metrics := make(map[string][]float64)

	for _, result := range results {
		for metric, valueStr := range result.MetricValues {
			value, err := strconv.ParseFloat(valueStr, 64)
			if err == nil {
				metrics[metric] = append(metrics[metric], value)
			}
		}
	}

	return metrics
}

// calculateQuartiles 计算第一和第三四分位
func calculateQuartiles(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}

	// 排序数据
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	// 计算四分位
	q1Index := len(sorted) / 4
	q3Index := len(sorted) * 3 / 4

	return sorted[q1Index], sorted[q3Index]
}

// calculateSeverity 根据偏离程度计算严重性
func calculateSeverity(value, lowerBound, upperBound float64) string {
	if value < lowerBound {
		ratio := (lowerBound - value) / (lowerBound - lowerBound*0.5)
		if ratio > 1.0 {
			return "高"
		} else if ratio > 0.5 {
			return "中"
		}
		return "低"
	}

	if value > upperBound {
		ratio := (value - upperBound) / (upperBound*1.5 - upperBound)
		if ratio > 1.0 {
			return "高"
		} else if ratio > 0.5 {
			return "中"
		}
		return "低"
	}

	return "正常"
}

// generateAnomalyDescription 生成异常描述
func generateAnomalyDescription(metric string, value, lowerBound, upperBound float64) string {
	if value < lowerBound {
		return "指标 " + metric + " 异常偏低，当前值 " + strconv.FormatFloat(value, 'f', 2, 64) +
			" 低于下限 " + strconv.FormatFloat(lowerBound, 'f', 2, 64)
	}

	return "指标 " + metric + " 异常偏高，当前值 " + strconv.FormatFloat(value, 'f', 2, 64) +
		" 高于上限 " + strconv.FormatFloat(upperBound, 'f', 2, 64)
}
