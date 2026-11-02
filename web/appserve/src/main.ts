import { createApp } from 'vue';
// Not anfra-design's index.css: its class layer is for the SDK's framework-free panel.
import '@holistics/anfra-design/fonts.css';
import '@holistics/anfra-design/tokens.css';
import '@holistics/anfra-design/type.css';
import './theme.css';
import App from './App.vue';

createApp(App).mount('#app');
