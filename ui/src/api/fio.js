import axios from 'axios'
import { getEndpoints, getHeaders } from "@/api/help";
import namespacesApi from "@/api/namespaces";

// 创建基础客户端，设置基本超时时间
const fioClient = axios.create({
  timeout: 10000
});

// API路径常量
const API_GROUP = 'testtools.xiaoming.com'
const API_VERSION = 'v1'
const FIO_RESOURCE = 'fios'
const TESTREPORT_RESOURCE = 'testreports'

// 请求拦截器，在每次请求前设置自定义请求头
fioClient.interceptors.request.use(config => {
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
  // Fio资源相关API
  async getFioList() {
    try {
      const namespace = namespacesApi.getCurrentNamespace();
      const response = await fioClient.get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${namespace}/${FIO_RESOURCE}`)
      return response.data
    } catch (error) {
      console.error('获取Fio列表失败:', error)
      throw error
    }
  },

  async getFio(name) {
    try {
      const namespace = namespacesApi.getCurrentNamespace();
      const response = await fioClient.get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${namespace}/${FIO_RESOURCE}/${name}`)
      return response.data
    } catch (error) {
      console.error(`获取Fio ${name} 失败:`, error)
      throw error
    }
  },

  async createFio(fio) {
    try {
      const namespace = namespacesApi.getCurrentNamespace();
      // 确保创建对象具有正确的apiVersion和kind
      const fioResource = {
        apiVersion: `${API_GROUP}/${API_VERSION}`,
        kind: 'Fio',
        metadata: {
          name: fio.name,
          namespace: namespace
        },
        spec: {
          ...fio.spec
        }
      }
      const response = await fioClient.post(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${namespace}/${FIO_RESOURCE}`, fioResource)
      return response.data
    } catch (error) {
      console.error('创建Fio失败:', error)
      throw error
    }
  },

  async deleteFio(name) {
    try {
      const namespace = namespacesApi.getCurrentNamespace();
      const response = await fioClient.delete(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${namespace}/${FIO_RESOURCE}/${name}`)
      return response.data
    } catch (error) {
      console.error(`删除Fio ${name} 失败:`, error)
      throw error
    }
  },

  // 获取与特定Fio相关的TestReport
  async getFioTestReport(testReportName) {
    if (!testReportName) {
      return null
    }
    try {
      const namespace = namespacesApi.getCurrentNamespace();
      const response = await fioClient.get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${namespace}/${TESTREPORT_RESOURCE}/${testReportName}`)
      return response.data
    } catch (error) {
      console.error(`获取Fio的TestReport ${testReportName} 失败:`, error)
      throw error
    }
  }
} 