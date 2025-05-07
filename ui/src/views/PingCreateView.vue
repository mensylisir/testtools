<template>
  <div class="ping-create">
    <div class="header-actions">
      <h1>创建连通性测试</h1>
      <router-link to="/ping" class="btn btn-secondary">返回查询列表</router-link>
    </div>

    <div class="card">
      <form @submit.prevent="createPing">
        <div class="form-group">
          <label for="name">名称 *</label>
          <input
              type="text"
              id="name"
              v-model="pingForm.name"
              class="form-control"
              required
              placeholder="输入一个唯一的名称,例如:example-com"
          />
          <small>只能使用小写字母、数字和连字符，且必须以字母开头</small>
        </div>
        <div class="form-group">
          <label for="target">目标主机/IP地址 *</label>
          <input
              type="text"
              id="target"
              v-model="pingForm.spec.host"
              class="form-control"
              required
              placeholder="输入要测试的主机名、域名或IP地址"
          />
        </div>

        <div class="form-row">
          <div class="form-group">
            <label for="count">请求次数</label>
            <input
                type="number"
                id="count"
                v-model.number="pingForm.spec.count"
                min="1"
                max="100"
                class="form-control"
            />
            <small>发送的ICMP请求数量 (1-100)</small>
          </div>

          <div class="form-group">
            <label for="interval">间隔</label>
            <input
                type="number"
                id="interval"
                v-model.number="pingForm.spec.interval"
                min="0.1"
                step="0.1"
                class="form-control"
            />
            <small>每次请求的间隔时间 (秒)</small>
          </div>
        </div>

        <div class="form-row">
          <div class="form-group">
            <label for="timeout">超时</label>
            <input
                type="number"
                id="timeout"
                v-model.number="pingForm.spec.timeout"
                min="1"
                max="60"
                class="form-control"
            />
            <small>单个请求的超时时间 (秒)</small>
          </div>

          <div class="form-group">
            <label for="size">数据包大小</label>
            <input
                type="number"
                id="size"
                v-model.number="pingForm.spec.packetSize"
                min="16"
                max="65507"
                class="form-control"
            />
            <small>ICMP数据包大小 (16-65507 字节)</small>
          </div>
        </div>

        <div class="form-row">
          <div class="form-group">
            <label for="nodeName">节点名称</label>
            <input
                type="text"
                id="nodeName"
                v-model="pingForm.spec.nodeName"
                class="form-control"
                placeholder="可选，指定在哪个节点上运行"
            />
          </div>
        </div>

        <div class="form-actions">
          <button
              type="button"
              @click="resetForm"
              class="btn btn-secondary"
              :disabled="creating"
          >
            重置
          </button>
          <button
              type="submit"
              class="btn btn-primary"
              :disabled="creating || !isFormValid"
          >
            <span v-if="creating">
              <div class="spinner-small"></div>
              创建中...
            </span>
            <span v-else>创建连通性测试</span>
          </button>
        </div>
      </form>
    </div>

    <div v-if="successMessage" class="success-message card">
      <h3>创建成功</h3>
      <p> {{ successMessage }}</p>
      <div class="success-actions">
        <router-link :to="'/ping/' + createdPingName" class="btn btn-primary">查看详情</router-link>
        <router-link to="/ping" class="btn btn-secondary">返回列表</router-link>
      </div>
    </div>

    <div v-if="error" class="error-message card">
      <h3>创建失败</h3>
      <p>{{ error }}</p>
      <button @click="error = null" class="btn btn-secondary">关闭</button>
    </div>
  </div>
</template>

<script>
import {ref, reactive, computed} from 'vue'
import { useRouter } from 'vue-router'
import pingApi from '../api/ping.js'

