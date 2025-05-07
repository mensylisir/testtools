<template>
  <div class="app-layout">
    <SideMenu :is-menu-collapsed="isMenuCollapsedState" @toggleMenu="isMenuCollapsedState = $event"></SideMenu>
    <div class="main-content-wrapper" :class="{'shifted': !isMenuCollapsedState}">
      <header class="app-header">
        <div class="container header-container">
          <nav class="main-nav">
            <router-link to="/" class="nav-link">首页</router-link>
            <router-link to="/dig" class="nav-link">DNS查询工具</router-link>
            <router-link to="/ping" class="nav-link">连通性测试工具</router-link>
            <router-link to="/fio" class="nav-link">IO性能工具</router-link>
          </nav>
          <div class="right-container">
            <NamespaceSelector @namespace-changed="handleNamespaceChange"/>
          </div>
        </div>
      </header>

      <main class="container main-content-scrollable">
        <router-view/>
      </main>

      <footer class="app-footer">
        <div class="container">
          <p>&copy; 2025 TestTools - Kubernetes 网络测试工具</p>
        </div>
      </footer>
    </div>
  </div>
</template>

<script>
import {ref} from 'vue';
import NamespaceSelector from './components/NamespaceSelector.vue'
import SideMenu from './components/SideMenu.vue'

export default {
  components: {
    NamespaceSelector,
    SideMenu
  },
  setup() {
    const handleNamespaceChange = (namespace) => {
      console.log('命名空间已变更:', namespace)
      window.location.reload()
    };
    const isMenuCollapsedState = ref(false);

    return {
      handleNamespaceChange,
      isMenuCollapsedState
    }
  }
}
</script>

<style>

.app-layout {
  display: flex;
  height: 100vh;
  width: 100%;
  background-color: var(--background-color);
  overflow: hidden;
}

.main-content-wrapper {
  flex-grow: 1;
  display: flex;
  flex-direction: column;
  margin-left: auto;
  transition: margin-left 0.3s ease;
  overflow-x: hidden;
  overflow-y: hidden;
}

.main-content-wrapper.shifted {
  margin-left: auto;
  overflow-x: hidden;
  overflow-y: hidden;
}

.container {
  margin: 0 auto;
  box-sizing: border-box;
  width: 100%;
  padding: 0 20px;
}


.header-container {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.main-content-scrollable.container {
  flex-grow: 1;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 0.5rem 1rem;
}

.app-header {
  background-color: var(--background-color);
  color: var(--text-primary);
  padding: 1rem 0;
  flex-shrink: 0;
  box-shadow: 0 2px 5px rgba(0, 0, 0, 0.1);
  z-index: 50;
}

.app-footer > .container {
  text-align: center;
}

.app-header h1 {
  margin: 0;
  font-size: 1.5rem;
}

.main-nav {
  display: flex;
  gap: 1rem;
}

.nav-link {
  text-decoration: none;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  transition: background-color 0.3s;
  background-color: transparent;
  color: var(--text-primary);
}

.nav-link:hover,
.router-link-active {
  font-weight: bold;
  color: var(--primary-color);
  background-color: var(--primary-color-light);
}

.right-container {
  display: flex;
  align-items: center;
}

main {
  padding: 2rem 0;
  flex-grow: 1;
  width: 100%;
  margin: 0 auto;
  box-sizing: border-box;
}

.app-footer {
  background-color: var(--background-color);
  color: var(--text-primary);
  padding: 1rem 0;
  margin-top: auto;
  flex-shrink: 0;
  width: 100%;
  box-shadow: 0 -2px 5px rgba(0, 0, 0, 0.2);
  z-index: 50;
}
</style>