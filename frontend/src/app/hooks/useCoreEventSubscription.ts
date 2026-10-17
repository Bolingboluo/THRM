import { useEffect } from 'react';
import { SetCoreEventSubscription } from '../../../wailsjs/go/main/App';

// 窗口不可见（最小化或收进托盘）时暂停高频遥测：GUI 不再把风扇/温度推送转给页面，
// 并向核心退订，核心便回到后台低频采样。窗口重新可见时恢复订阅，GUI 会立即补推一份
// 完整的温度（含传感器清单）与风扇数据。
//
// 噪音测试、散热收益测试这类在后台也要实时遥测的流程用 holdCoreTelemetry 占住订阅，
// 期间即使窗口被隐藏也不退订。

let holds = 0;
let lastSent: boolean | null = null;
// Wails 在各自的协程里并发执行绑定调用，快速连续的隐藏/显示若同时在途，后端可能
// 按相反的顺序生效，最终停在"已暂停"而窗口其实可见。因此同一时间只发一个调用，
// 期间的变化记为 dirty，等它返回后按当时的最新状态再同步一次。
let inFlight = false;
let dirty = false;

function syncSubscription() {
  if (typeof document === 'undefined') return;
  if (inFlight) {
    dirty = true;
    return;
  }
  const active = !document.hidden || holds > 0;
  if (active === lastSent) return;
  lastSent = active;
  inFlight = true;
  Promise.resolve()
    .then(() => SetCoreEventSubscription(active))
    .catch(() => {
      // 非 Wails 环境（例如纯浏览器预览）没有后端绑定；其余失败在下一次变化时重试。
      lastSent = null;
    })
    .finally(() => {
      inFlight = false;
      if (dirty) {
        dirty = false;
        syncSubscription();
      }
    });
}

// holdCoreTelemetry 在返回的释放函数被调用前保持实时遥测，不受窗口可见性影响。
export function holdCoreTelemetry(): () => void {
  holds += 1;
  syncSubscription();
  let released = false;
  return () => {
    if (released) return;
    released = true;
    holds -= 1;
    syncSubscription();
  };
}

export function useCoreEventSubscription() {
  useEffect(() => {
    syncSubscription();
    document.addEventListener('visibilitychange', syncSubscription);
    return () => document.removeEventListener('visibilitychange', syncSubscription);
  }, []);
}
