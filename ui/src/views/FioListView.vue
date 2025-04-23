<template>
  <div class="fio-list-view">
    <div class="header-actions">
      <h1>IO性能测试列表</h1>
        <router-link to="/fio/create" class="btn btn-primary">创建新测试</router-link>
    </div>

    <div v-if="loading" class="loading-container">
      <div class="spinner"></div>
      <p>加载中...</p>
    </div>

    <!-- 错误显示 -->
    <div v-else-if="error" class="error-container card">
      <p class="error-message">{{ error }}</p>
      <button @click="fetchFios" class="btn btn-primary">重试</button>
    </div>

    <!-- 空结果显示 -->
    <div v-else-if="filteredFios.length === 0" class="empty-container card">
      <h3>暂无Fio测试</h3>
      <p>点击上方按钮创建新的IO测试任务</p>
    </div>

    <!-- 结果列表 -->
    <div v-else class="fios-list-container">

      <div class="filter-container">
        <input
            type="text"
            v-model="searchQuery"
            placeholder="搜索域名或状态..."
            class="form-control"
        />
      </div>
      <div class="card fios-table-container">
        <table class="fios-table">
        <thead>
          <tr>
            <th>名称</th>
            <th>测试路径</th>
            <th>测试类型</th>
            <th>状态</th>
            <th>查询次数</th>
            <th>成功/失败</th>
            <th>最后执行时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="fio in filteredFios" :key="fio.metadata.name">
            <td>{{ fio.metadata.name }}</td>
            <td>{{ fio.spec.filePath || '-' }}</td>
            <td>{{ getReadWriteMode(fio.spec.readWrite) || '-' }}</td>
            <td>
              <span :class="'status-badge ' + getStatusClass(fio.status.status)">
                {{ fio.status.status || '等待中' }}
              </span>
            </td>
            <td>{{ fio.status?.queryCount || 0 }}</td>
            <td>{{ (fio.status?.successCount || 0) + '/' + (fio.status?.failureCount || 0) }}</td>
            <td>{{ formatDate(fio.metadata.creationTimestamp) }}</td>
            <td class="actions">
              <router-link :to="`/fio/${fio.metadata.name}`" class="btn btn-secondary">详情</router-link>
              <button @click="confirmDelete(fio)" class="btn btn-danger">删除</button>
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
        <p>确定要删除 <strong>{{ fioToDelete?.metadata.name }}</strong> 吗？此操作不可逆。</p>
        <div class="modal-actions">
          <button @click="showDeleteConfirm = false" class="btn">取消</button>
          <button @click="deleteFio" class="btn btn-danger" :disabled="deleting">
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
import kubernetesApi from '../api/fio.js'

export default {
  setup() {
    const fios = ref([])
    const loading = ref(true)
    const error = ref(null)
    const searchQuery = ref('')
    const showDeleteConfirm = ref(false)
    const fioToDelete = ref(null)
    const deleting = ref(false)

    const fetchFios = async () => {
      loading.value = true
      error.value = null
      try {
        const response = await kubernetesApi.getFioList()
        fios.value = response.items || []
      } catch (err) {
        error.value = '获取IO性能测试列表失败: ' + (err.message || '未知错误')
        console.error(err)
      } finally {
        loading.value = false
      }
    }

    const filteredFios = computed(() => {
      if (!searchQuery.value) return fios.value

      const query = searchQuery.value.toLowerCase()
      return fios.value.filter(fio =>
        fio.metadata.name.toLowerCase().includes(query) ||
        (fio.spec.filename && fio.spec.filename.toLowerCase().includes(query)) ||
        (fio.spec.readwrite && fio.spec.readwrite.toLowerCase().includes(query)) ||
        (fio.status && fio.status.status && fio.status.status.toLowerCase().includes(query))
      )
    })

    const confirmDelete = (fio) => {
      fioToDelete.value = fio
      showDeleteConfirm.value = true
    }

    const deleteFio = async () => {
      if (!fioToDelete.value) return

      deleting.value = true
      try {
        await kubernetesApi.deleteFio(fioToDelete.value.metadata.name)
        // 从列表中移除
        fios.value = fios.value.filter(f => f.metadata.name !== fioToDelete.value.metadata.name)
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

    const getReadWriteMode = (mode) => {
      if (!mode) return '-'

      const modes = {
        'read': '顺序读取',
        'write': '顺序写入',
        'randread': '随机读取',
        'randwrite': '随机写入',
        'readwrite': '混合读写',
        'randrw': '随机混合读写'
      }

      return modes[mode] || mode
    }

    onMounted(fetchFios)

    return {
      fios,
      loading,
      error,
      searchQuery,
      filteredFios,
      showDeleteConfirm,
      fioToDelete,
      deleting,
      fetchFios,
      confirmDelete,
      deleteFio,
      getStatusClass,
      formatDate,
      getReadWriteMode,
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

.fios-table-container {
  overflow-x: auto;
}

.fios-table {
  width: 100%;
  border-collapse: collapse;
}

.fios-table th, .fios-table td {
  padding: 0.75rem;
  text-align: left;
  border-bottom: 1px solid #eee;
}

.fios-table th {
  font-weight: 600;
  background-color: #f9f9f9;
}

.fios-table tr:hover {
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