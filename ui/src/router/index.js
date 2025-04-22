import { createRouter, createWebHistory } from 'vue-router'

// 导入视图组件
import HomeView from '../views/HomeView.vue'
import DigListView from '../views/DigListView.vue'
import DigCreateView from '../views/DigCreateView.vue'
import DigDetailView from '../views/DigDetailView.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'home',
      component: HomeView
    },
    {
      path: '/dig',
      name: 'dig-list',
      component: DigListView
    },
    {
      path: '/dig/create',
      name: 'dig-create',
      component: DigCreateView
    },
    {
      path: '/dig/:name',
      name: 'dig-detail',
      component: DigDetailView,
      props: true
    }
  ]
})

export default router 