import type { InfiniteData } from '@tanstack/react-query';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiClient, API_BASE_URL } from '../client';
import { logger } from '@/lib/logger';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

/**
 * 尝试状态
 */
export type AttemptStatus = 'success' | 'failed' | 'circuit_break' | 'skipped';

/**
 * 单次渠道尝试信息
 */
export interface ChannelAttempt {
    channel_id: number;
    channel_key_id?: number;
    channel_name: string;
    model_name: string;
    attempt_num: number;    // 第几次尝试
    status: AttemptStatus;
    duration: number;       // 耗时(毫秒)
    sticky?: boolean;
    msg?: string;
}

/**
 * 日志数据
 */
export interface RelayLog {
    id: number;
    time: number;                // 时间戳
    request_model_name: string;  // 请求模型名称
    request_api_key_name?: string; // 请求使用的 API Key 名称
    channel: number;             // 实际使用的渠道ID
    channel_name: string;        // 渠道名称
    actual_model_name: string;   // 实际使用模型名称
    input_tokens: number;        // 输入Token
    output_tokens: number;       // 输出Token
    ftut: number;                // 首字时间(毫秒)
    use_time: number;            // 总用时(毫秒)
    cost: number;                // 消耗费用
    request_content: string;     // 请求内容
    response_content: string;    // 响应内容
    error: string;               // 错误信息
    attempts?: ChannelAttempt[]; // 所有尝试记录
    total_attempts?: number;     // 总尝试次数
    username?: string;            // 调用方用户名（后台管理员登录名）
    log_type?: string;           // 日志类型：relay=转发日志，test=渠道测试日志
}

/**
 * 日志列表查询参数
 */
export interface LogListParams {
    page?: number;
    page_size?: number;
    start_time?: number;
    end_time?: number;
}

/**
 * 清空日志 Hook
 * 
 * @example
 * const clearLogs = useClearLogs();
 * 
 * clearLogs.mutate();
 */
export function useClearLogs() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async () => {
            return apiClient.delete<null>('/api/v1/log/clear');
        },
        onSuccess: () => {
            logger.log('日志清空成功');
            queryClient.invalidateQueries({ queryKey: ['logs'] });
        },
        onError: (error) => {
            logger.error('日志清空失败:', error);
        },
    });
}

const logsInfiniteQueryKey = (pageSize: number) => ['logs', 'infinite', pageSize] as const;
const channelLogsQueryKey = (page: number, pageSize: number) => ['channel-logs', 'page', page, pageSize] as const;

/**
 * 渠道日志列表分页响应
 */
export interface ChannelLogPage {
    list: RelayLog[];
    total: number;
}

/**
 * 渠道日志管理 Hook（分组日志 + 渠道测试日志）
 * 整合分页查询、SSE 实时推送；默认每页 20 条，支持 20/50/100 条切换
 *
 * @example
 * const { logs, total, page, setPage, pageSize, setPageSize, isConnected } = useChannelLogs();
 *
 * // 切换每页条数（自动回到第 1 页）
 * setPageSize(50);
 *
 * // 翻页
 * setPage(page + 1);
 */
export function useChannelLogs(options: { pageSize?: number } = {}) {
    const [page, setPage] = useState(1);
    const [pageSize, setPageSizeState] = useState(options.pageSize ?? 20);

    const pageRef = useRef(page);
    pageRef.current = page;
    const pageSizeRef = useRef(pageSize);
    pageSizeRef.current = pageSize;

    const setPageSize = useCallback((size: number) => {
        setPageSizeState(size);
        setPage(1);
    }, []);

    const [isConnected, setIsConnected] = useState(false);
    const [error, setError] = useState<Error | null>(null);
    const eventSourceRef = useRef<EventSource | null>(null);

    const queryClient = useQueryClient();

    const logsQuery = useQuery({
        queryKey: channelLogsQueryKey(page, pageSize),
        queryFn: async () => {
            const params = new URLSearchParams();
            params.set('page', String(page));
            params.set('page_size', String(pageSize));
            const result = await apiClient.get<ChannelLogPage | null>(`/api/v1/log/channel-list?${params.toString()}`);
            return { list: result?.list ?? [], total: result?.total ?? 0 };
        },
        staleTime: Infinity,
        refetchOnMount: 'always',
    });

    const logs = useMemo(() => {
        return logsQuery.data?.list ?? [];
    }, [logsQuery.data]);

    const total = logsQuery.data?.total ?? 0;

    useEffect(() => {
        let cancelled = false;

        const connect = async () => {
            try {
                const { token } = await apiClient.get<{ token: string }>('/api/v1/log/stream-token');
                if (cancelled) return;

                const eventSource = new EventSource(`${API_BASE_URL}/api/v1/log/stream?token=${token}`);
                eventSourceRef.current = eventSource;

                eventSource.onopen = () => {
                    setIsConnected(true);
                    setError(null);
                };

                eventSource.onmessage = (event) => {
                    try {
                        const log: RelayLog = JSON.parse(event.data);
                        // 渠道日志页签显示全部日志（relay + test），不过滤
                        // 分页模式下，仅当停留在第 1 页时把新日志插入头部，总数 +1；其他页不干扰当前分页数据
                        if (pageRef.current !== 1) return;

                        queryClient.setQueryData(
                            channelLogsQueryKey(pageRef.current, pageSizeRef.current),
                            (old: ChannelLogPage | undefined) => {
                                if (!old) return old;

                                const exists = old.list.some((x) => x.id === log.id);
                                if (exists) return old;

                                return {
                                    list: [log, ...old.list].slice(0, pageSizeRef.current),
                                    total: old.total + 1,
                                };
                            }
                        );
                    } catch (e) {
                        logger.error('解析渠道日志数据失败:', e);
                    }
                };

                eventSource.onerror = () => {
                    setIsConnected(false);
                    setError(new Error('SSE 连接断开'));
                    eventSource.close();
                    eventSourceRef.current = null;
                };
            } catch (e) {
                if (cancelled) return;
                setError(e instanceof Error ? e : new Error('获取 stream token 失败'));
                logger.error('获取 stream token 失败:', e);
            }
        };

        connect();

        return () => {
            cancelled = true;
            eventSourceRef.current?.close();
            eventSourceRef.current = null;
            setIsConnected(false);
        };
    }, [queryClient]);

    const clear = useCallback(() => {
        queryClient.removeQueries({ queryKey: channelLogsQueryKey(page, pageSize) });
    }, [page, pageSize, queryClient]);

    return {
        logs,
        total,
        page,
        setPage,
        pageSize,
        setPageSize,
        isConnected,
        error,
        isLoading: logsQuery.isLoading,
        refetch: logsQuery.refetch,
        clear,
    };
}

