import axios from 'axios'
import { getEndpoints, getHeaders } from "@/api/help";
import namespacesApi from "@/api/namespaces";

const digClient = axios.create({
  timeout: 10000
});

const API_GROUP = 'testtools.xiaoming.com'
const API_VERSION = 'v1'
const DIG_RESOURCE = 'digs'
const TESTREPORT_RESOURCE = 'testreports'

digClient.interceptors.request.use(config => {
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
  async getDigList() {
    try {
      const namespace = namespacesApi.getCurrentNamespace();
      const response = await digClient.get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${namespace}/${DIG_RESOURCE}`)
      return response.data
    } catch (error) {
      console.error('获取Dig列表失败:', error)
      throw error
    }
  },

  async getDig(name) {
    try {
      const namespace = namespacesApi.getCurrentNamespace();
      const response = await digClient.get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${namespace}/${DIG_RESOURCE}/${name}`)
      return response.data
    } catch (error) {
      console.error(`获取Dig ${name} 失败:`, error)
      throw error
    }
  },

  async createDig(dig) {
    try {
      const namespace = namespacesApi.getCurrentNamespace();
      const digResource = {
        apiVersion: `${API_GROUP}/${API_VERSION}`,
        kind: 'Dig',
        metadata: {
          name: dig.name,
          namespace: namespace
        },
        spec: {
          ...dig.spec
        }
      }
      const response = await digClient.post(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${namespace}/${DIG_RESOURCE}`, digResource)
      return response.data
    } catch (error) {
      console.error('创建Dig失败:', error)
      throw error
    }
  },

  async deleteDig(name) {
    try {
      const namespace = namespacesApi.getCurrentNamespace();
      const response = await digClient.delete(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${namespace}/${DIG_RESOURCE}/${name}`)
      return response.data
    } catch (error) {
      console.error(`删除Dig ${name} 失败:`, error)
      throw error
    }
  },

  async getTestReportList() {
    try {
      const namespace = namespacesApi.getCurrentNamespace();
      const response = await digClient.get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${namespace}/${TESTREPORT_RESOURCE}`)
      return response.data
    } catch (error) {
      console.error('获取TestReport列表失败:', error)
      throw error
    }
  },

  async getTestReport(name) {
    try {
      const namespace = namespacesApi.getCurrentNamespace();
      const response = await digClient.get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${namespace}/${TESTREPORT_RESOURCE}/${name}`)
      return response.data
    } catch (error) {
      console.error(`获取TestReport ${name} 失败:`, error)
      throw error
    }
  },

  async getDigTestReport(testReportName) {
    if (!testReportName) {
      return null
    }
    try {
      const namespace = namespacesApi.getCurrentNamespace();
      const response = await digClient.get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/${namespace}/${TESTREPORT_RESOURCE}/${testReportName}`)
      return response.data
    } catch (error) {
      console.error(`获取Dig的TestReport ${testReportName} 失败:`, error)
      throw error
    }
  }
} 