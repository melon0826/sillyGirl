import { createApp } from 'vue';
import 'ant-design-vue/dist/reset.css';
import Home from './Home.vue';
import './styles.css';
import { bootWatchdog } from './watchdog';

bootWatchdog('home');
createApp(Home).mount('#home-root');
