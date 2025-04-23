<template>
  <div class="namespace-selector">
<!--    <div class="selector-label">命名空间:</div>-->
    <div class="selector-dropdown" v-click-outside="hideNamespaceOptions">
      <div class="selected-option" @click="toggleNamespaceOptions">
        <span>{{ currentNamespace }}</span>
        <span class="dropdown-arrow">▼</span>
      </div>
      <div class="options-container" v-if="showNamespaceOptions">
        <div v-if="loading" class="option loading">正在加载...</div>
        <div v-else-if="error" class="option error">加载失败</div>
        <div v-else-if="namespaceOptions.length === 0" class="option empty">没有命名空间</div>
        <div
          v-else
          v-for="ns in namespaceOptions"
          :key="ns.metadata.name"
          class="option"
          :class="{ 'active': ns.metadata.name === currentNamespace }"
          @click="selectNamespace(ns.metadata.name)"
        >
          {{ ns.metadata.name }}
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { ref, onMounted } from 'vue'
import namespacesApi from '../api/namespaces.js'

// 点击外部关闭下拉菜单指令
const clickOutside = {
  beforeMount(el, binding) {
    el._clickOutside = (event) => {
      if (!(el === event.target || el.contains(event.target))) {
        binding.value(event)
      }
    }
    document.addEventListener('click', el._clickOutside)
  },
  unmounted(el) {
    document.removeEventListener('click', el._clickOutside)
  }
}

export default {
  name: 'NamespaceSelector',
  directives: {
    'click-outside': clickOutside
  },
  emits: ['namespace-changed'],
  setup(_, { emit }) {
    const namespaceOptions = ref([])
    const currentNamespace = ref('')
    const showNamespaceOptions = ref(false)
    const loading = ref(false)
    const error = ref(null)

    // 获取命名空间列表
    const fetchNamespaces = async () => {
      loading.value = true
      error.value = null

      try {
        const response = await namespacesApi.getNamespacesList()
        namespaceOptions.value = response.items || []
      } catch (err) {
        console.error('获取命名空间列表失败:', err)
        error.value = err
      } finally {
        loading.value = false
      }
    }

    // 切换显示/隐藏下拉菜单
    const toggleNamespaceOptions = () => {
      showNamespaceOptions.value = !showNamespaceOptions.value
      if (showNamespaceOptions.value) {
        fetchNamespaces()
      }
    }

    // 隐藏下拉菜单
    const hideNamespaceOptions = () => {
      showNamespaceOptions.value = false
    }

    // 选择命名空间
    const selectNamespace = (namespace) => {
      currentNamespace.value = namespace
      namespacesApi.saveCurrentNamespace(namespace)
      emit('namespace-changed', namespace)
      hideNamespaceOptions()
    }

    // 初始化组件
    onMounted(() => {
      currentNamespace.value = namespacesApi.getCurrentNamespace()
    })

    return {
      namespaceOptions,
      currentNamespace,
      showNamespaceOptions,
      loading,
      error,
      toggleNamespaceOptions,
      hideNamespaceOptions,
      selectNamespace
    }
  }
}
</script>

<style scoped>
.namespace-selector {
  display: flex;
  align-items: center;
}

.selector-label {
  margin-right: 0.5rem;
  color: white;
  font-size: 0.9rem;
}

.selector-dropdown {
  position: relative;
  min-width: 150px;
}

.selected-option {
  padding: 0.25rem 0.5rem;
  background-color: rgba(255, 255, 255, 0.2);
  border-radius: 4px;
  cursor: pointer;
  display: flex;
  justify-content: space-between;
  align-items: center;
  color: white;
  user-select: none;
}

.dropdown-arrow {
  margin-left: 0.5rem;
  font-size: 0.8rem;
}

.options-container {
  position: absolute;
  top: 100%;
  left: 0;
  right: 0;
  margin-top: 0.25rem;
  background-color: white;
  border-radius: 4px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
  z-index: 1000;
  max-height: 250px;
  overflow-y: auto;
}

.option {
  padding: 0.5rem;
  color: black;
  cursor: pointer;
  transition: background-color 0.2s;
}

.option:hover {
  background-color: #f5f5f5;
}

.option.active {
  background-color: #e6f7ff;
  font-weight: 500;
}

.option.loading,
.option.error,
.option.empty {
  text-align: center;
  padding: 0.75rem;
  color: #999;
  cursor: default;
}

.option.error {
  color: #d32f2f;
}
</style>