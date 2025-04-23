import axios from 'axios'
import store from "@/store";
import { getEndpoints, getHeaders } from "@/api/help";

// 创建基础客户端，设置基本超时时间
const k8sClient = axios.create({
  timeout: 10000
});

// 请求拦截器，在每次请求前设置自定义请求头
k8sClient.interceptors.request.use(config => {
  // 获取当前的endpoints和headers
  const endpoints = getEndpoints();
  const headers = getHeaders();
  
  // 设置基础URL为相对路径，让Vite开发服务器或Nginx处理代理
  config.baseURL = '/api';
  
  // 将endpoints和token放入自定义请求头中
  config.headers = {
    ...config.headers,
    ...headers
  };
  
  // 如果headers中有Authorization头，提取出token放入自定义头
  if (headers.Authorization) {
    const token = headers.Authorization.replace('Bearer ', '');
    config.headers['X-K8s-Token'] = token;
  }
  
  console.log('API Endpoint:', endpoints);
  console.log('Headers:', config.headers);
  
  return config;
}, error => {
  return Promise.reject(error);
});

export default {
  // 获取命名空间列表
  async getNamespacesList() {
    try {
      const response = await k8sClient.get('/api/v1/namespaces')
      return response.data
    } catch (error) {
      console.error('获取命名空间列表失败:', error)
      throw error
    }
  },

  async saveCurrentNamespace(namespace) {
    await store.dispatch('setCurrentNamespace', namespace)
  },

  // 获取当前命名空间
  getCurrentNamespace() {
    return store.getters["getCurrentNamespace"] || 'default'
  }
} 