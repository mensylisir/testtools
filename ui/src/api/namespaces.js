import axios from 'axios'
import store from "@/store";
import { getEndpoints, getHeaders } from "@/api/help";

const k8sClient = axios.create({
  timeout: 10000
});

k8sClient.interceptors.request.use(config => {
  const endpoints = getEndpoints();
  const headers = getHeaders();
  
  config.baseURL = '/api';
  
  config.headers = {
    ...config.headers,
    ...headers
  };
  
  if (headers.Authorization) {
    const token = headers.Authorization.replace('Bearer ', '');
    config.headers['X-K8s-Token'] = token;
  }
  
  // console.log('API Endpoint:', endpoints);
  // console.log('Headers:', config.headers);
  
  return config;
}, error => {
  return Promise.reject(error);
});

export default {
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

  getCurrentNamespace() {
    return store.getters["getCurrentNamespace"] || 'default'
  }
} 