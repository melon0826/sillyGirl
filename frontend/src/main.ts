import { createApp } from 'vue';
import 'ant-design-vue/dist/reset.css';
import App from './App.vue';
import './styles.css';
import { bootWatchdog } from './watchdog';

bootWatchdog('admin');
createApp(App).mount('#root');
