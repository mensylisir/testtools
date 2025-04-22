import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { initMessageListener } from './utils/messageListener'

import './assets/main.css'

createApp(App)
  .use(router)
  .mount('#app')
initMessageListener()