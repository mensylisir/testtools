FROM node:18-alpine as builder
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
RUN npm run build

FROM node:18-alpine as server-builder
WORKDIR /app
COPY --from=builder /app/dist ./dist
COPY src/proxy ./src/proxy
COPY package*.json ./
RUN npm install --production

FROM nginx:alpine
RUN apk add --no-cache nodejs npm supervisor
COPY --from=server-builder /app /app
COPY nginx.conf /etc/nginx/nginx.conf
COPY supervisord.conf /etc/supervisor/conf.d/supervisord.conf
EXPOSE 80
CMD ["supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]