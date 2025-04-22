<template>
  <div class="dig-detail">
    <div class="header-actions">
      <h1>DNS查询详情</h1>
      <router-link to="/dig" class="btn btn-secondary">返回查询列表</router-link>
    </div>

    <div v-if="loading" class="loading-container">
      <div class="spinner"></div>
      <p>加载中...</p>
    </div>

    <div v-else-if="error" class="error-container card">
      <h3>错误</h3>
      <p>{{ error }}</p>
      <button @click="fetchDigData" class="btn btn-primary">重试</button>
    </div>

    <template v-else>
      <!-- 状态摘要卡片 -->
      <div class="card status-card">
        <div class="status-header">
          <h2>{{ dig.metadata.name }}</h2>
          <span :class="'status-badge ' + getStatusClass(dig.status.status)">
            {{ dig.status.status || '等待中' }}
          </span>
        </div>
        
        <div class="status-details">
          <div class="status-item">
            <span class="status-label">域名:</span>
            <span class="status-value">{{ dig.spec.domain }}</span>
          </div>
          
          <div class="status-item">
            <span class="status-label">查询类型:</span>
            <span class="status-value">{{ dig.spec.queryType || 'A' }}</span>
          </div>
          
          <div class="status-item">
            <span class="status-label">DNS服务器:</span>
            <span class="status-value">{{ dig.spec.server || '默认' }}</span>
          </div>
          
          <div class="status-item">
            <span class="status-label">最后执行时间:</span>
            <span class="status-value">{{ formatDate(dig.status.lastExecutionTime) }}</span>
          </div>
        </div>
        
        <template v-if="isRunning">
          <div class="progress-section">
            <span>正在执行DNS查询...</span>
            <div class="progress-bar">
              <div class="progress-bar-fill" :style="{ width: progressWidth }"></div>
            </div>
          </div>
        </template>
      </div>

      <!-- 统计信息卡片 -->
      <div class="card stats-card">
        <h3>统计信息</h3>
        <div class="stats-grid">
          <div class="stat-item">
            <div class="stat-value">{{ dig.status.queryCount || 0 }}</div>
            <div class="stat-label">总查询次数</div>
          </div>
          
          <div class="stat-item">
            <div class="stat-value success">{{ dig.status.successCount || 0 }}</div>
            <div class="stat-label">成功次数</div>
          </div>
          
          <div class="stat-item">
            <div class="stat-value danger">{{ dig.status.failureCount || 0 }}</div>
            <div class="stat-label">失败次数</div>
          </div>
          
          <div class="stat-item">
            <div class="stat-value">{{ dig.status.averageResponseTime || '-' }}</div>
            <div class="stat-label">平均响应时间</div>
          </div>
        </div>
      </div>

      <!-- 详细配置卡片 -->
      <div class="card config-card">
        <div class="card-header">
          <h3>配置详情</h3>
          <button @click="showConfigDetails = !showConfigDetails" class="btn btn-secondary small-btn">
            {{ showConfigDetails ? '收起' : '展开' }}
          </button>
        </div>
        
        <div v-if="showConfigDetails" class="config-details">
          <div class="config-row" v-for="(value, key) in configDetails" :key="key">
            <div class="config-key">{{ formatConfigKey(key) }}:</div>
            <div class="config-value">{{ formatConfigValue(value) }}</div>
          </div>
        </div>
      </div>

      <!-- 最后一次查询结果 -->
      <div class="card result-card">
        <div class="card-header">
          <h3>最后一次查询结果</h3>
          <button @click="refreshData" class="btn btn-primary small-btn">
            <span v-if="refreshing"><div class="spinner-small"></div></span>
            <span>刷新</span>
          </button>
        </div>
        
        <div v-if="dig.status.lastResult" class="result-content">
          <pre>{{ dig.status.lastResult }}</pre>
        </div>
        <div v-else class="empty-result">
          <p>暂无查询结果</p>
        </div>
      </div>

      <!-- TestReport详情卡片 -->
      <div v-if="dig.status.testReportName" class="card testreport-card">
        <div class="card-header">
          <h3>测试报告</h3>
          <button @click="fetchTestReport" class="btn btn-secondary small-btn" :disabled="loadingTestReport">
            <span v-if="loadingTestReport"><div class="spinner-small"></div></span>
            <span>{{ testReport ? '刷新' : '加载' }}</span>
          </button>
        </div>
        
        <div v-if="loadingTestReport" class="loading-container-small">
          <div class="spinner"></div>
          <p>加载测试报告中...</p>
        </div>
        
        <div v-else-if="testReport" class="testreport-details">
          <h4>测试摘要</h4>
          <div class="summary-stats">
            <div class="summary-stat">
              <span class="summary-label">总测试数:</span>
              <span class="summary-value">{{ testReport.status.summary.total }}</span>
            </div>
            
            <div class="summary-stat">
              <span class="summary-label">成功:</span>
              <span class="summary-value success">{{ testReport.status.summary.succeeded }}</span>
            </div>
            
            <div class="summary-stat">
              <span class="summary-label">失败:</span>
              <span class="summary-value danger">{{ testReport.status.summary.failed }}</span>
            </div>
            
            <div class="summary-stat">
              <span class="summary-label">平均响应时间:</span>
              <span class="summary-value">{{ testReport.status.summary.averageResponseTime || '-' }}</span>
            </div>
          </div>
          
          <h4>最近结果</h4>
          <div class="results-list">
            <div v-for="(result, index) in testReport.status.results.slice(0, 5)" :key="index" class="result-item card">
              <div class="result-header">
                <span :class="'status-badge ' + (result.success ? 'status-success' : 'status-error')">
                  {{ result.success ? '成功' : '失败' }}
                </span>
                <span class="result-time">{{ formatDate(result.executionTime) }}</span>
              </div>
              
              <div class="result-details">
                <div class="result-metric">
                  <span class="result-metric-label">响应时间:</span>
                  <span class="result-metric-value">{{ result.responseTime || '-' }}</span>
                </div>
                
                <div v-if="result.error" class="result-error">
                  <span class="result-error-label">错误:</span>
                  <span class="result-error-value">{{ result.error }}</span>
                </div>
              </div>
              
              <div v-if="result.output" class="result-output">
                <pre>{{ result.output }}</pre>
              </div>
            </div>
          </div>
        </div>
        
        <div v-else-if="testReportError" class="error-message">
          <p>{{ testReportError }}</p>
          <button @click="fetchTestReport" class="btn btn-secondary">重试</button>
        </div>
        
        <div v-else class="empty-result">
          <p>暂未加载测试报告</p>
        </div>
      </div>
    </template>
  </div>
