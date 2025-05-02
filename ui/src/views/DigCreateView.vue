 <template>
  <div class="dig-create">
    <div class="header-actions">
      <h1>创建DNS查询</h1>
      <router-link to="/dig" class="btn btn-secondary">返回查询列表</router-link>
    </div>

    <div class="card">
      <form @submit.prevent="createDig">
        <div class="form-group">
          <label for="name">查询名称 *</label>
          <input 
            type="text" 
            id="name" 
            v-model="digForm.name" 
            class="form-control" 
            required
            placeholder="输入一个唯一的名称，例如：example-com-dns"
          />
          <small>只能使用小写字母、数字和连字符，且必须以字母开头</small>
        </div>

        <div class="form-group">
          <label for="domain">域名 *</label>
          <input 
            type="text" 
            id="domain" 
            v-model="digForm.spec.domain" 
            class="form-control" 
            required
            placeholder="要查询的域名，例如：example.com"
          />
        </div>

        <div class="form-row">
          <div class="form-group">
            <label for="queryType">查询类型</label>
            <select id="queryType" v-model="digForm.spec.queryType" class="form-control">
              <option value="A">A - IPv4地址</option>
              <option value="AAAA">AAAA - IPv6地址</option>
              <option value="CNAME">CNAME - 别名记录</option>
              <option value="MX">MX - 邮件交换</option>
              <option value="NS">NS - 名称服务器</option>
              <option value="TXT">TXT - 文本记录</option>
              <option value="SOA">SOA - 权威记录</option>
              <option value="SRV">SRV - 服务记录</option>
              <option value="PTR">PTR - 指针记录</option>
              <option value="CAA">CAA - CA授权记录</option>
            </select>
          </div>

          <div class="form-group">
            <label for="server">DNS服务器</label>
            <input 
              type="text" 
              id="server" 
              v-model="digForm.spec.server" 
              class="form-control"
              placeholder="可选，例如：8.8.8.8"
            />
          </div>
        </div>

        <div class="form-row">
          <div class="form-group">
            <label for="timeout">超时时间 (秒)</label>
            <input 
              type="number" 
              id="timeout" 
              v-model.number="digForm.spec.timeout" 
              min="1" 
              max="60" 
              class="form-control"
            />
          </div>

          <div class="form-group">
            <label for="port">查询端口</label>
            <input 
              type="number" 
              id="port" 
              v-model.number="digForm.spec.port" 
              min="1" 
              max="65535" 
              class="form-control"
              placeholder="可选，默认为53"
            />
          </div>
        </div>

        <div class="form-row">
          <div class="form-group">
            <label for="schedule">定时计划 (Cron格式)</label>
            <input 
              type="text" 
              id="schedule" 
              v-model="digForm.spec.schedule" 
              class="form-control"
              placeholder="可选，例如：*/5 * * * * (每5分钟执行一次)"
            />
            <small>留空表示只执行一次</small>
          </div>

          <div class="form-group">
            <label for="maxRetries">最大重试次数</label>
            <input 
              type="number" 
              id="maxRetries" 
              v-model.number="digForm.spec.maxRetries" 
              min="1" 
              max="10" 
              class="form-control"
            />
          </div>
        </div>

        <div class="form-row options-row">
          <div class="form-group checkbox-group">
            <label class="checkbox-label">
              <input type="checkbox" v-model="digForm.spec.useTCP" />
              <span>使用TCP协议</span>
            </label>
          </div>

          <div class="form-group checkbox-group">
            <label class="checkbox-label">
              <input type="checkbox" v-model="digForm.spec.useIPv4Only" />
              <span>仅使用IPv4</span>
            </label>
          </div>

          <div class="form-group checkbox-group">
            <label class="checkbox-label">
              <input type="checkbox" v-model="digForm.spec.useIPv6Only" />
              <span>仅使用IPv6</span>
            </label>
          </div>
        </div>

        <div class="form-row">
          <div class="form-group">
            <label for="nodeName">节点名称</label>
            <input 
              type="text" 
              id="nodeName" 
              v-model="digForm.spec.nodeName" 
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
            <span v-else>创建DNS查询</span>
          </button>
        </div>
      </form>
    </div>

    <!-- 创建结果提示 -->
    <div v-if="successMessage" class="success-message card">
      <h3>创建成功</h3>
      <p>{{ successMessage }}</p>
      <div class="success-actions">
        <router-link :to="'/dig/' + createdDigName" class="btn btn-primary">查看详情</router-link>
        <router-link to="/dig" class="btn btn-secondary">返回列表</router-link>
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
import { ref, computed, reactive } from 'vue'
import { useRouter } from 'vue-router'
import digApi from '../api/dig.js'

