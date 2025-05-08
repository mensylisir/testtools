import axios from 'axios'
import { getEndpoints, getHeaders } from "@/api/help";
import namespacesApi from "@/api/namespaces";

const fioClient = axios.create({
  timeout: 10000
});

const API_GROUP = 'testtools.xiaoming.com'
const API_VERSION = 'v1'
const FIO_RESOURCE = 'fios'
const TESTREPORT_RESOURCE = 'testreports'

fioClient.interceptors.request.use(config => {
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