</template>

<script>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import digApi from '../api/dig.js'

export default {
  setup() {
    const route = useRoute()
    const digName = computed(() => route.params.name)
    
    const dig = ref(null)
    const loading = ref(true)
    const error = ref(null)
    const refreshing = ref(false)
    const showConfigDetails = ref(false)
    const testReport = ref(null)
    const loadingTestReport = ref(false)
    const testReportError = ref(null)
    const refreshInterval = ref(null)
    const refreshProgress = ref(0)

    // 是否正在运行查询
    const isRunning = computed(() => {
      if (!dig.value || !dig.value.status) return false
      
      const status = (dig.value.status.status || '').toLowerCase()
      return status.includes('progress') || status === 'running' || status === 'pending'
    })

    // 进度条宽度
    const progressWidth = computed(() => {
      return `${refreshProgress.value}%`
    })

    // 格式化配置显示
    const configDetails = computed(() => {
      if (!dig.value) return {}
      
      const details = { ...dig.value.spec }
      return details
    })

    // 获取Dig详情
    const fetchDigData = async () => {
      loading.value = true
      error.value = null
      
      try {
        const response = await digApi.getDig(digName.value)
        dig.value = response
        
        // 如果有测试报告，自动加载
        if (response.status && response.status.testReportName) {
          fetchTestReport()
        }
      } catch (err) {
        error.value = '获取Dig详情失败: ' + (err.message || '未知错误')
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
        const response = await digApi.getDig(digName.value)
        dig.value = response
      } catch (err) {
        error.value = '刷新数据失败: ' + (err.message || '未知错误')
        console.error(err)
      } finally {
        refreshing.value = false
      }
    }

    // 获取TestReport
    const fetchTestReport = async () => {
      if (!dig.value || !dig.value.status.testReportName) return
      
      loadingTestReport.value = true
      testReportError.value = null
      
      try {
        const response = await digApi.getDigTestReport(dig.value.status.testReportName)
        testReport.value = response
      } catch (err) {
        testReportError.value = '获取测试报告失败: ' + (err.message || '未知错误')
        console.error(err)
      } finally {
        loadingTestReport.value = false
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
        .replace(/Dns/g, 'DNS')
        .replace(/Tcp/g, 'TCP')
        .replace(/Ipv4/g, 'IPv4')
        .replace(/Ipv6/g, 'IPv6')
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
      if (status.includes('success') || status === 'completed') return 'status-success'
      if (status.includes('fail') || status.includes('error')) return 'status-error'
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

    // 定时刷新数据
    const startRefreshInterval = () => {
      // 每15秒刷新一次数据
      refreshInterval.value = setInterval(() => {
        refreshData()
        
        // 如果有测试报告，也刷新测试报告
        if (dig.value && dig.value.status && dig.value.status.testReportName) {
          fetchTestReport()
        }
      }, 15000)
      
      // 启动进度条动画
      startProgressAnimation()
    }
    
    // 进度条动画
    const startProgressAnimation = () => {
      let direction = 1 // 1表示增加，-1表示减少
      let animationInterval = setInterval(() => {
        if (isRunning.value) {
          refreshProgress.value += direction
          
          if (refreshProgress.value >= 100) {
            direction = -1
          } else if (refreshProgress.value <= 0) {
            direction = 1
          }
        } else {
          refreshProgress.value = 100
          clearInterval(animationInterval)
        }
      }, 30)
    }

    onMounted(() => {
      fetchDigData()
      startRefreshInterval()
    })

    onUnmounted(() => {
      if (refreshInterval.value) {
        clearInterval(refreshInterval.value)
      }
    })

    return {
      dig,
      loading,
      error,
      refreshing,
      showConfigDetails,
      testReport,
      loadingTestReport,
      testReportError,
      configDetails,
      isRunning,
      progressWidth,
      fetchDigData,
      refreshData,
      fetchTestReport,
      formatConfigKey,
      formatConfigValue,
      getStatusClass,
      formatDate
    }
  }
}
</script>

<style scoped>
.header-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1.5rem;
}

.loading-container, .error-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 3rem;
  text-align: center;
}

