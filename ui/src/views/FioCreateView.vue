<template>
  <div class="fio-create-view">
    <h1>创建IO性能测试</h1>
    
    <div v-if="loading" class="loading-container">
      <div class="spinner"></div>
      <p>创建中...</p>
    </div>

    <div v-else-if="success" class="success-container card">
      <h3>创建成功!</h3>
      <p>IO性能测试 <strong>{{ form.name }}</strong> 已成功创建</p>
      <div class="success-actions">
        <router-link :to="`/fio/${form.name}`" class="btn btn-primary">
          查看详情
        </router-link>
        <router-link to="/fio" class="btn">
          返回列表
        </router-link>
      </div>
    </div>

    <div v-else-if="error" class="error-container card">
      <h3>创建失败</h3>
      <p class="error-message">{{ error }}</p>
      <button @click="error = null" class="btn">返回编辑</button>
    </div>

    <form v-else @submit.prevent="submitForm" class="fio-form card">
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
        <label for="filename">测试路径 <span class="required">*</span></label>
        <input 
          type="text" 
          id="filename" 
          v-model="form.spec.filename" 
          placeholder="/path/to/test" 
          required
          :class="{ 'input-error': errors.filename }"
        />
        <div v-if="errors.filename" class="error-help">{{ errors.filename }}</div>
        <div class="form-help">测试IO性能的目标文件或目录路径</div>
      </div>

      <div class="form-group">
        <label for="readwrite">测试类型 <span class="required">*</span></label>
        <select 
          id="readwrite" 
          v-model="form.spec.readwrite" 
          required
          :class="{ 'input-error': errors.readwrite }"
        >
          <option value="read">顺序读取</option>
          <option value="write">顺序写入</option>
          <option value="randread">随机读取</option>
          <option value="randwrite">随机写入</option>
          <option value="readwrite">混合读写</option>
          <option value="randrw">随机混合读写</option>
        </select>
        <div v-if="errors.readwrite" class="error-help">{{ errors.readwrite }}</div>
      </div>

      <div class="form-group">
        <label for="size">测试文件大小</label>
        <div class="size-input-group">
          <input 
            type="number" 
            id="size" 
            v-model.number="form.spec.size" 
            min="1"
          />
          <select v-model="sizeUnit">
            <option value="K">KB</option>
            <option value="M">MB</option>
            <option value="G">GB</option>
          </select>
        </div>
        <div class="form-help">要生成或使用的测试文件大小</div>
      </div>

      <div class="form-group">
        <label for="bs">块大小</label>
        <div class="size-input-group">
          <input 
            type="number" 
            id="bs" 
            v-model.number="form.spec.bs" 
            min="1"
          />
          <select v-model="bsUnit">
            <option value="K">KB</option>
            <option value="M">MB</option>
          </select>
        </div>
        <div class="form-help">I/O操作的块大小</div>
      </div>

      <div class="form-group">
        <label for="iodepth">I/O深度</label>
        <input 
          type="number" 
          id="iodepth" 
          v-model.number="form.spec.iodepth" 
          min="1"
        />
        <div class="form-help">一次提交的I/O请求数量</div>
      </div>

      <div class="form-group">
        <label for="runtime">运行时间 (秒)</label>
        <input 
          type="number" 
          id="runtime" 
          v-model.number="form.spec.runtime" 
          min="1"
          max="3600"
        />
        <div class="form-help">测试运行的最大时间 (1-3600秒)</div>
      </div>

      <div class="form-actions">
        <router-link to="/fio" class="btn">取消</router-link>
        <button type="submit" class="btn btn-primary">创建</button>
      </div>
    </form>
  </div>
</template>

<script>
import { ref, reactive, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import fioApi from '../api/fio.js'

export default {
  setup() {
    const router = useRouter()
    const loading = ref(false)
    const success = ref(false)
    const error = ref(null)
    const errors = reactive({})
    const sizeUnit = ref('M')
    const bsUnit = ref('K')

    // 表单数据
    const form = reactive({
      name: '',
      spec: {
        filename: '/tmp/test',
        readwrite: 'read',
        size: 100,
        bs: 4,
        iodepth: 8,
        runtime: 60
      }
    })

    // 处理单位转换
    watch(sizeUnit, (newUnit, oldUnit) => {
      if (!form.spec.size) return
      
      // 转换为新单位
      if (oldUnit === 'K' && newUnit === 'M') {
        form.spec.size = Math.max(1, Math.round(form.spec.size / 1024))
      } else if (oldUnit === 'K' && newUnit === 'G') {
        form.spec.size = Math.max(1, Math.round(form.spec.size / 1024 / 1024))
      } else if (oldUnit === 'M' && newUnit === 'K') {
        form.spec.size = form.spec.size * 1024
      } else if (oldUnit === 'M' && newUnit === 'G') {
        form.spec.size = Math.max(1, Math.round(form.spec.size / 1024))
      } else if (oldUnit === 'G' && newUnit === 'K') {
        form.spec.size = form.spec.size * 1024 * 1024
      } else if (oldUnit === 'G' && newUnit === 'M') {
        form.spec.size = form.spec.size * 1024
      }
    })

    watch(bsUnit, (newUnit, oldUnit) => {
      if (!form.spec.bs) return
      
      // 转换为新单位
      if (oldUnit === 'K' && newUnit === 'M') {
        form.spec.bs = Math.max(1, Math.round(form.spec.bs / 1024))
      } else if (oldUnit === 'M' && newUnit === 'K') {
        form.spec.bs = form.spec.bs * 1024
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

      // 文件路径验证
      if (!form.spec.filename) {
        newErrors.filename = '测试路径不能为空'
      }

      // 读写模式验证
      if (!form.spec.readwrite) {
        newErrors.readwrite = '测试类型不能为空'
      }

      // 将验证结果设置到errors对象中
      Object.keys(errors).forEach(key => delete errors[key])
      Object.keys(newErrors).forEach(key => {
        errors[key] = newErrors[key]
      })

      return Object.keys(newErrors).length === 0
    }

    // 获取格式化的大小值
    const getFormattedSize = () => {
      return `${form.spec.size}${sizeUnit.value}`
    }

    // 获取格式化的块大小值
    const getFormattedBs = () => {
      return `${form.spec.bs}${bsUnit.value}`
    }

    // 提交表单
    const submitForm = async () => {
      if (!validateForm()) {
        return
      }

      loading.value = true
      error.value = null

      try {
        // 克隆表单数据并格式化大小和块大小
        const fioData = {
          name: form.name,
          spec: {
            ...form.spec,
            size: getFormattedSize(),
            bs: getFormattedBs()
          }
        }

        await fioApi.createFio(fioData)
        success.value = true
      } catch (err) {
        error.value = '创建IO性能测试失败: ' + (err.message || '未知错误')
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
      sizeUnit,
      bsUnit,
      submitForm
    }
  }
}
</script>

<style scoped>
.fio-form {
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

.size-input-group {
  display: flex;
  gap: 0.5rem;
}

.size-input-group input {
  flex: 1;
}

.size-input-group select {
  width: 80px;
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