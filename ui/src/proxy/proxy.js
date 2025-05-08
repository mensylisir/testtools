import express from 'express';
import axios from 'axios';
import https from 'https';
import bodyParser from 'body-parser';
import cors from 'cors';

const app = express();
const port = process.env.PORT || 3001;

app.use(cors());
app.use(bodyParser.json());
app.use(bodyParser.urlencoded({ extended: true }));

const agent = new https.Agent({
  rejectUnauthorized: false
});

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

app.use('/proxy', async (req, res) => {
  try {
    const endpoints = req.headers['x-k8s-endpoints'];
    const token = req.headers['x-k8s-token'];
    
    if (!endpoints) {
      return res.status(400).json({ error: 'Missing x-k8s-endpoints header' });
    }
    
    const path = req.originalUrl.replace(/^\/proxy/, '');
    const targetUrl = `https://${endpoints}${path}`;
    
    console.log(`Proxying request to: ${targetUrl}`);
    
    const headers = { ...req.headers };
    
    delete headers['x-k8s-endpoints'];
    delete headers['host'];
    
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }
    
    const response = await axios({
      method: req.method,
      url: targetUrl,
      headers: headers,
      data: req.method !== 'GET' ? req.body : undefined,
      httpsAgent: agent,
      responseType: 'stream'
    });
    
    Object.keys(response.headers).forEach(key => {
      res.set(key, response.headers[key]);
    });
    
    res.status(response.status);
    response.data.pipe(res);
    
  } catch (error) {
    console.error('Proxy error:', error.message);
    
    if (error.response) {
      Object.keys(error.response.headers).forEach(key => {
        res.set(key, error.response.headers[key]);
      });
      
      res.status(error.response.status);
      error.response.data.pipe(res);
    } else {
      res.status(500).json({
        error: 'Proxy Error',
        message: error.message
      });
    }
  }
});

app.use((req, res) => {
  res.status(404).json({ 
    error: 'Not Found',
    message: 'The requested path does not exist. Use /proxy for API requests.'
  });
});

app.listen(port,'0.0.0.0', () => {
  console.log(`Proxy server is running on port ${port}`);
});