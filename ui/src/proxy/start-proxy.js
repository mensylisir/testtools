/**
 * 代理服务器启动脚本
 * 
 * 使用方法:
 * node src/proxy/start-proxy.js [端口号]
 * 
 * 如果不指定端口号，将使用默认端口3001
 */

// 获取命令行参数中的端口号（如果提供）
const args = process.argv.slice(2);
if (args.length > 0 && !isNaN(parseInt(args[0]))) {
  process.env.PORT = parseInt(args[0]);
}

// 导入代理服务模块
import './proxy.js';

console.log('Started the proxy server with the following environment:');
console.log(`- Node version: ${process.version}`);
console.log(`- Environment: ${process.env.NODE_ENV || 'development'}`);
console.log(`- Port: ${process.env.PORT || 3001}`);
console.log('');
console.log('To use this proxy:');
console.log('1. Make sure your client sends the following headers:');
console.log('   - X-K8s-Endpoints: your-k8s-api-server:port');
console.log('   - X-K8s-Token: your-token (optional)');
console.log('');
console.log('2. Send requests to http://localhost:{PORT}/proxy/your/api/path');
console.log('');
console.log('Press Ctrl+C to stop the proxy server'); 