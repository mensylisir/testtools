package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TestFioControllerInteraction 测试FIO控制器与TestReport控制器的交互
func TestFioControllerInteraction(t *testing.T) {
	// 设置测试环境
	// 注册自定义资源
	s := scheme.Scheme
	require.NoError(t, testtoolsv1.AddToScheme(s))

	// 创建测试FIO资源
	fio := &testtoolsv1.Fio{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-fio",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
		Status: testtoolsv1.FioStatus{
			TestReportName:    "test-fio-report",
			LastExecutionTime: &metav1.Time{Time: time.Now()},
			Status:            "Succeeded",
			Stats: testtoolsv1.FioStats{
				ReadIOPS:     "1000",
				WriteIOPS:    "500",
				ReadBW:       "4096",
				WriteBW:      "2048",
				ReadLatency:  "10",
				WriteLatency: "15",
			},
		},
	}

	// 创建对应的测试报告资源
	testReport := &testtoolsv1.TestReport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-fio-report",
			Namespace: "default",
		},
		Spec: testtoolsv1.TestReportSpec{
			TestName: "FIO性能测试",
			TestType: "Performance",
			Target:   "storage",
			ResourceSelectors: []testtoolsv1.ResourceSelector{
				{
					Kind:       "Fio",
					Name:       "test-fio",
					Namespace:  "default",
					APIVersion: "testtools.xiaoming.com/v1",
				},
			},
		},
	}

	// 创建伪客户端并添加测试资源
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(fio, testReport).Build()

	// 创建控制器，使用正确的客户端和Scheme
	r := &TestReportReconciler{
		Client: c,
		Scheme: s,
	}

	// 执行测试
	requests := r.findTestReportForResource(context.Background(), fio)

	// 验证
	assert.Equal(t, 1, len(requests))
	assert.Equal(t, "test-fio-report", requests[0].NamespacedName.Name)
	assert.Equal(t, "default", requests[0].NamespacedName.Namespace)
}

// TestDigControllerInteraction 测试Dig控制器与TestReport控制器的交互
func TestDigControllerInteraction(t *testing.T) {
	// 设置测试环境
	// 注册自定义资源
	s := scheme.Scheme
	require.NoError(t, testtoolsv1.AddToScheme(s))

	// 创建测试Dig资源
	dig := &testtoolsv1.Dig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dig",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
		Status: testtoolsv1.DigStatus{
			TestReportName:    "test-dig-report",
			LastExecutionTime: &metav1.Time{Time: time.Now()},
			Status:            "Succeeded",
		},
	}

	// 创建对应的测试报告资源
	testReport := &testtoolsv1.TestReport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dig-report",
			Namespace: "default",
		},
		Spec: testtoolsv1.TestReportSpec{
			TestName: "DNS测试",
			TestType: "DNS",
			Target:   "network",
			ResourceSelectors: []testtoolsv1.ResourceSelector{
				{
					Kind:       "Dig",
					Name:       "test-dig",
					Namespace:  "default",
					APIVersion: "testtools.xiaoming.com/v1",
				},
			},
		},
	}

	// 创建伪客户端并添加测试资源
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(dig, testReport).Build()

	// 创建控制器，使用正确的客户端和Scheme
	r := &TestReportReconciler{
		Client: c,
		Scheme: s,
	}

	// 执行测试
	requests := r.findTestReportForResource(context.Background(), dig)

	// 验证
	assert.Equal(t, 1, len(requests))
	assert.Equal(t, "test-dig-report", requests[0].NamespacedName.Name)
	assert.Equal(t, "default", requests[0].NamespacedName.Namespace)
}

// TestPingControllerInteraction 测试Ping控制器与TestReport控制器的交互
func TestPingControllerInteraction(t *testing.T) {
	// 设置测试环境
	// 注册自定义资源
	s := scheme.Scheme
	require.NoError(t, testtoolsv1.AddToScheme(s))

	// 创建测试Ping资源
	ping := &testtoolsv1.Ping{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ping",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
		Status: testtoolsv1.PingStatus{
			TestReportName:    "test-ping-report",
			LastExecutionTime: &metav1.Time{Time: time.Now()},
			Status:            "Succeeded",
		},
	}

	// 创建对应的测试报告资源
	testReport := &testtoolsv1.TestReport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ping-report",
			Namespace: "default",
		},
		Spec: testtoolsv1.TestReportSpec{
			TestName: "网络连通性测试",
			TestType: "Network",
			Target:   "connectivity",
			ResourceSelectors: []testtoolsv1.ResourceSelector{
				{
					Kind:       "Ping",
					Name:       "test-ping",
					Namespace:  "default",
					APIVersion: "testtools.xiaoming.com/v1",
				},
			},
		},
	}

	// 创建伪客户端并添加测试资源
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ping, testReport).Build()

	// 创建控制器，使用正确的客户端和Scheme
	r := &TestReportReconciler{
		Client: c,
		Scheme: s,
	}

	// 执行测试
	requests := r.findTestReportForResource(context.Background(), ping)

	// 验证
	assert.Equal(t, 1, len(requests))
	assert.Equal(t, "test-ping-report", requests[0].NamespacedName.Name)
	assert.Equal(t, "default", requests[0].NamespacedName.Namespace)
}

// TestTestReportReconciler_Reconcile 测试TestReport控制器的Reconcile方法
func TestTestReportReconciler_Reconcile(t *testing.T) {
	// 注册自定义资源
	s := scheme.Scheme
	require.NoError(t, testtoolsv1.AddToScheme(s))

	// 创建测试TestReport资源
	testReport := &testtoolsv1.TestReport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-report",
			Namespace: "default",
		},
		Spec: testtoolsv1.TestReportSpec{
			TestName: "性能测试",
			TestType: "Performance",
			Target:   "storage",
			ResourceSelectors: []testtoolsv1.ResourceSelector{
				{
					Kind:       "Fio",
					Name:       "test-fio",
					Namespace:  "default",
					APIVersion: "testtools.xiaoming.com/v1",
				},
			},
		},
	}

	// 创建FIO资源
	fio := &testtoolsv1.Fio{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-fio",
			Namespace: "default",
		},
		Status: testtoolsv1.FioStatus{
			TestReportName:    "test-report",
			LastExecutionTime: &metav1.Time{Time: time.Now()},
			Status:            "Succeeded",
			Stats: testtoolsv1.FioStats{
				ReadIOPS:     "1000",
				WriteIOPS:    "500",
				ReadBW:       "4096",
				WriteBW:      "2048",
				ReadLatency:  "10",
				WriteLatency: "15",
			},
		},
	}

	// 创建伪客户端
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(testReport, fio).Build()

	// 创建控制器
	r := &TestReportReconciler{
		Client: c,
		Scheme: s,
	}

	// 执行Reconcile方法
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-report",
			Namespace: "default",
		},
	}

	// 调用Reconcile方法
	_, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)

	// 获取更新后的TestReport资源
	updatedReport := &testtoolsv1.TestReport{}
	err = c.Get(context.Background(), types.NamespacedName{Name: "test-report", Namespace: "default"}, updatedReport)
	require.NoError(t, err)

	// 验证状态是否更新
	require.NotEmpty(t, updatedReport.Status.Phase)
}
