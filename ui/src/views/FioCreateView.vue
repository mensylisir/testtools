<template>
  <div class="fio-create">
    <div class="header-actions">
      <h1>创建IO性能测试</h1>
      <router-link to="/fio" class="btn btn-secondary">返回查询列表</router-link>
    </div>

    <div class="card">
      <form @submit.prevent="createFio">
        <div class="form-group">
          <label for="name">名称 *</label>
          <input
              type="text"
              id="name"
              v-model="fioForm.name"
              class="form-control"
              required
              placeholder="输入唯一的测试名称"
          />
          <small>名称只能包含小写字母、数字和'-'，且必须以字母或数字开头和结尾</small>
        </div>

        <div class="form-group">
          <label for="filename">测试路径 *</label>
          <input
              type="text"
              id="filename"
              v-model="fioForm.spec.filePath"
              class="form-control"
              required
              placeholder="/path/to/test"
          />
          <small>测试IO性能的目标文件或目录路径</small>
        </div>

        <div class="form-row">
          <div class="form-group">
            <label for="readwrite">测试类型 *</label>
            <select
                id="readwrite"
                v-model="fioForm.spec.readWrite"
                required
                class="form-control"
            >
              <option value="read">顺序读取</option>
              <option value="write">顺序写入</option>
              <option value="randread">随机读取</option>
              <option value="randwrite">随机写入</option>
              <option value="readwrite">混合读写</option>
              <option value="randrw">随机混合读写</option>
            </select>
          </div>
          <div class="form-group">
            <label for="size">测试文件大小</label>
            <div class="input-with-unit-group">
              <input
                  type="number"
                  id="size"
                  v-model.number="fioForm.spec.size"
                  min="1"
                  class="form-control"
              />
              <select v-model="sizeUnit" class="unit-select">
                <option value="K">KB</option>
                <option value="M">MB</option>
                <option value="G">GB</option>
              </select>
            </div>
            <small>要生成或使用的测试文件大小</small>
          </div>
        </div>

        <div class="form-row">
          <div class="form-group">
            <label for="bs">块大小</label>
            <div class="input-with-unit-group">
              <input
                  type="number"
                  id="bs"
                  v-model.number="fioForm.spec.blockSize"
                  min="1"
                  class="form-control"
              />
              <select v-model="bsUnit" class="unit-select">
                <option value="K">KB</option>
                <option value="M">MB</option>
              </select>
            </div>
            <small>I/O操作的块大小</small>
          </div>

          <div class="form-group">
            <label for="iodepth">I/O深度</label>
            <input
                type="number"
                id="iodepth"
                v-model.number="fioForm.spec.ioDepth"
                min="1"
                class="form-control"
            />
            <small>一次提交的I/O请求数量</small>
          </div>
        </div>

        <div class="form-row">
          <div class="form-group">
            <label for="runtime">运行时间 (秒)</label>
            <input
                type="number"
                id="runtime"
                v-model.number="fioForm.spec.runtime"
                min="1"
                max="3600"
                class="form-control"
            />
            <small>测试运行的最大时间 (1-3600秒)</small>
          </div>
        </div>

        <div class="form-row">
          <div class="form-group">
            <label for="nodeName">节点名称</label>
            <input
                type="text"
                id="nodeName"
                v-model="fioForm.spec.nodeName"
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
            <span v-else>创建IO测试</span>
          </button>
        </div>
      </form>
    </div>

    <div v-if="successMessage" class="success-message card">
      <h3>创建成功</h3>
      <p>{{ successMessage }}</p>
      <div class="success-actions">
        <router-link :to="'/fio/' + createdFioName" class="btn btn-primary">查看详情</router-link>
        <router-link to="/fio" class="btn btn-secondary">返回列表</router-link>
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
import {ref, reactive, computed, watch} from 'vue'
import {useRouter} from 'vue-router'
import fioApi from '../api/fio.js'

