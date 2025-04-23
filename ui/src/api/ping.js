import axios from 'axios'
import { getEndpoints, getHeaders } from "@/api/help";
import namespacesApi from "@/api/namespaces";

// 创建基础客户端，设置基本超时时间
const pingClient = axios.create({
  timeout: 10000
});

// API路径常量
const API_GROUP = 'testtools.xiaoming.com'
const API_VERSION = 'v1'
const PING_RESOURCE = 'pings'
const TESTREPORT_RESOURCE = 'testreports'

// 请求拦截器，在每次请求前设置自定义请求头
pingClient.interceptors.request.use(config => {
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
  // Ping资源相关API
  async getPingList() {
    try {
      const namespace = namespacesApi.getCurrentNamespace();
      const response = await pingClient.get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${namespace}/${PING_RESOURCE}`)
      return response.data
    } catch (error) {
      console.error('获取Ping列表失败:', error)
      throw error
    }
  },

  async getPing(name) {
    try {
      const namespace = namespacesApi.getCurrentNamespace();
      const response = await pingClient.get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${namespace}/${PING_RESOURCE}/${name}`)
      return response.data
    } catch (error) {
      console.error(`获取Ping ${name} 失败:`, error)
      throw error
    }
  },

  async createPing(ping) {
    try {
      const namespace = namespacesApi.getCurrentNamespace();
      // 确保创建对象具有正确的apiVersion和kind
      const pingResource = {
        apiVersion: `${API_GROUP}/${API_VERSION}`,
        kind: 'Ping',
        metadata: {
          name: ping.name,
          namespace: namespace
        },
        spec: {
          ...ping.spec
        }
      }
      const response = await pingClient.post(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${namespace}/${PING_RESOURCE}`, pingResource)
      return response.data
    } catch (error) {
      console.error('创建Ping失败:', error)
      throw error
    }
  },

  async deletePing(name) {
    try {
      const namespace = namespacesApi.getCurrentNamespace();
      const response = await pingClient.delete(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${namespace}/${PING_RESOURCE}/${name}`)
      return response.data
    } catch (error) {
      console.error(`删除Ping ${name} 失败:`, error)
      throw error
    }
  },

  // 获取与特定Ping相关的TestReport
  async getPingTestReport(testReportName) {
    if (!testReportName) {
      return null
    }
    try {
      const namespace = namespacesApi.getCurrentNamespace();
      const response = await pingClient.get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${namespace}/${TESTREPORT_RESOURCE}/${testReportName}`)
      return response.data
    } catch (error) {
      console.error(`获取Ping的TestReport ${testReportName} 失败:`, error)
      throw error
    }
  }
} 