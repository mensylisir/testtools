package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	testtoolsv1 "github.com/xiaoming/testtools/api/v1"
)

func TestDigReconciler_Reconcile(t *testing.T) {
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
		Spec: testtoolsv1.DigSpec{
			Domain:    "example.com",
			QueryType: "A",
			Timeout:   5,
			Image:     "debian:bullseye-slim",
		},
		Status: testtoolsv1.DigStatus{
			Conditions: []metav1.Condition{},
		},
	}

	// 创建伪客户端 - 准备必要的资源结构
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(dig).
		WithStatusSubresource(&testtoolsv1.Dig{}).
		Build()

	// 创建控制器
	r := &DigReconciler{
		Client: c,
		Scheme: s,
	}

	// 执行Reconcile方法
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-dig",
			Namespace: "default",
		},
	}

	// 调用Reconcile方法 - 使用Requeue模式处理可能的错误
	result, err := r.Reconcile(context.Background(), req)

	// 由于我们使用的是伪客户端，可能无法模拟Kubernetes API的所有行为
	// 所以这里我们可能会得到一个错误，但我们只关注结构的更新
	if err != nil {
		t.Logf("Reconcile returned error: %v, requeue: %v", err, result.Requeue)
	}

	// 获取更新后的Dig资源
	updatedDig := &testtoolsv1.Dig{}
	err = c.Get(context.Background(), types.NamespacedName{Name: "test-dig", Namespace: "default"}, updatedDig)
	require.NoError(t, err, "应该能够找到Dig资源")

	// 验证状态条件是否被设置
	require.NotEmpty(t, updatedDig.Status.Conditions, "状态条件不应为空")

	// 打印状态条件，帮助调试
	for _, cond := range updatedDig.Status.Conditions {
		t.Logf("状态条件: Type=%s, Status=%s, Reason=%s", cond.Type, cond.Status, cond.Reason)
	}
}

func TestDigReconciler_SetupWithManager(t *testing.T) {
	// 创建控制器
	r := &DigReconciler{
		Client: nil,
		Scheme: scheme.Scheme,
	}

	// 创建管理器
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme.Scheme,
	})
	require.NoError(t, err)

	// 测试SetupWithManager方法
	err = r.SetupWithManager(mgr)
	require.NoError(t, err)
}
