<template>
  <div class="ping-detail">
    <div class="header-actions">
      <h1>连通性测试详情</h1>
      <div>
        <router-link to="/ping" class="btn btn-secondary">返回列表</router-link>
      </div>
    </div>

    <!-- 加载中显示 -->
    <div v-if="loading" class="loading-container">
      <div class="spinner"></div>
      <p>加载中...</p>
    </div>

    <!-- 错误显示 -->
    <div v-else-if="error" class="error-container card">
      <h3>错误</h3>
      <p class="error-message">{{ error }}</p>
      <div class="error-actions">
        <button @click="fetchPingData" class="btn btn-primary">重试</button>
        <router-link to="/ping" class="btn btn-secondary">返回列表</router-link>
      </div>
    </div>

    <!-- Ping详情 -->
    <template v-else>
      <div class="card">
        <div class="card-header">
          <h2>{{ ping.metadata.name }}</h2>
            <span class="status-badge" :class="getStatusClass(ping.status?.status)">
              {{ ping.status?.status || '等待中' }}
            </span>
        </div>

        <div class="card-body">
          <div class="info-grid">
            <div class="info-item">
              <div class="info-label">地址</div>
              <div class="info-value">{{ ping.spec.host }}</div>
            </div>

            <div class="info-item">
              <div class="info-label">请求次数</div>
              <div class="info-value">{{ ping.spec.count }}</div>
            </div>

            <div class="info-item">
              <div class="info-label">间隔</div>
              <div class="info-value">{{ ping.spec.interval }}</div>
            </div>

            <div class="info-item">
              <div class="info-label">最后执行时间</div>
              <div class="info-value">{{ formatDate(ping.status.lastExecutionTime) }}</div>
            </div>
          </div>

          <div v-if="isRunning" class="refresh-status">
            <span>正在执行Ping查询...</span>
            <div class="refresh-progress">
              <div class="progress-bar" :style="{ width: progressWidth }"></div>
            </div>
          </div>
        </div>
      </div>

      <!-- 概览信息 -->
      <div class="card">
        <div class="card-header">
          <h3>统计信息</h3>
        </div>
        <div class="card-body">
          <div class="summary-grid">
            <div class="summary-item">
              <div class="summary-label">总查询次数</div>
              <div class="summary-value">{{ ping.status.queryCount || 0 }}</div>
            </div>

            <div class="summary-item">
              <div class="summary-label">成功次数</div>
              <div class="summary-value">{{ ping.status.successCount || 0 }}</div>
            </div>

            <div class="summary-item">
              <div class="summary-label">失败次数</div>
              <div class="summary-value">{{ ping.status.failureCount || 0 }}</div>
            </div>

          </div>
        </div>
      </div>

      <!-- 配置详情 -->
      <div class="card">
        <div class="card-header">
          <h3>配置详情</h3>
          <button
            @click="showConfigDetails = !showConfigDetails"
            class="btn btn-sm"
          >
            {{ showConfigDetails ? '收起' : '展开' }}
          </button>
        </div>


        <div v-if="showConfigDetails" class="card-body">
          <div class="config-list">
            <div
              v-for="(value, key) in configDetails"
              :key="key"
              class="config-item"
            >
              <div class="config-label">{{ formatConfigKey(key) }}</div>
              <div class="config-value">{{ formatConfigValue(value) }}</div>
            </div>
          </div>
        </div>
      </div>

      <!-- 测试结果 -->
      <div class="card" v-if="ping.status && ping.status.testReportName">
        <div class="card-header">
          <h3>详细结果</h3>
          <button
              @click="refreshData"
              class="btn btn-primary btn-sm"
              :disabled="refreshing"
          >
            <span v-if="refreshing" class="spinner-small"></span>
            <span>刷新</span>
          </button>
        </div>
        <div class="card-body">
          <pre v-if="ping.status.lastResult" class="output-pre">{{ ping.status.lastResult }}</pre>
          <div v-else class="text-center">
            <p>暂无查询结果</p>
          </div>
        </div>
      </div>
      <div v-if="ping.status.testReportName" class="card">
        <div class="card-header">
          <h3>测试报告</h3>
          <button
              @click="fetchTestReport"
              class="btn btn-secondary btn-sm"
              :disabled="loadingTestReport"
          >
            <span v-if="loadingTestReport" class="spinner-small"></span>
            <span>{{ testReport ? '刷新' : '加载' }}</span>
          </button>
        </div>

        <div class="card-body">
          <div v-if="loadingTestReport" class="loading-container">
            <div class="spinner"></div>
            <p>加载测试报告中...</p>
          </div>

          <div v-else-if="testReport">
            <h4 class="mb-4">测试摘要</h4>
            <div class="summary-grid">
              <div class="summary-item">
                <div class="summary-label">平均延迟</div>
                <div class="summary-value">{{ testReport.status.summary.averageResponseTime }}</div>
              </div>

              <div class="summary-item">
                <div class="summary-label">最大延迟</div>
                <div class="summary-value">{{ testReport.status.summary.maxResponseTime }}</div>
              </div>

              <div class="summary-item">
                <div class="summary-label">最小延迟</div>
                <div class="summary-value">{{ testReport.status.summary.minResponseTime }}</div>
              </div>

              <div class="summary-item">
                <div class="summary-label">丢包率</div>
                <div class="summary-value">{{ testReport.status.summary.min || '-' }}</div>
              </div>
            </div>

            <h4 class="mb-4">最近结果</h4>
            <div class="space-y-4">
              <div
                  v-for="(result, index) in testReport.status.results.slice(0, 5)"
                  :key="index"
                  class="info-item"
              >
                <div class="flex justify-between items-center mb-3">
                  <span class="status-badge" :class="result.success ? 'status-success' : 'status-failed'">
                    {{ result.success ? '成功' : '失败' }}
                  </span>
                  <span class="text-sm">{{ formatDate(result.executionTime) }}</span>
                </div>

                <div class="info-grid mb-3">
                  <div class="info-item">
                    <div class="info-label">响应时间</div>
                    <div class="info-value">{{ result.responseTime || '-' }}</div>
                  </div>

                  <div v-if="result.error" class="info-item">
                    <div class="info-label">错误</div>
                    <div class="info-value error-text">{{ result.error }}</div>
                  </div>
                </div>

                <div v-if="result.output" class="mt-3">
                  <pre class="output-pre">{{ result.output }}</pre>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="testReportError" class="error-container">
            <p class="error-message">{{ testReportError }}</p>
            <div class="error-actions">
              <button @click="fetchTestReport" class="btn btn-primary">重试</button>
            </div>
          </div>

          <div v-else class="text-center">
            <p>暂未加载测试报告</p>
          </div>
        </div>
      </div>
    </template>

    <!-- 删除确认对话框 -->
    <div v-if="showDeleteConfirm" class="modal-overlay">
      <div class="modal-container card">
        <h3>确认删除</h3>
        <p>确定要删除 <strong>{{ pingName }}</strong> 吗？此操作不可逆。</p>
        <div class="modal-actions">
          <button @click="showDeleteConfirm = false" class="btn">取消</button>
          <button @click="deletePing" class="btn btn-danger" :disabled="deleting">
            <span v-if="deleting">删除中...</span>
            <span v-else>确认删除</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import pingApi from '../api/ping.js'

