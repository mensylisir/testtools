package exporters

import (
	"context"
	"fmt"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// PrometheusExporter 导出测试结果到Prometheus
type PrometheusExporter struct {
	gateway string
	job     string
}

// NewPrometheusExporter 创建新的Prometheus导出器
func NewPrometheusExporter(gateway, job string) *PrometheusExporter {
	return &PrometheusExporter{
		gateway: gateway,
		job:     job,
	}
}

// ExportResults 导出测试结果
func (e *PrometheusExporter) ExportResults(ctx context.Context, testReport *testtoolsv1.TestReport) error {
	logger := log.FromContext(ctx)
	logger.Info("开始导出测试结果到Prometheus", "gateway", e.gateway, "job", e.job)

	pusher := push.New(e.gateway, e.job)

	// 为每个测试结果创建指标
	for _, result := range testReport.Status.Results {
		labels := prometheus.Labels{
			"name":      result.ResourceName,
			"namespace": result.ResourceNamespace,
			"kind":      result.ResourceKind,
			"test_name": testReport.Spec.TestName,
			"test_type": testReport.Spec.TestType,
			"success":   strconv.FormatBool(result.Success),
			"target":    testReport.Spec.Target,
		}

		// 导出响应时间
		responseTimeGauge := prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "testtools_response_time_ms",
			Help:        "测试响应时间(毫秒)",
			ConstLabels: labels,
		})
		responseTimeGauge.Set(result.ResponseTime)
		pusher.Collector(responseTimeGauge)

		// 导出所有指标
		for metric, valueStr := range result.MetricValues {
			value, err := strconv.ParseFloat(valueStr, 64)
			if err != nil {
				logger.Error(err, "无法解析指标值", "metric", metric, "value", valueStr)
				continue
			}

			metricLabels := prometheus.Labels{}
			for k, v := range labels {
				metricLabels[k] = v
			}
			metricLabels["metric"] = metric

			gauge := prometheus.NewGauge(prometheus.GaugeOpts{
				Name:        fmt.Sprintf("testtools_%s", normalizeMetricName(metric)),
				Help:        fmt.Sprintf("TestTools metric: %s", metric),
				ConstLabels: metricLabels,
			})
			gauge.Set(value)
			pusher.Collector(gauge)
		}
	}

	// 导出摘要指标
	summaryLabels := prometheus.Labels{
		"name":      testReport.Name,
		"namespace": testReport.Namespace,
		"test_name": testReport.Spec.TestName,
		"test_type": testReport.Spec.TestType,
		"target":    testReport.Spec.Target,
	}

	totalGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "testtools_summary_total",
		Help:        "测试总数",
		ConstLabels: summaryLabels,
	})
	totalGauge.Set(float64(testReport.Status.Summary.Total))
	pusher.Collector(totalGauge)

	succeededGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "testtools_summary_succeeded",
		Help:        "成功测试数",
		ConstLabels: summaryLabels,
	})
	succeededGauge.Set(float64(testReport.Status.Summary.Succeeded))
	pusher.Collector(succeededGauge)

	failedGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "testtools_summary_failed",
		Help:        "失败测试数",
		ConstLabels: summaryLabels,
	})
	failedGauge.Set(float64(testReport.Status.Summary.Failed))
	pusher.Collector(failedGauge)

	avgRespTimeGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "testtools_summary_avg_response_time_ms",
		Help:        "平均响应时间(毫秒)",
		ConstLabels: summaryLabels,
	})
	avgRespTimeGauge.Set(testReport.Status.Summary.AverageResponseTime)
	pusher.Collector(avgRespTimeGauge)

	logger.Info("完成导出测试结果到Prometheus")
	return pusher.Push()
}

// normalizeMetricName 标准化指标名称，使其符合Prometheus命名规范
func normalizeMetricName(name string) string {
	// Prometheus指标名称只能包含字母、数字和下划线
	// 将非法字符替换为下划线
	result := ""
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			result += string(r)
		} else {
			result += "_"
		}
	}

	// 如果第一个字符是数字，前面加上下划线
	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
		result = "_" + result
	}

	return result
}
