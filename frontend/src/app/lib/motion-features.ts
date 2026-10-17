// framer-motion 的动画特性包（animate / exit / variants / hover·tap 手势），
// 由 AppShell 里的 LazyMotion 异步加载，单独成块，不进首屏 JS。
// 项目里只用到透明度、位移、缩放与 height:auto 的进出场，不需要 layout/drag（domMax）。
import { domAnimation } from 'framer-motion';

export default domAnimation;
