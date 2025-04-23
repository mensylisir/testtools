<template>
  <div class="ping-create-view">
    <h1>创建连通性测试</h1>
    
    <div v-if="loading" class="loading-container">
      <div class="spinner"></div>
      <p>创建中...</p>
    </div>

    <div v-else-if="success" class="success-container card">
      <h3>创建成功!</h3>
      <p>连通性测试 <strong>{{ form.name }}</strong> 已成功创建</p>
      <div class="success-actions">
        <router-link :to="`/ping/${form.name}`" class="btn btn-primary">
          查看详情
        </router-link>
        <router-link to="/ping" class="btn">
          返回列表
        </router-link>
      </div>
    </div>

    <div v-else-if="error" class="error-container card">
      <h3>创建失败</h3>
      <p class="error-message">{{ error }}</p>
      <button @click="error = null" class="btn">返回编辑</button>
    </div>

    <form v-else @submit.prevent="submitForm" class="ping-form card">
      <div class="form-group">
        <label for="name">名称 <span class="required">*</span></label>
        <input 
          type="text" 
          id="name" 
          v-model="form.name" 
          placeholder="输入测试名称" 
          required
          :class="{ 'input-error': errors.name }"
        />
        <div v-if="errors.name" class="error-help">{{ errors.name }}</div>
        <div class="form-help">名称只能包含小写字母、数字和'-'，且必须以字母或数字开头和结尾</div>
      </div>

      <div class="form-group">
        <label for="target">目标主机/IP地址 <span class="required">*</span></label>
        <input 
          type="text" 
          id="target" 
          v-model="form.spec.host"
          placeholder="输入主机名或IP地址" 
          required
          :class="{ 'input-error': errors.host }"
        />
        <div v-if="errors.target" class="error-help">{{ errors.host }}</div>
      </div>

      <div class="form-group">
        <label for="count">请求次数</label>
        <input 
          type="number" 
          id="count" 
          v-model.number="form.spec.count" 
          min="1" 
          max="100"
        />
        <div class="form-help">发送的ICMP请求数量 (1-100)</div>
      </div>

      <div class="form-group">
        <label for="interval">间隔</label>
        <input 
          type="number" 
          id="interval" 
          v-model.number="form.spec.interval" 
          min="0.1" 
          step="0.1"
        />
        <div class="form-help">每次请求的间隔时间 (秒)</div>
      </div>

      <div class="form-group">
        <label for="timeout">超时</label>
        <input 
          type="number" 
          id="timeout" 
          v-model.number="form.spec.timeout" 
          min="1" 
          max="60"
        />
        <div class="form-help">单个请求的超时时间 (秒)</div>
      </div>

      <div class="form-group">
        <label for="size">数据包大小</label>
        <input 
          type="number" 
          id="size" 
          v-model.number="form.spec.packetSize"
          min="16" 
          max="65507"
        />
        <div class="form-help">ICMP数据包大小 (16-65507 字节)</div>
      </div>

      <div class="form-actions">
        <router-link to="/ping" class="btn">取消</router-link>
        <button type="submit" class="btn btn-primary">创建</button>
      </div>
    </form>
  </div>
</template>

<script>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import pingApi from '../api/ping.js'

export default {
  setup() {
    const router = useRouter()
    const loading = ref(false)
    const success = ref(false)
    const error = ref(null)
    const errors = reactive({})

    // 表单数据
    const form = reactive({
      name: '',
      spec: {
        target: '',
        count: 5,
        interval: 1,
        timeout: 5,
        size: 56
      }
    })

    // 验证表单
    const validateForm = () => {
      const newErrors = {}
      
      // 名称验证 (符合Kubernetes资源命名规则)
      if (!form.name) {
        newErrors.name = '名称不能为空'
      } else if (!/^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(form.name)) {
        newErrors.name = '名称格式不正确'
      }

      // 目标验证
      if (!form.spec.host) {
        newErrors.host = '目标地址不能为空'
      }

      // 将验证结果设置到errors对象中
      Object.keys(errors).forEach(key => delete errors[key])
      Object.keys(newErrors).forEach(key => {
        errors[key] = newErrors[key]
      })

      return Object.keys(newErrors).length === 0
    }

    // 提交表单
    const submitForm = async () => {
      if (!validateForm()) {
        return
      }

      loading.value = true
      error.value = null

      try {
        await pingApi.createPing(form)
        success.value = true
      } catch (err) {
        error.value = '创建连通性测试失败: ' + (err.message || '未知错误')
        console.error(err)
      } finally {
        loading.value = false
      }
    }

    return {
      form,
      loading,
      success,
      error,
      errors,
      submitForm
    }
  }
}
</script>

<style scoped>
.ping-form {
  max-width: 600px;
  margin: 0 auto;
  padding: 1.5rem;
}

.form-group {
  margin-bottom: 1.5rem;
}

label {
  display: block;
  margin-bottom: 0.5rem;
  font-weight: 500;
}

.required {
  color: #d32f2f;
}

input, select, textarea {
  width: 100%;
  padding: 0.5rem;
  border: 1px solid #ddd;
  border-radius: 4px;
  font-size: 1rem;
}

input.input-error, select.input-error, textarea.input-error {
  border-color: #d32f2f;
}

.form-help {
  margin-top: 0.25rem;
  font-size: 0.8rem;
  color: #6c757d;
}

.error-help {
  margin-top: 0.25rem;
  font-size: 0.8rem;
  color: #d32f2f;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
  margin-top: 2rem;
}

.loading-container, .success-container, .error-container {
  max-width: 600px;
  margin: 0 auto;
  padding: 2rem;
  text-align: center;
}

.loading-container .spinner {
  margin-bottom: 1rem;
}

.success-actions, .error-actions {
  display: flex;
  gap: 0.5rem;
  justify-content: center;
  margin-top: 1.5rem;
}
</style> 