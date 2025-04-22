<template>
  <header class="app-header">
    <div class="container">
      <h1>TestTools</h1>
      <nav class="main-nav">
        <router-link to="/" class="nav-link">首页</router-link>
        <router-link to="/dig" class="nav-link">DNS查询工具</router-link>
        <router-link to="/ping" class="nav-link">Ping工具</router-link>
        <router-link to="/fio" class="nav-link">IO性能工具</router-link>
      </nav>
    </div>
  </header>

  <main class="container">
    <router-view />
  </main>

  <footer class="app-footer">
    <div class="container">
      <p>&copy; 2025 TestTools - Kubernetes 网络测试工具</p>
    </div>
  </footer>
</template>

<style>
.app-header {
  background-color: var(--primary-color);
  color: white;
  padding: 1rem 0;
  box-shadow: 0 2px 5px rgba(0, 0, 0, 0.1);
}

.app-header h1 {
  margin: 0;
  font-size: 1.5rem;
}

.main-nav {
  margin-top: 0.5rem;
  display: flex;
  gap: 1rem;
}

.nav-link {
  color: white;
  text-decoration: none;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  transition: background-color 0.3s;
}

.nav-link:hover,
.router-link-active {
  background-color: rgba(255, 255, 255, 0.2);
}

main {
  padding: 2rem 0;
  min-height: calc(100vh - 170px);
}

.app-footer {
  background-color: #333;
  color: white;
  padding: 1rem 0;
  margin-top: 2rem;
}
</style> 