export default {
  setup() {
    const route = useRoute()
    const router = useRouter()
    const pingName = computed(() => route.params.name)

    const ping = ref(null)
    const loading = ref(true)
    const error = ref(null)
    const refreshing = ref(false)
    const showConfigDetails = ref(false)
    const testReport = ref(null)
    const loadingTestReport = ref(false)
    const testReportError = ref(null)
    const refreshInterval = ref(null)
    const refreshProgress = ref(0)
    const showDeleteConfirm = ref(false)
    const deleting = ref(false)

    // 是否正在运行查询
    const isRunning = computed(() => {
      if (!ping.value || !ping.value.status) return false

      const status = (ping.value.status.status || '').toLowerCase()
      return status.includes('progress') || status === 'running' || status === 'pending'
    })

    // 进度条宽度
    const progressWidth = computed(() => {
      return `${refreshProgress.value}%`
    })

    // 格式化配置显示
    const configDetails = computed(() => {
      if (!ping.value) return {}

      const details = { ...ping.value.spec }
      return details
    })

    // 获取Ping详情
    const fetchPingData = async () => {
      loading.value = true
      error.value = null

      try {
        const response = await pingApi.getPing(pingName.value)
        ping.value = response

        // 如果有测试报告，自动加载
        if (response.status && response.status.testReportName) {
          fetchTestReport()
        }
      } catch (err) {
        error.value = '获取Ping详情失败: ' + (err.message || '未知错误')
        console.error(err)
      } finally {
        loading.value = false
      }
    }

    // 刷新数据
    const refreshData = async () => {
      if (refreshing.value) return

      refreshing.value = true
      error.value = null

      try {
        const response = await pingApi.getPing(pingName.value)
        ping.value = response

        // 如果状态有更新且有测试报告
        if (response.status && response.status.testReportName) {
          fetchTestReport()
        }
      } catch (err) {
        error.value = '刷新数据失败: ' + (err.message || '未知错误')
        console.error(err)
      } finally {
        refreshing.value = false
      }
    }

    // 获取TestReport
    const fetchTestReport = async () => {
      if (!ping.value || !ping.value.status.testReportName) return

      loadingTestReport.value = true
      testReportError.value = null

      try {
        const response = await pingApi.getPingTestReport(ping.value.status.testReportName)
        testReport.value = response
      } catch (err) {
        testReportError.value = '获取测试报告失败: ' + (err.message || '未知错误')
        console.error(err)
      } finally {
        loadingTestReport.value = false
      }
    }

    // 确认删除
    const confirmDelete = () => {
      showDeleteConfirm.value = true
    }

    // 删除Ping
    const deletePing = async () => {
      deleting.value = true

      try {
        await pingApi.deletePing(pingName.value)
        router.push('/ping')
      } catch (err) {
        error.value = '删除失败: ' + (err.message || '未知错误')
        console.error(err)
        showDeleteConfirm.value = false
      } finally {
        deleting.value = false
      }
    }

    // 格式化配置键名
    const formatConfigKey = (key) => {
      if (!key) return ''

      // 将驼峰命名转换为空格分隔的名称
      return key.replace(/([A-Z])/g, ' $1')
        .replace(/^./, str => str.toUpperCase())
        // 对特定缩写进行处理
        .replace(/Ip/g, 'IP')
    }

    // 格式化配置值
    const formatConfigValue = (value) => {
      if (value === null || value === undefined) return '-'

      if (typeof value === 'boolean') {
        return value ? '是' : '否'
      }

      return value.toString()
    }

    // 获取状态样式类
    const getStatusClass = (status) => {
      if (!status) return 'status-pending'

      status = status.toLowerCase()
      if (status.includes('success') || status.includes('succeeded') || status === 'completed') return 'status-success'
      if (status.includes('fail') || status.includes('failed') || status.includes('error')) return 'status-error'
      if (status.includes('progress') || status === 'running') return 'status-running'

      return 'status-pending'
    }

    // 格式化日期
    const formatDate = (dateString) => {
      if (!dateString) return '-'

      try {
        const date = new Date(dateString)
        return date.toLocaleString()
      } catch (e) {
        return dateString
      }
    }

    // 设置自动刷新
    const setupAutoRefresh = () => {
      // 清除现有定时器
      clearInterval(refreshInterval.value)

      // 如果任务正在运行，设置定时刷新
      if (isRunning.value) {
        // 每5秒刷新一次
        refreshInterval.value = setInterval(() => {
          refreshData()
        }, 5000)

        // 设置进度条动画
        let progressCounter = 0
        const progressAnimInterval = setInterval(() => {
          progressCounter = (progressCounter + 1) % 100
          refreshProgress.value = progressCounter
        }, 50)

        // 保存进度条定时器引用
        refreshInterval.value = [refreshInterval.value, progressAnimInterval]
      }
    }

    // 组件挂载时获取数据
    onMounted(() => {
      fetchPingData()
    })

    // 监听Ping数据变化，设置自动刷新
    watch(() => ping.value, (newVal) => {
      if (newVal) {
        setupAutoRefresh()
      }
    }, { deep: true })

    // 监听运行状态变化
    watch(isRunning, () => {
      setupAutoRefresh()
    })

    // 组件卸载时清除定时器
    onUnmounted(() => {
      if (refreshInterval.value) {
        if (Array.isArray(refreshInterval.value)) {
          refreshInterval.value.forEach(interval => clearInterval(interval))
        } else {
          clearInterval(refreshInterval.value)
        }
      }
    })

    return {
      pingName,
      ping,
      loading,
      error,
      refreshing,
      showConfigDetails,
      testReport,
      loadingTestReport,
      testReportError,
      isRunning,
      progressWidth,
      configDetails,
      showDeleteConfirm,
      deleting,
      fetchPingData,
      refreshData,
      fetchTestReport,
      confirmDelete,
      deletePing,
      formatConfigKey,
      formatConfigValue,
      getStatusClass,
      formatDate
    }
  }
}
</script>

