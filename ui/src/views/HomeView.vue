<template>
  <div class="home">
    <div class="hero card">
      <h1>TestTools - Kubernetes网络测试工具</h1>
      <p>使用TestTools轻松进行DNS查询、网络连通性测试和IO性能测试</p>
    </div>

    <div class="tools-section">
      <h2>可用工具</h2>
      
      <div class="tools-grid">
        <div class="tool-card card">
          <h3>DNS查询</h3>
          <p>使用Dig工具进行DNS查询，检查域名解析是否正常工作</p>
          <router-link to="/dig" class="btn btn-primary">查看Dig工具</router-link>
        </div>
        
        <div class="tool-card card">
          <h3>连通性测试</h3>
          <p>使用Ping工具测试与目标主机的网络连通性</p>
          <router-link to="/ping" class="btn btn-primary">查看Ping工具</router-link>
        </div>
        
        <div class="tool-card card">
          <h3>IO性能测试</h3>
          <p>使用FIO工具测试存储IO性能</p>
          <router-link to="/fio" class="btn btn-primary">查看FIO工具</router-link>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.hero {
  padding: 2rem;
  text-align: center;
  margin-bottom: 2rem;
  background-color: var(--primary-color);
  color: white;
}

.hero h1 {
  font-size: 2rem;
  margin-bottom: 1rem;
}

.tools-section h2 {
  margin-bottom: 1.5rem;
}

.tools-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 1.5rem;
}

.tool-card {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.tool-card h3 {
  margin-bottom: 1rem;
  color: var(--primary-color);
}

.tool-card p {
  margin-bottom: 1.5rem;
  flex-grow: 1;
}
</style> 