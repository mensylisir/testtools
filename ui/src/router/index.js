import { createRouter, createWebHistory } from 'vue-router'

// 导入视图组件
import HomeView from '../views/HomeView.vue'
import DigListView from '../views/DigListView.vue'
import DigCreateView from '../views/DigCreateView.vue'
import DigDetailView from '../views/DigDetailView.vue'
import PingListView from '../views/PingListView.vue'
import PingCreateView from '../views/PingCreateView.vue'
import PingDetailView from '../views/PingDetailView.vue'
import FioListView from '../views/FioListView.vue'
import FioCreateView from '../views/FioCreateView.vue'
import FioDetailView from '../views/FioDetailView.vue'

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
    },
    {
      path: '/ping',
      name: 'ping-list',
      component: PingListView
    },
    {
      path: '/ping/create',
      name: 'ping-create',
      component: PingCreateView
    },
    {
      path: '/ping/:name',
      name: 'ping-detail',
      component: PingDetailView,
      props: true
    },
    {
      path: '/fio',
      name: 'fio-list',
      component: FioListView
    },
    {
      path: '/fio/create',
      name: 'fio-create',
      component: FioCreateView
    },
    {
      path: '/fio/:name',
      name: 'fio-detail',
      component: FioDetailView,
      props: true
    }
  ]
})

export default router 