import { createApp } from 'vue';
import 'ant-design-vue/dist/reset.css';
import UserCenter from './User.vue';
import './styles.css';
import { bootWatchdog } from './watchdog';

bootWatchdog('user');
createApp(UserCenter).mount('#user-root');