<style scoped>
.ping-detail {
  padding: 1rem 0;
}

.header-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1.5rem;
}

.header-actions h1 {
  margin: 0;
  color: var(--primary-color);
  font-size: 1.5rem;
  font-weight: 600;
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 1rem;
}

.info-item {
  padding: 0.75rem;
  background-color: #f9f9f9;
  border-radius: 4px;
}

.info-label {
  font-size: 0.8rem;
  color: var(--text-secondary);
  margin-bottom: 0.25rem;
}

.info-value {
  font-size: 0.95rem;
  font-weight: 500;
}

.status-badge {
  display: inline-block;
  padding: 0.25rem 0.5rem;
  border-radius: 9999px;
  font-size: 0.75rem;
  font-weight: 500;
}

.status-success {
  background-color: #d1fae5;
  color: #065f46;
}

.status-pending {
  background-color: #fef3c7;
  color: #92400e;
}

.status-running {
  background-color: #dbeafe;
  color: #1e40af;
}

.status-error {
  background-color: #fee2e2;
  color: #b91c1c;
}

.status-failed {
  background-color: #fee2e2;
  color: #b91c1c;
}

.spinner {
  width: 40px;
  height: 40px;
  border: 4px solid rgba(0, 0, 0, 0.1);
  border-radius: 50%;
  border-left-color: var(--primary-color);
  animation: spin 1s linear infinite;
  margin-bottom: 1rem;
}