/**
 * 日志管理 Hook
 * 整合初始加载、SSE 实时推送、滚动加载更多
 * 
 * @example
 * const { logs, isConnected, hasMore, isLoadingMore, loadMore, clear } = useLogs();
 * 
 * // logs 自动包含历史日志和实时日志，按时间倒序
 * logs.forEach(log => console.log(log.request_model_name));
 * 
 * // 滚动到底部时加载更多
 * if (hasMore && !isLoadingMore) loadMore();
 */
export function useLogs(options: { pageSize?: number } = {}) {
    const { pageSize = 20 } = options;

    const [isConnected, setIsConnected] = useState(false);
    const [error, setError] = useState<Error | null>(null);
    const eventSourceRef = useRef<EventSource | null>(null);

    const queryClient = useQueryClient();

    const logsQuery = useInfiniteQuery({
        queryKey: logsInfiniteQueryKey(pageSize),
        initialPageParam: 1,
        queryFn: async ({ pageParam }) => {
            const params = new URLSearchParams();
            params.set('page', String(pageParam));
            params.set('page_size', String(pageSize));
            const result = await apiClient.get<RelayLog[] | null>(`/api/v1/log/list?${params.toString()}`);
            return result ?? [];
        },
        getNextPageParam: (lastPage, allPages) => {
            if (!lastPage || lastPage.length < pageSize) return undefined;
            return allPages.length + 1;
        },
        staleTime: Infinity,
        refetchOnMount: 'always',
    });

    const logs = useMemo(() => {
        const pages = logsQuery.data?.pages ?? [];
        const seen = new Set<number>();
        const merged: RelayLog[] = [];

        for (const page of pages) {
            for (const log of page) {
                if (seen.has(log.id)) continue;
                seen.add(log.id);
                merged.push(log);
            }
        }

        merged.sort((a, b) => b.time - a.time);
        return merged;
    }, [logsQuery.data]);

    const loadMore = useCallback(async () => {
        if (!logsQuery.hasNextPage) return;
        if (logsQuery.isFetchingNextPage) return;

        try {
            await logsQuery.fetchNextPage();
        } catch (e) {
            logger.error('加载更多日志失败:', e);
        }
    }, [logsQuery]);

    useEffect(() => {
        let cancelled = false;

        const connect = async () => {
            try {
                const { token } = await apiClient.get<{ token: string }>('/api/v1/log/stream-token');
                if (cancelled) return;

                const eventSource = new EventSource(`${API_BASE_URL}/api/v1/log/stream?token=${token}`);
                eventSourceRef.current = eventSource;

                eventSource.onopen = () => {
                    setIsConnected(true);
                    setError(null);
                };

                eventSource.onmessage = (event) => {
                    try {
                        const log: RelayLog = JSON.parse(event.data);
                        // 分组日志页签只显示转发日志（relay）与分组测试日志（group_test），过滤掉渠道测试日志（test）
                        if (log.log_type === 'test') return;
                        queryClient.setQueryData(
                            logsInfiniteQueryKey(pageSize),
                            (old: InfiniteData<RelayLog[], number> | undefined) => {
                                if (!old) {
                                    return { pages: [[log]], pageParams: [1] };
                                }

                                const exists = old.pages.some((p) => p?.some((x) => x.id === log.id));
                                if (exists) return old;

                                const firstPage = old.pages[0] ?? [];
                                return { ...old, pages: [[log, ...firstPage], ...old.pages.slice(1)] };
                            }
                        );
                    } catch (e) {
                        logger.error('解析日志数据失败:', e);
                    }
                };

                eventSource.onerror = () => {
                    setIsConnected(false);
                    setError(new Error('SSE 连接断开'));
                    eventSource.close();
                    eventSourceRef.current = null;
                };
            } catch (e) {
                if (cancelled) return;
                setError(e instanceof Error ? e : new Error('获取 stream token 失败'));
                logger.error('获取 stream token 失败:', e);
            }
        };

        connect();

        return () => {
            cancelled = true;
            eventSourceRef.current?.close();
            eventSourceRef.current = null;
            setIsConnected(false);
        };
    }, [pageSize, queryClient]);

    const clear = useCallback(() => {
        queryClient.removeQueries({ queryKey: logsInfiniteQueryKey(pageSize) });
    }, [pageSize, queryClient]);

    return {
        logs,
        isConnected,
        error,
        hasMore: !!logsQuery.hasNextPage,
        isLoading: logsQuery.isLoading,
        isLoadingMore: logsQuery.isFetchingNextPage,
        loadMore,
        clear,
    };
}
