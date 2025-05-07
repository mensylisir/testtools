<template>
  <div class="ping-list">
    <div class="header-actions">
      <h1>连通性测试工具</h1>
      <router-link to="/ping/create" class="btn btn-primary">创建新的Ping测试</router-link>
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
      <button @click="fetchPings" class="btn btn-primary">重试</button>
    </div>

    <div v-else-if="pings.length === 0" class="empty-container card">
      <h3>暂无Ping查询</h3>
      <p>点击上方按钮创建新的Ping查询任务</p>
    </div>

    <!-- 结果列表 -->
    <div v-else class="ping-list-container">
      <div class="filter-container">
        <input
            type="text"
            v-model="searchQuery"
            placeholder="搜索域名或状态..."
            class="form-control"
        />
      </div>

      <div class="card pings-table-container">
        <table class="pings-table">
          <thead>
            <tr>
              <th>名称</th>
              <th>目标地址</th>
              <th>状态</th>
              <th>查询次数</th>
              <th>成功/失败</th>
              <th>最后执行时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="ping in filteredPings" :key="ping.metadata.name">
              <td>{{ ping.metadata.name }}</td>
              <td>{{ ping.spec.host }}</td>
              <td>
                <span class="status-badge" :class="getStatusClass(ping.status?.status)">
                  {{ ping.status?.status || '等待中' }}
                </span>
              </td>
              <td>{{ ping.status.queryCount || 0 }}</td>
              <td>{{ (ping.status.successCount || 0) + '/' + (ping.status.failureCount || 0) }}</td>
              <td>{{ formatDate(ping.status.lastExecutionTime) }}</td>
              <td>
                <div class="action">
                  <router-link :to="`/ping/${ping.metadata.name}`" class="btn btn-secondary">详情</router-link>
                  <button @click="confirmDelete(ping)" class="btn btn-danger">删除</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 删除确认对话框 -->
    <div v-if="showDeleteConfirm" class="modal-overlay">
      <div class="modal-container card">
        <h3>确认删除</h3>
        <p>确定要删除 <strong>{{ pingToDelete?.metadata.name }}</strong> 吗？此操作不可逆。</p>
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
import { ref, computed, onMounted } from 'vue'
import kubernetesApi from '../api/ping.js'

export default {
  setup() {
    const pings = ref([])
    const loading = ref(true)
    const error = ref(null)
    const searchQuery = ref('')
    const showDeleteConfirm = ref(false)
    const pingToDelete = ref(null)
    const deleting = ref(false)

    const fetchPings = async () => {
      loading.value = true
      error.value = null
      try {
        const response = await kubernetesApi.getPingList()
        pings.value = response.items || []
      } catch (err) {
        error.value = '获取连通性测试列表失败: ' + (err.message || '未知错误')
        console.error(err)
      } finally {
        loading.value = false
      }
    }

    const filteredPings = computed(() => {
      if (!searchQuery.value) return pings.value

      const query = searchQuery.value.toLowerCase()
      return pings.value.filter(ping =>
        ping.metadata.name.toLowerCase().includes(query) ||
        ping.spec.target.toLowerCase().includes(query) ||
        (ping.status.status && ping.status.status.toLowerCase().includes(query))
      )
    })

    const confirmDelete = (ping) => {
      pingToDelete.value = ping
      showDeleteConfirm.value = true
    }

    const deletePing = async () => {
      if (!pingToDelete.value) return

      deleting.value = true
      try {
        await kubernetesApi.deletePing(pingToDelete.value.metadata.name)
        // 从列表中移除
        pings.value = pings.value.filter(d => d.metadata.name !== pingToDelete.value.metadata.name)
        showDeleteConfirm.value = false
      } catch (err) {
        error.value = '删除失败: ' + (err.message || '未知错误')
        console.error(err)
      } finally {
        deleting.value = false
      }
    }

    const getStatusClass = (status) => {
      if (!status) return 'status-pending'

      status = status.toLowerCase()
      if (status.includes('success') || status.includes('succeeded') || status === 'completed') return 'status-success'
      if (status.includes('fail') || status.includes('failed') || status.includes('error')) return 'status-error'
      if (status.includes('progress') || status === 'running') return 'status-running'

      return 'status-pending'
    }

    const formatDate = (dateString) => {
      if (!dateString) return '-'

      try {
        const date = new Date(dateString)
        return date.toLocaleString()
      } catch (e) {
        return dateString
      }
    }

    onMounted(fetchPings)

    return {
      pings,
      loading,
      error,
      searchQuery,
      filteredPings,
      showDeleteConfirm,
      pingToDelete,
      deleting,
      fetchPings,
      confirmDelete,
      deletePing,
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
  color: var(--text-primary);
}

.header-actions h1 {
  font-size: 1.8rem;
  margin: 0;
}

.loading-container, .error-container, .empty-container {
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

.filter-container {
  margin-bottom: 1rem;
  padding: 10px 20px 10px 0px;
}

.pings-table-container {
  overflow-x: auto;
}

.pings-table {
  width: 100%;
  border-collapse: collapse;
}

.pings-table th, .pings-table td {
  padding: 0.75rem;
  text-align: left;
  border-bottom: 1px solid #eee;
}

.pings-table th {
  font-weight: 600;
  background-color: #f9f9f9;
}

.pings-table tr:hover {
  background-color: #f5f5f5;
}

.status-badge {
  display: inline-block;
  padding: 0.25rem 0.5rem;
  border-radius: 12px;
  font-size: 0.8rem;
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

.action {
  display: flex;
  gap: 0.5rem;
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
  width: 400px;
  padding: 1.5rem;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
  margin-top: 1rem;
}
</style>