# 第一阶段：构建前端项目
FROM node:18-alpine as builder
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
RUN npm run build

# 第二阶段：准备 Node.js 代理模块
FROM node:18-alpine
WORKDIR /app
COPY --from=builder /app/dist ./dist
COPY server ./server
COPY package*.json ./
RUN npm install --production

# 第三阶段：安装 Nginx、Node.js 并配置，同时启动 Node.js 代理模块和 Nginx
FROM nginx:alpine

# 安装 Node.js 和 supervisord
RUN apk add --no-cache nodejs npm supervisor

COPY --from=1 /app /app
COPY nginx.conf /etc/nginx/nginx.conf

# 配置 supervisord
COPY supervisord.conf /etc/supervisor/conf.d/supervisord.conf

EXPOSE 80
CMD ["supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]