export default {
  setup() {
    const router = useRouter()
    const creating = ref(false)
    const error = ref(false)
    const successMessage = ref(null)
    const createdFioName = ref('')
    const sizeUnit = ref('M')
    const bsUnit = ref('K')

    // 表单数据
    const fioForm = reactive({
      name: '',
      spec: {
        filePath: '/tmp/test',
        readWrite: 'read',
        size: 100,
        blockSize: 4,
        ioDepth: 8,
        runtime: 60,
        nodeName: ''
      }
    })

    // 处理单位转换
    watch(sizeUnit, (newUnit, oldUnit) => {
      if (fioForm.spec.size === null || fioForm.spec.size === '') return

      if (oldUnit === 'K' && newUnit === 'M') {
        fioForm.spec.size = Math.max(1, Math.round(fioForm.spec.size / 1024))
      } else if (oldUnit === 'K' && newUnit === 'G') {
        fioForm.spec.size = Math.max(1, Math.round(fioForm.spec.size / 1024 / 1024))
      } else if (oldUnit === 'M' && newUnit === 'K') {
        fioForm.spec.size = fioForm.spec.size * 1024
      } else if (oldUnit === 'M' && newUnit === 'G') {
        fioForm.spec.size = Math.max(1, Math.round(fioForm.spec.size / 1024))
      } else if (oldUnit === 'G' && newUnit === 'K') {
        fioForm.spec.size = fioForm.spec.size * 1024 * 1024
      } else if (oldUnit === 'G' && newUnit === 'M') {
        fioForm.spec.size = fioForm.spec.size * 1024
      }
    })

    watch(bsUnit, (newUnit, oldUnit) => {
      if (fioForm.spec.blockSize === null || fioForm.spec.blockSize === '') return

      // 转换为新单位
      if (oldUnit === 'K' && newUnit === 'M') {
        fioForm.spec.blockSize = Math.max(1, Math.round(fioForm.spec.blockSize / 1024))
      } else if (oldUnit === 'M' && newUnit === 'K') {
        fioForm.spec.blockSize = fioForm.spec.blockSize * 1024
      }
    })

    const isFormValid = computed(() => {
      const namePattern = /^[a-z][a-z0-9-]*$/
      const isNameValid = namePattern.test(fioForm.name)

      const isFileNameValid = !!fioForm.spec.filePath.trim()

      const isReadWriteValid = !!fioForm.spec.readWrite.trim()

      return isNameValid && isFileNameValid && isReadWriteValid

    })

    const getFormattedSize = () => {
      return `${fioForm.spec.size}${sizeUnit.value}`
    }

    const getFormattedBs = () => {
      return `${fioForm.spec.blockSize}${bsUnit.value}`
    }

    const createFio = async () => {
      if (!isFormValid.value) return
      creating.value = true
      error.value = null
      successMessage.value = null

      try {
        const cleanedSpec = {
          ...fioForm.spec,
          size: getFormattedSize(),
          blockSize: getFormattedBs()
        }

        Object.keys(cleanedSpec).forEach((key) => {
          if (cleanedSpec[key] === '' || cleanedSpec[key] === null) {
            delete cleanedSpec[key]
          }
        })
        const response = await fioApi.createFio({
          name: fioForm.name,
          spec: cleanedSpec,
        })

        createdFioName.value = response.metadata.name
        successMessage.value = `成功创建IO查询：${response.metadata.name}`
        resetForm()
      } catch (err) {
        error.value = '创建失败:' + (err.message || '未知错误')
        console.error(err)
      } finally {
        creating.value = false
      }
    }

    const resetForm = () => {
      fioForm.name = ''
      fioForm.spec.filePath = ''
      fioForm.spec.readWrite = ''
      fioForm.spec.size = ''
      fioForm.spec.blockSize = ''
      fioForm.spec.ioDepth = ''
      fioForm.spec.runtime = ''
      fioForm.spec.nodeName = ''
    }

    return {
      fioForm,
      creating,
      error,
      successMessage,
      createdFioName,
      isFormValid,
      createFio,
      resetForm,
      sizeUnit,
      bsUnit,
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

.input-with-unit-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.input-with-unit-group .form-control {
  flex-grow: 1;
  min-width: 0;
}

.input-with-unit-group .unit-select {
  flex-shrink: 0;
  width: 70px;
  padding: 10px;
  border: 1px solid #ddd;
  border-radius: 4px;
  font-size: 14px;
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