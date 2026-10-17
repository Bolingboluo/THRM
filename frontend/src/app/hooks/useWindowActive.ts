import { useSyncExternalStore } from 'react';

/*
 * 窗口是否处于"有人在看"的状态：页面可见且窗口持有焦点。
 *
 * Mica/亚克力是透明窗口，任何持续动画（风扇图标旋转、灯带预览）都会让
 * WebView2 合成器和 DWM 每个刷新周期都出一帧，GPU 永远闲不下来。
 * 窗口失焦或被最小化时这些纯装饰动画没人看，暂停即可；重新聚焦后接着动。
 *
 * 所有订阅者共用一组全局监听，不随组件挂载反复增删。
 */

const listeners = new Set<() => void>();
let installed = false;
let active = true;

function computeActive() {
  return document.visibilityState === 'visible' && document.hasFocus();
}

function update() {
  const next = computeActive();
  if (next === active) return;
  active = next;
  listeners.forEach((listener) => listener());
}

function install() {
  if (installed || typeof window === 'undefined') return;
  installed = true;
  active = computeActive();
  window.addEventListener('focus', update);
  window.addEventListener('blur', update);
  document.addEventListener('visibilitychange', update);
}

export function subscribeWindowActive(listener: () => void) {
  install();
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function isWindowActive() {
  install();
  return active;
}

// 服务端预渲染时按"活跃"处理，与客户端首帧一致；真实状态在订阅后立即生效。
function getServerSnapshot() {
  return true;
}

export function useWindowActive() {
  return useSyncExternalStore(subscribeWindowActive, isWindowActive, getServerSnapshot);
}