export default {
  setup() {
    const router = useRouter()
    const creating = ref(false)
    const error = ref(null)
    const successMessage = ref(null)
    const createdDigName = ref('')

    // 初始化表单数据
    const digForm = reactive({
      name: '',
      spec: {
        domain: '',
        queryType: 'A',
        server: '',
        timeout: 5,
        port: null,
        schedule: '',
        maxRetries: 3,
        useTCP: false,
        useIPv4Only: false,
        useIPv6Only: false,
        nodeName: ''
      }
    })

    // 表单验证
    const isFormValid = computed(() => {
      // 名称必须是小写字母、数字和连字符，且必须以字母开头
      const namePattern = /^[a-z][a-z0-9-]*$/
      const isNameValid = namePattern.test(digForm.name)
      
      // 域名必须非空
      const isDomainValid = !!digForm.spec.domain.trim()
      
      // IPv4和IPv6选项不能同时选中
      const isIPVersionValid = !(digForm.spec.useIPv4Only && digForm.spec.useIPv6Only)
      
      return isNameValid && isDomainValid && isIPVersionValid
    })

    // 创建Dig资源
    const createDig = async () => {
      if (!isFormValid.value) return
      
      creating.value = true
      error.value = null
      successMessage.value = null
      
      try {
        const cleanedSpec = { ...digForm.spec }
        
        // 移除空值
        Object.keys(cleanedSpec).forEach(key => {
          if (cleanedSpec[key] === '' || cleanedSpec[key] === null) {
            delete cleanedSpec[key]
          }
        })
        
        const response = await digApi.createDig({
          name: digForm.name,
          spec: cleanedSpec
        })
        
        createdDigName.value = response.metadata.name
        successMessage.value = `成功创建DNS查询: ${response.metadata.name}`
        resetForm()
      } catch (err) {
        error.value = '创建失败: ' + (err.message || '未知错误')
        console.error(err)
      } finally {
        creating.value = false
      }
    }

    // 重置表单
    const resetForm = () => {
      digForm.name = ''
      digForm.spec.domain = ''
      digForm.spec.queryType = 'A'
      digForm.spec.server = ''
      digForm.spec.timeout = 5
      digForm.spec.port = null
      digForm.spec.schedule = ''
      digForm.spec.maxRetries = 3
      digForm.spec.useTCP = false
      digForm.spec.useIPv4Only = false
      digForm.spec.useIPv6Only = false
      digForm.spec.nodeName = ''
    }

    return {
      digForm,
      creating,
      error,
      successMessage,
      createdDigName,
      isFormValid,
      createDig,
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
}

.form-group {
  margin-bottom: 1.5rem;
}

.form-row {
  display: flex;
  gap: 1rem;
  margin-bottom: 0;
}

.form-row .form-group {
  flex: 1;
}

label {
  display: block;
  margin-bottom: 0.5rem;
  font-weight: 500;
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
}

.error-message {
  background-color: #ffebee;
  border-left: 4px solid var(--danger-color);
}

.success-actions {
  display: flex;
  gap: 1rem;
  margin-top: 1rem;
}

.spinner-small {
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-radius: 50%;
  border-top: 2px solid white;
  width: 16px;
  height: 16px;
  display: inline-block;
  margin-right: 8px;
  animation: spin 1s linear infinite;
}

.options-row {
  gap: 2rem;
}
</style> 