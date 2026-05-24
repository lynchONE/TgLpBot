import { useEffect, useState } from 'react';
import { fetchMe } from '../lib/api';

/**
 * 拉取当前 Telegram 用户的 `/me` 信息。
 *
 * - hasInitData=false 时不发起请求（避免本地调试 401 噪音）
 * - 失败时静默忽略（上层会从 `realtime_positions` 响应里 fallback）
 * - 依赖变化或卸载时 abort
 */
export function useAuthData({ apiBaseUrl, initData, hasInitData }) {
    const [me, setMe] = useState(null);

    useEffect(() => {
        if (!hasInitData) return undefined;
        let aborted = false;
        const controller = new AbortController();

        (async () => {
            try {
                const resp = await fetchMe({ apiBaseUrl, initData, signal: controller.signal });
                if (aborted) return;
                setMe(resp);
            } catch {
                // ignore; fallback to `realtime_positions` response
            }
        })();

        return () => {
            aborted = true;
            controller.abort();
        };
    }, [apiBaseUrl, initData, hasInitData]);

    return { me, setMe };
}

export default useAuthData;
