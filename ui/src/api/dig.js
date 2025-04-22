import axios from 'axios'
import { getEndpoints, getHeaders } from "@/api/help";

// 创建基础客户端，设置基本超时时间
const digClient = axios.create({
  timeout: 10000
});

// API路径常量
const API_GROUP = 'testtools.xiaoming.com'
const API_VERSION = 'v1'
const DIG_RESOURCE = 'digs'
const TESTREPORT_RESOURCE = 'testreports'
const NAMESPACE = 'default' // 默认命名空间，可根据需要修改

// 请求拦截器，在每次请求前设置自定义请求头
digClient.interceptors.request.use(config => {
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
  // Dig资源相关API
  async getDigList() {
    try {
      const response = await digClient.get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${NAMESPACE}/${DIG_RESOURCE}`)
      return response.data
    } catch (error) {
      console.error('获取Dig列表失败:', error)
      throw error
    }
  },

  async getDig(name) {
    try {
      const response = await digClient.get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${NAMESPACE}/${DIG_RESOURCE}/${name}`)
      return response.data
    } catch (error) {
      console.error(`获取Dig ${name} 失败:`, error)
      throw error
    }
  },

  async createDig(dig) {
    try {
      // 确保创建对象具有正确的apiVersion和kind
      const digResource = {
        apiVersion: `${API_GROUP}/${API_VERSION}`,
        kind: 'Dig',
        metadata: {
          name: dig.name,
          namespace: NAMESPACE
        },
        spec: {
          ...dig.spec
        }
      }
      const response = await digClient.post(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${NAMESPACE}/${DIG_RESOURCE}`, digResource)
      return response.data
    } catch (error) {
      console.error('创建Dig失败:', error)
      throw error
    }
  },

  async deleteDig(name) {
    try {
      const response = await digClient.delete(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${NAMESPACE}/${DIG_RESOURCE}/${name}`)
      return response.data
    } catch (error) {
      console.error(`删除Dig ${name} 失败:`, error)
      throw error
    }
  },

  // TestReport资源相关API
  async getTestReportList() {
    try {
      const response = await digClient.get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${NAMESPACE}/${TESTREPORT_RESOURCE}`)
      return response.data
    } catch (error) {
      console.error('获取TestReport列表失败:', error)
      throw error
    }
  },

  async getTestReport(name) {
    try {
      const response = await digClient.get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${NAMESPACE}/${TESTREPORT_RESOURCE}/${name}`)
      return response.data
    } catch (error) {
      console.error(`获取TestReport ${name} 失败:`, error)
      throw error
    }
  },

  // 获取与特定Dig相关的TestReport
  async getDigTestReport(testReportName) {
    if (!testReportName) {
      return null
    }
    try {
      const response = await digClient.get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${NAMESPACE}/${TESTREPORT_RESOURCE}/${testReportName}`)
      return response.data
    } catch (error) {
      console.error(`获取Dig的TestReport ${testReportName} 失败:`, error)
      throw error
    }
  }
} 