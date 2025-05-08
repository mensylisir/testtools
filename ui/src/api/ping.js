import axios from 'axios'
import { getEndpoints, getHeaders } from "@/api/help";
import namespacesApi from "@/api/namespaces";

const pingClient = axios.create({
  timeout: 10000
});

const API_GROUP = 'testtools.xiaoming.com'
const API_VERSION = 'v1'
const PING_RESOURCE = 'pings'
const TESTREPORT_RESOURCE = 'testreports'

pingClient.interceptors.request.use(config => {
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