export default {
  setup() {
    const router = useRouter()
    const creating = ref(false)
    const error = ref(null)
    const successMessage = ref(null)
    const createdPingName = ref('')

    // 表单数据
    const pingForm = reactive({
      name: '',
      spec: {
        target: '',
        count: 5,
        interval: 1,
        timeout: 5,
        size: 56,
        nodeName: ''
      }
    })

    const isFormValid = computed( () => {

      const namePattern = /^[a-z][a-z0-9-]*$/
      const isNameValid = namePattern.test(pingForm.name)

      const target = pingForm.spec.target.trim();
      const domainRegex = /^(?!:\/\/)([a-zA-Z0-9-_]+\.)*[a-zA-Z0-9][a-zA-Z0-9-_]+\.[a-zA-Z]{2,}$/;
      const ipv4Regex = /^(25[0-5]|2[0-4]\d|1\d{2}|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d{2}|[1-9]?\d)){3}$/;
      const ipv6Regex = /^(([0-9a-fA-F]{1,4}):){7}([0-9a-fA-F]{1,4})$/;
      const isDomainValid = !!target && (domainRegex.test(target) || ipv4Regex.test(target) || ipv6Regex.test(target));

      return isNameValid && isDomainValid
    });


    const createPing = async () => {
      if (!isFormValid.value) return
      creating.value = true
      error.value = null
      successMessage.value = null

      try {
        const cleanedSpec = { ...pingForm.spec }
        Object.keys(cleanedSpec).forEach((key) => {
          if (cleanedSpec[key] === '' || cleanedSpec[key] === null) {
            delete cleanedSpec[key]
          }
        })

        const response = await pingApi.createPing({
          name: pingForm.name,
          spec: cleanedSpec,
        })

        createdPingName.value = response.metadata.name
        successMessage.value = `成功创建Ping测试: ${response.metadata.name}`
        resetForm()
      } catch (err) {
        error.value = '创建失败：' + (err.message || '未知错误')
        console.error(err)
      } finally {
        creating.value = false
      }
    }

    const resetForm = () => {
      pingForm.name = ''
      pingForm.spec.target = ''
      pingForm.spec.count = ''
      pingForm.spec.interval = ''
      pingForm.spec.timeout = ''
      pingForm.spec.size = ''
      pingForm.spec.nodeName = ''
    }

    return {
      pingForm,
      creating,
      error,
      successMessage,
      createdPingName,
      isFormValid,
      createPing,
      resetForm
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

.form-group {
  margin-bottom: 1.5rem;
}

.form-row {
  display: flex;
  gap: 2rem;
  margin-bottom: 0;
  flex-wrap: wrap;
}

.form-row .form-group {
  flex: 1;
  min-width: 250px;
}

label {
  display: block;
  margin-bottom: 0.5rem;
  font-weight: 500;
  color: var(--text-primary);
}

small {
  display: block;
  margin-top: 0.25rem;
  color: var(--text-secondary);
  font-size: 0.8rem;
}

.checkbox-group {
  margin-top: 1rem;
}

.checkbox-label {
  display: flex;
  align-items: center;
  cursor: pointer;
}

.checkbox-label input {
  margin-right: 0.5rem;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 1rem;
  margin-top: 2rem;
}

.success-message, .error-message {
  margin-top: 1.5rem;
  padding: 1.5rem;
}

.success-message {
  background-color: #e8f5e9;
  border-left: 4px solid var(--success-color);
  color: var(--text-primary);
}

.success-message h3 {
  margin-top: 0;
  color: var(--success-color);
}

.error-message {
  background-color: #ffebee;
  border-left: 4px solid var(--danger-color);
  color: var(--text-primary);
}

.error-message h3 {
  margin-top: 0;
  color: var(--danger-color);
}

.success-actions {
  display: flex;
  gap: 1rem;
  margin-top: 1rem;
}

.spinner-small {
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top: 2px solid var(--primary-color);
  border-radius: 50%;
  width: 16px;
  height: 16px;
  display: inline-block;
  margin-right: 8px;
  vertical-align: middle;
  animation: spin 1s linear infinite;
}

.options-row {
  gap: 2rem;
}
</style> 