'use client';

import { memo } from 'react';
import { Fan } from 'lucide-react';
import clsx from 'clsx';
import { useWindowActive } from '../hooks/useWindowActive';

/**
 * 按转速旋转的风扇图标（顶栏转速徽标与首页风扇卡片共用）。
 *
 * duration 为一圈的秒数，0 表示不转。窗口失焦/不可见时只暂停动画（停在当前角度），
 * 回到前台后接着转，避免透明窗口下合成器一直按刷新率出帧；
 * 系统开启"减少动态效果"时不转（见 globals.css 的 .fan-spin）。
 */
const SpinningFanIcon = memo(function SpinningFanIcon({ duration, className }: { duration: number; className: string }) {
  const windowActive = useWindowActive();
  const spinning = duration > 0;

  return (
    <span
      className={clsx('inline-flex', spinning && 'animate-spin fan-spin')}
      style={spinning ? { animationDuration: `${duration}s`, animationPlayState: windowActive ? 'running' : 'paused' } : undefined}
    >
      <Fan className={className} />
    </span>
  );
});

export default SpinningFanIcon;