.spinner-small {
  display: inline-block;
  width: 16px;
  height: 16px;
  border: 2px solid rgba(0, 0, 0, 0.1);
  border-radius: 50%;
  border-left-color: white;
  animation: spin 1s linear infinite;
  margin-right: 0.5rem;
  vertical-align: middle;
}

.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 2rem 0;
}

.error-container {
  padding: 1rem;
  background-color: #ffebee;
  color: var(--danger-color);
  margin-bottom: 1rem;
  border-radius: 4px;
}

.error-container h3 {
  margin-top: 0;
  margin-bottom: 0.5rem;
  font-size: 1.2rem;
}

.error-message {
  margin-bottom: 1rem;
}

.error-actions {
  display: flex;
  gap: 0.5rem;
}

.error-text {
  color: var(--danger-color);
}

.refresh-status {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.5rem;
  font-size: 0.9rem;
  color: var(--text-secondary);
}

.refresh-progress {
  width: 100px;
  height: 4px;
  background-color: #e9ecef;
  border-radius: 2px;
  overflow: hidden;
}

.progress-bar {
  height: 100%;
  background-color: var(--primary-color);
  transition: width 0.1s ease;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 1rem;
  margin-top: 1rem;
}

.summary-item {
  background-color: #f9f9f9;
  padding: 0.75rem;
  border-radius: 4px;
  text-align: center;
}

.summary-label {
  font-size: 0.8rem;
  color: var(--text-secondary);
  margin-bottom: 0.5rem;
}

.summary-value {
  font-size: 1.2rem;
  font-weight: 500;
  color: var(--text-primary);
}

.output-pre {
  background-color: #f5f5f5;
  padding: 1rem;
  border-radius: 4px;
  overflow-x: auto;
  font-family: monospace;
  font-size: 0.9rem;
  white-space: pre-wrap;
  margin-top: 1rem;
}

.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: rgba(0, 0, 0, 0.5);
  display: flex;
  justify-content: center;
  align-items: center;
  z-index: 1000;
}

.modal-container {
  width: 90%;
  max-width: 500px;
  padding: 1.5rem;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
  margin-top: 1.5rem;
}

.config-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
  gap: 1rem;
}

.config-item {
  padding: 0.75rem;
  background-color: #f9f9f9;
  border-radius: 4px;
}

.config-label {
  font-size: 0.8rem;
  color: var(--text-secondary);
  margin-bottom: 0.25rem;
}

.config-value {
  font-size: 0.95rem;
}

.mb-4 {
  margin-bottom: 1rem;
}

.btn-sm {
  padding: 0.25rem 0.5rem;
  font-size: 0.875rem;
}

@keyframes spin {
  0% { transform: rotate(0deg); }
  100% { transform: rotate(360deg); }
}
</style>