.loading-container .spinner {
  margin-bottom: 1rem;
}

.loading-container-small {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 2rem;
}

.loading-container-small .spinner {
  width: 24px;
  height: 24px;
  margin-bottom: 0.5rem;
}

.card {
  margin-bottom: 1.5rem;
}

.status-card {
  background-color: #f9f9f9;
  border-left: 4px solid var(--primary-color);
}

.status-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
  padding-bottom: 1rem;
  border-bottom: 1px solid #eee;
}

.status-details {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 1rem;
  margin-bottom: 1rem;
}

.status-item {
  display: flex;
  flex-direction: column;
}

.status-label {
  font-weight: 500;
  color: var(--text-secondary);
  font-size: 0.9rem;
  margin-bottom: 0.25rem;
}

.status-value {
  font-size: 1.1rem;
}

.progress-section {
  margin-top: 1rem;
  padding-top: 1rem;
  border-top: 1px solid #eee;
}

.status-badge {
  display: inline-block;
  padding: 0.25rem 0.75rem;
  border-radius: 16px;
  font-size: 0.9rem;
  font-weight: 500;
}

.status-success {
  background-color: var(--success-color);
  color: white;
}

.status-error {
  background-color: var(--danger-color);
  color: white;
}

.status-running {
  background-color: var(--secondary-color);
  color: white;
}

.status-pending {
  background-color: var(--warning-color);
  color: var(--text-primary);
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 1.5rem;
}

.stat-item {
  text-align: center;
  padding: 1rem;
  background-color: #f9f9f9;
  border-radius: 8px;
}

.stat-value {
  font-size: 1.8rem;
  font-weight: 600;
  margin-bottom: 0.5rem;
}

.success {
  color: var(--success-color);
}

.danger {
  color: var(--danger-color);
}

.stat-label {
  color: var(--text-secondary);
  font-size: 0.9rem;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
  padding-bottom: 0.5rem;
  border-bottom: 1px solid #eee;
}

.small-btn {
  padding: 0.25rem 0.5rem;
  font-size: 0.9rem;
}

.config-details {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 0.75rem;
}

.config-row {
  display: flex;
  padding: 0.5rem;
  border-bottom: 1px solid #f5f5f5;
}

.config-row:last-child {
  border-bottom: none;
}

.config-key {
  font-weight: 500;
  width: 40%;
  color: var(--text-secondary);
}

.config-value {
  width: 60%;
  word-break: break-word;
}

.result-content {
  background-color: #f5f5f5;
  padding: 1rem;
  border-radius: 4px;
  overflow-x: auto;
}

pre {
  margin: 0;
  white-space: pre-wrap;
  word-wrap: break-word;
  font-family: monospace;
}

.empty-result {
  padding: 2rem;
  text-align: center;
  color: var(--text-secondary);
}

.summary-stats {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 1rem;
  margin: 1rem 0;
}

.summary-stat {
  display: flex;
  justify-content: space-between;
  padding: 0.5rem;
  background-color: #f9f9f9;
  border-radius: 4px;
}

.results-list {
  margin-top: 1rem;
}

.result-item {
  margin-bottom: 1rem;
  padding: 1rem;
}

.result-header {
  display: flex;
  justify-content: space-between;
  margin-bottom: 0.75rem;
}

.result-time {
  color: var(--text-secondary);
  font-size: 0.9rem;
}

.result-details {
  margin-bottom: 0.75rem;
}

.result-metric {
  display: flex;
  margin-bottom: 0.5rem;
}

.result-metric-label {
  font-weight: 500;
  width: 100px;
  color: var(--text-secondary);
}

.result-error {
  display: flex;
  color: var(--danger-color);
}

.result-error-label {
  font-weight: 500;
  width: 100px;
}

.result-output {
  background-color: #f5f5f5;
  padding: 0.75rem;
  border-radius: 4px;
  margin-top: 0.75rem;
}

.spinner-small {
  border: 2px solid rgba(0, 0, 0, 0.1);
  border-radius: 50%;
  border-top: 2px solid var(--primary-color);
  width: 14px;
  height: 14px;
  display: inline-block;
  margin-right: 5px;
  animation: spin 1s linear infinite;
}

.error-message {
  color: var(--danger-color);
  padding: 1rem;
  background-color: #ffebee;
  border-radius: 4px;
}
</style> 