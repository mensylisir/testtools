<template>
  <div class="dig-list">
    <div class="header-actions">
      <h1>DNS查询工具</h1>
      <router-link to="/dig/create" class="btn btn-primary">创建新的Dig查询</router-link>
    </div>

    <div v-if="loading" class="loading-container">
      <div class="spinner"></div>
      <p>加载中...</p>
    </div>

    <div v-else-if="error" class="error-container card">
      <h3>错误</h3>
      <p class="error-message">{{ error }}</p>
      <button @click="fetchDigs" class="btn btn-primary">重试</button>
    </div>

    <div v-else-if="digs.length === 0" class="empty-container card">
      <h3>暂无Dig查询</h3>
      <p>点击上方按钮创建新的DNS查询任务</p>
    </div>

    <div v-else class="dig-list-container">
      <div class="filter-container">
        <input 
          type="text" 
          v-model="searchQuery" 
          placeholder="搜索域名或状态..." 
          class="form-control"
        />
      </div>

      <div class="card digs-table-container">
        <table class="digs-table">
          <thead>
            <tr>
              <th>名称</th>
              <th>域名</th>
              <th>查询类型</th>
              <th>状态</th>
              <th>查询次数</th>
              <th>成功/失败</th>
              <th>平均响应时间</th>
              <th>最后执行时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="dig in filteredDigs" :key="dig.metadata.name">
              <td>{{ dig.metadata.name }}</td>
              <td>{{ dig.spec.domain }}</td>
              <td>{{ dig.spec.queryType || 'A' }}</td>
              <td>
                <span :class="'status-badge ' + getStatusClass(dig.status.status)">
                  {{ dig.status.status || '等待中' }}
                </span>
              </td>
              <td>{{ dig.status.queryCount || 0 }}</td>
              <td>{{ (dig.status.successCount || 0) + '/' + (dig.status.failureCount || 0) }}</td>
              <td>{{ dig.status.averageResponseTime || '-' }}</td>
              <td>{{ formatDate(dig.status.lastExecutionTime) }}</td>
              <td class="actions">
                <router-link :to="`/dig/${dig.metadata.name}`" class="btn btn-secondary">详情</router-link>
                <button @click="confirmDelete(dig)" class="btn btn-danger">删除</button>
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
        <p>确定要删除 <strong>{{ digToDelete?.metadata.name }}</strong> 吗？此操作不可逆。</p>
        <div class="modal-actions">
          <button @click="showDeleteConfirm = false" class="btn">取消</button>
          <button @click="deleteDig" class="btn btn-danger" :disabled="deleting">
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
import kubernetesApi from '../api/dig.js'

export default {
  setup() {
    const digs = ref([])
    const loading = ref(true)
    const error = ref(null)
    const searchQuery = ref('')
    const showDeleteConfirm = ref(false)
    const digToDelete = ref(null)
    const deleting = ref(false)

    const fetchDigs = async () => {
      loading.value = true
      error.value = null
      try {
        const response = await kubernetesApi.getDigList()
        digs.value = response.items || []
      } catch (err) {
        error.value = '获取Dig列表失败: ' + (err.message || '未知错误')
        console.error(err)
      } finally {
        loading.value = false
      }
    }

    const filteredDigs = computed(() => {
      if (!searchQuery.value) return digs.value
      
      const query = searchQuery.value.toLowerCase()
      return digs.value.filter(dig => 
        dig.metadata.name.toLowerCase().includes(query) ||
        dig.spec.domain.toLowerCase().includes(query) ||
        (dig.status.status && dig.status.status.toLowerCase().includes(query))
      )
    })

    const confirmDelete = (dig) => {
      digToDelete.value = dig
      showDeleteConfirm.value = true
    }

    const deleteDig = async () => {
      if (!digToDelete.value) return
      
      deleting.value = true
      try {
        await kubernetesApi.deleteDig(digToDelete.value.metadata.name)
        // 从列表中移除
        digs.value = digs.value.filter(d => d.metadata.name !== digToDelete.value.metadata.name)
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
      if (status.includes('success') || status === 'completed') return 'status-success'
      if (status.includes('fail') || status.includes('error')) return 'status-error'
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

    onMounted(fetchDigs)

    return {
      digs,
      loading,
      error,
      searchQuery,
      filteredDigs,
      showDeleteConfirm,
      digToDelete,
      deleting,
      fetchDigs,
      confirmDelete,
      deleteDig,
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
}

.digs-table-container {
  overflow-x: auto;
}

.digs-table {
  width: 100%;
  border-collapse: collapse;
}

.digs-table th, .digs-table td {
  padding: 0.75rem;
  text-align: left;
  border-bottom: 1px solid #eee;
}

.digs-table th {
  font-weight: 600;
  background-color: #f9f9f9;
}

.digs-table tr:hover {
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

.actions {
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
  width: 90%;
  max-width: 500px;
  padding: 1.5rem;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
  margin-top: 1rem;
}
</style> 