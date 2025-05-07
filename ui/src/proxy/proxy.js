import express from 'express';
import axios from 'axios';
import https from 'https';
import bodyParser from 'body-parser';
import cors from 'cors';

const app = express();
const port = process.env.PORT || 3001;

// 启用CORS和请求体解析
app.use(cors());
app.use(bodyParser.json());
app.use(bodyParser.urlencoded({ extended: true }));

// 配置忽略证书验证的agent
const agent = new https.Agent({
  rejectUnauthorized: false
});

// 为根路径添加简单的说明页面
app.get('/', (req, res) => {
  res.send(`
    <html>
      <head><title>K8s API Proxy</title></head>
      <body>
        <h1>Kubernetes API Proxy</h1>
        <p>This is a proxy server for Kubernetes API requests.</p>
        <p>To use it, send requests to <code>/proxy</code> with the following headers:</p>
        <ul>
          <li><code>x-k8s-endpoints</code>: Your Kubernetes API server address (e.g. 172.30.1.12:6443)</li>
          <li><code>x-k8s-token</code>: Your Kubernetes API token</li>
        </ul>
      </body>
    </html>
  `);
});

// 处理代理请求 - 使用明确的路径前缀
app.use('/proxy', async (req, res) => {
  try {
    // 从请求头中获取endpoints和token
    const endpoints = req.headers['x-k8s-endpoints'];
    const token = req.headers['x-k8s-token'];
    
    if (!endpoints) {
      return res.status(400).json({ error: 'Missing x-k8s-endpoints header' });
    }
    
    // 构建目标URL（去掉前面的/proxy前缀）
    const path = req.originalUrl.replace(/^\/proxy/, '');
    const targetUrl = `https://${endpoints}${path}`;
    
    console.log(`Proxying request to: ${targetUrl}`);
    
    // 准备请求头（复制原始请求头，并根据需要修改）
    const headers = { ...req.headers };
    
    // 删除代理相关的头
    delete headers['x-k8s-endpoints'];
    delete headers['host'];
    
    // 如果提供了token，设置Authorization头
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }
    
    // 发送请求到目标服务器
    const response = await axios({
      method: req.method,
      url: targetUrl,
      headers: headers,
      data: req.method !== 'GET' ? req.body : undefined,
      httpsAgent: agent,
      responseType: 'stream'
    });
    
    // 设置响应头
    Object.keys(response.headers).forEach(key => {
      res.set(key, response.headers[key]);
    });
    
    // 设置状态码并返回响应
    res.status(response.status);
    response.data.pipe(res);
    
  } catch (error) {
    console.error('Proxy error:', error.message);
    
    // 如果有响应，直接返回原始响应
    if (error.response) {
      // 设置响应头
      Object.keys(error.response.headers).forEach(key => {
        res.set(key, error.response.headers[key]);
      });
      
      res.status(error.response.status);
      error.response.data.pipe(res);
    } else {
      // 无响应的错误情况
      res.status(500).json({
        error: 'Proxy Error',
        message: error.message
      });
    }
  }
});

// 使用标准的404处理中间件，避免使用通配符
app.use((req, res) => {
  res.status(404).json({ 
    error: 'Not Found',
    message: 'The requested path does not exist. Use /proxy for API requests.'
  });
});

// 启动服务器
app.listen(port, '0.0.0.0',() => {
  console.log(`Proxy server is running on port ${port}`);
});

// 导出app以便测试
export default app;