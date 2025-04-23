<template>
  <nav class="flex py-3 px-5 text-gray-700 bg-gray-100 rounded-md shadow-sm mb-5">
    <ol class="inline-flex items-center space-x-1 md:space-x-3">
      <li class="inline-flex items-center">
        <router-link to="/" class="inline-flex items-center text-sm font-medium text-green-600 hover:text-green-700">
          <svg class="w-4 h-4 mr-2" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg">
            <path d="M10.707 2.293a1 1 0 00-1.414 0l-7 7a1 1 0 001.414 1.414L4 10.414V17a1 1 0 001 1h2a1 1 0 001-1v-2a1 1 0 011-1h2a1 1 0 011 1v2a1 1 0 001 1h2a1 1 0 001-1v-6.586l.293.293a1 1 0 001.414-1.414l-7-7z"></path>
          </svg>
          首页
        </router-link>
      </li>
      <li v-for="(item, index) in breadcrumbs" :key="index" class="inline-flex items-center">
        <div class="flex items-center">
          <svg class="w-6 h-6 text-gray-400" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg">
            <path fill-rule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clip-rule="evenodd"></path>
          </svg>
          <router-link
            v-if="item.path && index < breadcrumbs.length - 1"
            :to="item.path"
            class="ml-1 text-sm font-medium text-green-600 hover:text-green-700 md:ml-2"
          >
            {{ item.name }}
          </router-link>
          <span v-else class="ml-1 text-sm font-medium text-gray-500 md:ml-2">{{ item.name }}</span>
        </div>
      </li>
    </ol>
  </nav>
</template>

<script>
import { computed } from 'vue'
import { useRoute } from 'vue-router'

export default {
  props: {
    customBreadcrumbs: {
      type: Array,
      default: () => []
    }
  },
  setup(props) {
    const route = useRoute()

    // 根据当前路由自动生成面包屑
    const breadcrumbs = computed(() => {
      if (props.customBreadcrumbs.length > 0) {
        return props.customBreadcrumbs
      }

      const pathSegments = route.path.split('/').filter(segment => segment)
      const result = []

      // 根据路由路径生成面包屑
      let currentPath = ''

      for (let i = 0; i < pathSegments.length; i++) {
        const segment = pathSegments[i]
        currentPath += `/${segment}`

        // 根据路径段生成名称
        let name = ''

        switch (segment) {
          case 'dig':
            name = 'DNS查询工具'
            break
          case 'ping':
            name = '连通性测试工具'
            break
          case 'fio':
            name = 'IO性能工具'
            break
          case 'create':
            name = '创建'
            break
          default:
            // 如果是详情页面，使用参数作为名称
            if (i > 0 && (pathSegments[i-1] === 'dig' || pathSegments[i-1] === 'ping' || pathSegments[i-1] === 'fio') && segment !== 'create') {
              name = segment
            } else {
              name = segment
            }
        }

        result.push({
          name,
          path: currentPath
        })
      }

      return result
    })

    return {
      breadcrumbs
    }
  }
}
</script>


