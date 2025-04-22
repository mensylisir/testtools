/**
 * 代理服务器测试脚本
 * 
 * 使用方法:
 * node src/proxy/test-proxy.js <endpoints> <token>
 * 
 * 例如:
 * node src/proxy/test-proxy.js 172.30.1.12:6443 eyJhbGciOiJSUzI1...
 */

import axios from 'axios';

// 获取命令行参数
const args = process.argv.slice(2);
const endpoints = args[0] || '172.30.1.12:6443';
const token = args[1] || '';
const port = process.env.PORT || 3001;
const url = `http://localhost:${port}/proxy/api/v1/namespaces`;

console.log('Testing proxy server with:');
console.log(`- Endpoints: ${endpoints}`);
if (token) {
  console.log(`- Token: ${token.substring(0, 10)}...`);
} else {
  console.log('- Token: [none provided]');
}
console.log(`- URL: ${url}`);
console.log('');

// 测试代理服务器
async function testProxy() {
  try {
    console.log('Sending request...');
    
    const headers = {
      'x-k8s-endpoints': endpoints
    };
    
    if (token) {
      headers['x-k8s-token'] = token;
    }
    
    const response = await axios.get(url, { headers });
    
    console.log('Response received!');
    console.log(`Status: ${response.status}`);
    console.log('Headers:', JSON.stringify(response.headers, null, 2));
    console.log('Data (first 500 chars):', JSON.stringify(response.data).substring(0, 500) + '...');
    
    console.log('\nProxy test successful!');
  } catch (error) {
    console.error('Error testing proxy:');
    
    if (error.response) {
      console.error(`Status: ${error.response.status}`);
      console.error('Response data:', error.response.data);
    } else {
      console.error(error.message);
    }
    
    if (error.code === 'ECONNREFUSED') {
      console.error('\nProxy server is not running. Start it with: npm run proxy');
    }
  }
}

testProxy(); 