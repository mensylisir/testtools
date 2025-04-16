#!/bin/bash
# 测试FIO资源创建后自动创建TestReport的端到端测试

set -e

echo "创建FIO资源..."
kubectl apply -f config/samples/testtools_v1_fio_basic.yaml

echo "等待处理完成..."
sleep 10

REPORT_NAME="fio-fio-basic-report"
echo "检查TestReport是否自动创建: $REPORT_NAME"
kubectl get testreport $REPORT_NAME -o json | jq .

# 验证TestReport的内容
SELECTOR_NAME=$(kubectl get testreport $REPORT_NAME -o jsonpath='{.spec.resourceSelectors[0].name}')
if [ "$SELECTOR_NAME" != "fio-basic" ]; then
    echo "ERROR: TestReport资源选择器错误"
    exit 1
fi

echo "等待FIO测试完成..."
sleep 60

# 验证结果是否被收集
RESULTS_COUNT=$(kubectl get testreport $REPORT_NAME -o jsonpath='{.status.results}' | jq '. | length')
if [ "$RESULTS_COUNT" -lt 1 ]; then
    echo "ERROR: TestReport未收集结果"
    exit 1
fi

echo "验证Dig测试..."
kubectl apply -f config/samples/testtools_v1_dig_basic.yaml
sleep 10

DIG_REPORT_NAME="dig-dig-basic-report"
kubectl get testreport $DIG_REPORT_NAME -o json | jq .

echo "验证Ping测试..."
kubectl apply -f config/samples/testtools_v1_ping_basic.yaml
sleep 10

PING_REPORT_NAME="ping-ping-basic-report"
kubectl get testreport $PING_REPORT_NAME -o json | jq .

echo "测试通过!" 