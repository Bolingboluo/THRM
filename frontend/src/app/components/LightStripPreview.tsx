import { useLayoutEffect, useRef } from 'react';
import clsx from 'clsx';
import {
  LIGHT_SUBSTEP_MS,
  STRIP_SAMPLE_COUNT,
  averageColor,
  rgbToCss,
  sampleStripColors,
  stripGradient,
  type LightProgram,
  type RGB,
} from '../lib/lightProgram';
import { isWindowActive, subscribeWindowActive } from '../hooks/useWindowActive';

const STRIP_HIGHLIGHT =
  'linear-gradient(180deg, rgba(255,255,255,0.32) 0%, rgba(255,255,255,0.06) 42%, rgba(0,0,0,0.10) 100%)';
const UNLIT_SHADOW = 'inset 0 0 0 1px color-mix(in srgb, currentColor 12%, transparent)';

const OFF_STRIP: RGB[] = Array.from({ length: STRIP_SAMPLE_COUNT }, () => ({ r: 0, g: 0, b: 0 }));

function stripStyle(strip: RGB[]) {
  const glow = averageColor(strip);
  const lit = glow.r + glow.g + glow.b > 12;
  return {
    backgroundImage: [STRIP_HIGHLIGHT, stripGradient(strip)].join(', '),
    boxShadow: lit ? `0 0 18px 3px ${rgbToCss(glow)}` : UNLIT_SHADOW,
  };
}

// 首帧（含预渲染）用的熄灭态样式，之后由动画循环直接写 DOM。
const OFF_STYLE = stripStyle(OFF_STRIP);

/**
 * 两次重绘的最小间隔。颜色本身按真实经过时间取样（与帧数无关），这里只决定多久取一次样：
 * 上限约 30fps，但保证一次关键帧过渡至少取样 15 次，最快的智能温控预设（200ms 一次过渡）
 * 因此仍按显示器刷新率逐帧绘制，观感与原先一致。
 */
function minFrameInterval(program: LightProgram) {
  const transitionMs = Math.max(1, program.transitionTicks * 10) * LIGHT_SUBSTEP_MS;
  return Math.min(1000 / 30, transitionMs / 15);
}

/**
 * 设备灯带的实时预览。
 *
 * 设备端是一整条带导光罩的灯带，看到的是一条连续的光，不是六个分开的点。
 * 所以这里沿灯条连续取样上色（sampleStripColors），位置与时间两个方向都插值，
 * 亮光在灯条上是无级滑动的。颜色本身仍由 lightProgram.ts 复刻的固件关键帧算出。
 *
 * 实物灯带约 4.5cm 长、0.4cm 宽，因此预览按同样的长宽比渲染并居中，
 * 而不是撑满整个面板宽度——撑满会把它拉成一条与实物完全不像的细长条。
 */
export default function LightStripPreview({
  program,
  className,
}: {
  program: LightProgram | null;
  className?: string;
}) {
  const stripRef = useRef<HTMLDivElement | null>(null);

  // 动画每帧直接改 DOM 样式，不经过 React state：省掉每秒几十次的组件重渲染，
  // 而且内容没变（同一子步、熄灭段）时连样式都不写，浏览器也就不用重绘这圈光晕。
  // 窗口失焦/不可见或灯带滚出视口时暂停循环；相位按真实时间计算，恢复后直接接上。
  useLayoutEffect(() => {
    const el = stripRef.current;
    if (!el) return;

    let lastBackground = '';
    let lastShadow = '';
    const draw = (strip: RGB[]) => {
      const { backgroundImage, boxShadow } = stripStyle(strip);
      if (backgroundImage !== lastBackground) {
        el.style.backgroundImage = backgroundImage;
        lastBackground = backgroundImage;
      }
      if (boxShadow !== lastShadow) {
        el.style.boxShadow = boxShadow;
        lastShadow = boxShadow;
      }
    };

    if (!program) {
      draw(OFF_STRIP);
      return;
    }

    // 静态程序只有一帧，没必要每帧重算。
    if (program.lastKeyframe === 0) {
      draw(sampleStripColors(program, 0));
      return;
    }

    const start = performance.now();
    const interval = minFrameInterval(program);
    // 留一点余量，避免 vsync 抖动让 30fps 掉成 20fps。
    const intervalSlack = Math.min(4, interval / 4);
    let frame: number | null = null;
    let lastDrawAt = -Infinity;
    let onScreen = true;

    const tick = (now: number) => {
      frame = requestAnimationFrame(tick);
      if (now - lastDrawAt < interval - intervalSlack) return;
      lastDrawAt = now;
      draw(sampleStripColors(program, now - start));
    };

    const sync = () => {
      const shouldRun = onScreen && isWindowActive();
      if (shouldRun && frame === null) {
        lastDrawAt = -Infinity;
        frame = requestAnimationFrame(tick);
      } else if (!shouldRun && frame !== null) {
        cancelAnimationFrame(frame);
        frame = null;
      }
    };

    draw(sampleStripColors(program, 0));

    const unsubscribe = subscribeWindowActive(sync);
    let observer: IntersectionObserver | null = null;
    if (typeof IntersectionObserver !== 'undefined') {
      observer = new IntersectionObserver((entries) => {
        const entry = entries[entries.length - 1];
        if (!entry) return;
        onScreen = entry.isIntersecting;
        sync();
      });
      observer.observe(el);
    }
    sync();

    return () => {
      unsubscribe();
      observer?.disconnect();
      if (frame !== null) cancelAnimationFrame(frame);
      frame = null;
    };
  }, [program]);

  return (
    <div className={clsx('flex justify-center', className)}>
      {/* 只画灯带本身：外面再套一圈深色外壳只会变成一条黑边，实物没有这个东西。
          横向是灯珠的光晕叠加，纵向叠一层高光，看起来才像亮着的导光条。
          背景与光晕由上面的动画循环直接写入，这里只给首帧一个熄灭态。 */}
      <div
        ref={stripRef}
        className="aspect-[45/4] w-full max-w-[200px] rounded-full"
        style={OFF_STYLE}
      />
    </div>
  );
}
