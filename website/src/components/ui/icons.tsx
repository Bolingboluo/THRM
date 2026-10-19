// lucide 1.x 已移除品牌图标，GitHub 等标志单独内联。
export function GithubIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" className={className}>
      <path d="M12 .5a11.5 11.5 0 0 0-3.64 22.41c.58.1.79-.25.79-.56v-2c-3.2.7-3.88-1.37-3.88-1.37-.53-1.33-1.28-1.69-1.28-1.69-1.05-.71.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.55-.29-5.24-1.28-5.24-5.68 0-1.26.45-2.28 1.19-3.09-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.17 1.18a11 11 0 0 1 5.77 0c2.2-1.49 3.17-1.18 3.17-1.18.63 1.59.23 2.76.11 3.05.74.81 1.19 1.83 1.19 3.09 0 4.41-2.69 5.39-5.25 5.67.41.36.78 1.06.78 2.14v3.17c0 .31.21.67.8.56A11.5 11.5 0 0 0 12 .5Z" />
    </svg>
  );
}

export function WindowsIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" className={className}>
      <path d="M3 5.1 10.4 4v7.1H3V5.1Zm8.3-1.2L21 2.5v8.6h-9.7V3.9ZM3 12.9h7.4V20L3 18.9v-6Zm8.3 0H21v8.6l-9.7-1.4v-7.2Z" />
    </svg>
  );
}

export function LinuxIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" className={className}>
      <path d="M12 3c-2.2 0-3.4 1.9-3.4 4.4 0 1.6.3 2.4-.9 4.1C6.5 13.2 5 15.3 5 17.6c0 .6.1 1.1.3 1.5" />
      <path d="M12 3c2.2 0 3.4 1.9 3.4 4.4 0 1.6-.3 2.4.9 4.1 1.2 1.7 2.7 3.8 2.7 6.1 0 .6-.1 1.1-.3 1.5" />
      <path d="M8.2 19.6c.9.9 2.3 1.4 3.8 1.4s2.9-.5 3.8-1.4" />
      <circle cx="10.4" cy="7.6" r=".6" fill="currentColor" />
      <circle cx="13.6" cy="7.6" r=".6" fill="currentColor" />
      <path d="M10.6 10.1c.4.4.9.6 1.4.6s1-.2 1.4-.6" />
    </svg>
  );
}
