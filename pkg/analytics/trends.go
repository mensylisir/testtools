package analytics

import (
	"strconv"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
)

// TrendAnalysis 包含性能趋势分析的结果
type TrendAnalysis struct {
	// 各指标随时间变化的平均值
	AverageOverTime map[string][]float64 `json:"averageOverTime,omitempty"`
	// 各指标的趋势方向
	Trend map[string]string `json:"trend,omitempty"`
	// 趋势变化率
	ChangeRate map[string]float64 `json:"changeRate,omitempty"`
}

// PerformanceTrend 分析性能趋势
func PerformanceTrend(results []testtoolsv1.TestResult) *TrendAnalysis {
	// 实现性能趋势分析
	trend := &TrendAnalysis{
		AverageOverTime: make(map[string][]float64),
		Trend:           make(map[string]string),
		ChangeRate:      make(map[string]float64),
	}

	// 分析数据
	for _, result := range results {
		for metric, value := range result.MetricValues {
			if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
				trend.AverageOverTime[metric] = append(trend.AverageOverTime[metric], floatValue)
			}
		}
	}

	// 计算趋势
	for metric, values := range trend.AverageOverTime {
		if len(values) >= 3 {
			// 简单线性回归
			slope := calculateSlope(values)
			trend.ChangeRate[metric] = slope

			if slope > 0.05 {
				trend.Trend[metric] = "上升"
			} else if slope < -0.05 {
				trend.Trend[metric] = "下降"
			} else {
				trend.Trend[metric] = "稳定"
			}
		}
	}

	return trend
}

// calculateSlope 计算简单线性回归的斜率
func calculateSlope(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	n := float64(len(values))
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumXX := 0.0

	for i, y := range values {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}

	// 计算斜率: m = (n*∑xy - ∑x*∑y) / (n*∑x² - (∑x)²)
	m := (n*sumXY - sumX*sumY) / (n*sumXX - sumX*sumX)

	// 归一化斜率，计算变化率
	if len(values) > 0 && values[0] != 0 {
		m = m / values[0]
	}

	return m
}
