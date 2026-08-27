import { createApp } from 'vue'
import App from './App.vue'
import './styles.css'
import { installWebStatePersistence } from './statePersistence'

createApp(App).mount('#app')
installWebStatePersistence()
