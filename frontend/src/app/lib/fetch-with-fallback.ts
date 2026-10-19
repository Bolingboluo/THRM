/** 单个来源的超时：官网反代挂掉时不要让用户干等，尽快换下一个来源 */
const DEFAULT_TIMEOUT_MS = 8000;

export type FallbackResult<T> = {
  data: T;
  /** 实际成功的来源在 urls 里的下标，0 表示首选来源 */
  sourceIndex: number;
};

/**
 * 按顺序请求 urls，返回第一个成功（2xx 且能解析为 JSON）的结果。
 * 调用方的 signal 被中止时立即停止，不再尝试后面的来源。
 */
export async function fetchJsonWithFallback<T>(
  urls: readonly string[],
  init: RequestInit & { timeoutMs?: number } = {},
): Promise<FallbackResult<T>> {
  const { timeoutMs = DEFAULT_TIMEOUT_MS, signal, ...rest } = init;
  let lastError: unknown = new Error('no source');

  for (let i = 0; i < urls.length; i++) {
    if (signal?.aborted) throw signal.reason ?? new DOMException('Aborted', 'AbortError');

    const controller = new AbortController();
    const abort = () => controller.abort(signal?.reason);
    signal?.addEventListener('abort', abort, { once: true });
    const timer = window.setTimeout(() => controller.abort(new DOMException('Timeout', 'TimeoutError')), timeoutMs);

    try {
      const response = await fetch(urls[i], { ...rest, signal: controller.signal });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      return { data: (await response.json()) as T, sourceIndex: i };
    } catch (error) {
      lastError = error;
      if (signal?.aborted) throw error;
    } finally {
      window.clearTimeout(timer);
      signal?.removeEventListener('abort', abort);
    }
  }

  throw lastError;
}
