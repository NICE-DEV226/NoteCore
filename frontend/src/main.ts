import { mount } from 'svelte';
import App from './App.svelte';
import './fonts.css';
import './app.css';
import { installLinkGuard } from './lib/utils/links';

installLinkGuard();

const app = mount(App, {
  target: document.getElementById('app') as HTMLElement
});

export default app;
