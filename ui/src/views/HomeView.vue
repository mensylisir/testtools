<template>
  <div class="home">
    <div class="hero card">
      <h1>TestTools - Kubernetes网络测试工具</h1>
      <p>使用TestTools轻松进行DNS查询、网络连通性测试和IO性能测试</p>
    </div>

    <div class="tools-section">
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

.card {
  background-color: var(--card-color);
  border-radius: 8px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06), 0 4px 10px rgba(0, 0, 0, 0.08);
  padding: 1.5em;
  border: 1px solid #e9ecef;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.card:hover {
  transform: translateY(-4px);
  box-shadow: 0 8px 16px rgba(0, 0, 0, 0.15);
}


.hero.card {
  padding: 3rem 2rem;
  text-align: center;
  margin-bottom: 1.5rem;
  background-color: var(--primary-color);
  color: white;
  box-shadow: 0 4px 8px rgba(0, 0, 0, 0.06), 0 6px 15px rgba(0, 0, 0, 0.2);
  border: none;
}

.hero h1 {
  font-size: 2.8rem;
  margin-bottom: 1rem;
  color: inherit;
}

.hero p {
  font-size: 1.2rem;
  margin-bottom: 0;
  color: rgba(255, 255, 255, 0.9);
}

.tools-section {
  margin-bottom: 3rem;
}

.tools-section h2 {
  font-size: 2rem;
  margin-bottom: 2rem;
  color: var(--text-primary);
  text-align: center;
}

.tools-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
  gap: 1.5rem;
}

.tool-card.card {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.tool-card h3 {
  font-size: 1.4rem;
  margin-top: 0;
  margin-bottom: 0.8rem;
  color: var(--primary-color-dark);
}

.tool-card p {
  font-size: 1rem;
  margin-bottom: 1.5rem;
  flex-grow: 1;
  line-height: 1.6;
  color: var(--text-secondary);
}

.tool-card .btn {
  display: block;
  width: 100%;
  box-sizing: border-box;
  margin-top: auto;
}

</style> 