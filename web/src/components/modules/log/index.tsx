import { useCallback, useMemo, useState } from 'react';
import { useLogs, useChannelLogs, type RelayLog } from '@/api/endpoints/log';
import { LogCard } from './Item';
import { Loader2, ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight, RefreshCw } from 'lucide-react';
import { useTranslations } from 'use-intl';
import { VirtualizedGrid } from '@/components/common/VirtualizedGrid';
import { Tabs, TabsList, TabsTrigger } from '@/components/animate-ui/primitives/animate/tabs';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table';
import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/animate-ui/components/animate/tooltip';
import { cn, maskKey } from '@/lib/utils';

/**
 * 分组日志页签组件
 * - 初始加载 pageSize 条历史日志
 * - SSE 实时推送新日志
 * - 滚动自动加载更多
 */
function GroupLogs() {
    const t = useTranslations('log');
    const { logs, hasMore, isLoading, isLoadingMore, loadMore } = useLogs({ pageSize: 10 });

    const canLoadMore = hasMore && !isLoading && !isLoadingMore && logs.length > 0;
    const handleReachEnd = useCallback(() => {
        if (!canLoadMore) return;
        void loadMore();
    }, [canLoadMore, loadMore]);

    const footer = useMemo(() => {
        if (hasMore && (isLoading || isLoadingMore)) {
            return (
                <div className="flex justify-center py-4">
                    <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                </div>
            );
        }
        if (!hasMore && logs.length > 0) {
            return (
                <div className="flex justify-center py-4">
                    <span className="text-sm text-muted-foreground">{t('list.noMore')}</span>
                </div>
            );
        }
        return null;
    }, [hasMore, isLoading, isLoadingMore, logs.length, t]);

    return (
        <VirtualizedGrid
            items={logs}
            layout="list"
            columns={{ default: 1 }}
            estimateItemHeight={80}
            overscan={8}
            getItemKey={(log) => `log-${log.id}`}
            renderItem={(log) => <LogCard log={log} />}
            footer={footer}
            onReachEnd={handleReachEnd}
            reachEndEnabled={canLoadMore}
            reachEndOffset={2}
        />
    );
}

/**
 * 渠道日志表格行组件
 */
function ChannelLogRow({ log }: { log: RelayLog }) {
    const t = useTranslations('log');
    
    // 格式化金额
    const formatCost = (cost: number) => {
        return `$${cost.toFixed(6)}`;
    };

    // 格式化时间
    const formatTime = (timestamp: number) => {
        return new Date(timestamp * 1000).toLocaleString();
    };

    return (
        <TableRow>
            <TableCell className="whitespace-nowrap">{formatTime(log.time)}</TableCell>
            <TableCell className="whitespace-nowrap">{log.channel_name}</TableCell>
            <TableCell className="whitespace-nowrap">{log.username || '-'}</TableCell>
            <TableCell className="whitespace-nowrap">{maskKey(log.request_api_key_name) || '-'}</TableCell>
            <TableCell className="whitespace-nowrap">{log.actual_model_name || log.request_model_name}</TableCell>
            <TableCell className="whitespace-nowrap text-center">{log.input_tokens}</TableCell>
            <TableCell className="whitespace-nowrap text-center">{log.output_tokens}</TableCell>
            <TableCell className="whitespace-nowrap">{formatCost(log.cost)}</TableCell>
        </TableRow>
    );
}

/**
 * 渠道日志分页栏组件
 * - 显示总数、首页/末页、上一页/下一页、页码、跳转指定页、每页条数选择（20/50/100）
 */
function ChannelLogPagination({
    total,
    page,
    totalPages,
    pageSize,
    onPageChange,
    onPageSizeChange,
    onRefresh,
    refreshing,
}: {
    total: number;
    page: number;
    totalPages: number;
    pageSize: number;
    onPageChange: (page: number) => void;
    onPageSizeChange: (size: number) => void;
    onRefresh?: () => void;
    refreshing?: boolean;
}) {
    const t = useTranslations('log');
    const [jumpValue, setJumpValue] = useState('');

    const handleJump = () => {
        const n = parseInt(jumpValue, 10);
        if (isNaN(n) || n < 1 || n > totalPages) return;
        if (n !== page) onPageChange(n);
        setJumpValue('');
    };

    return (
        <div className="flex items-center justify-between gap-4 border-t border-border px-4 py-2.5">
            <span className="text-xs text-muted-foreground">
                {t('channelList.total', { total })}
            </span>
            <div className="flex items-center gap-1.5">
                <Button
                    variant="outline"
                    size="sm"
                    className="h-7 rounded-lg px-2"
                    disabled={page <= 1}
                    onClick={() => onPageChange(1)}
                >
                    <ChevronsLeft className="h-3.5 w-3.5" />
                    <span className="hidden sm:inline">{t('channelList.firstPage')}</span>
                </Button>
                <Button
                    variant="outline"
                    size="sm"
                    className="h-7 rounded-lg px-2"
                    disabled={page <= 1}
                    onClick={() => onPageChange(page - 1)}
                >
                    <ChevronLeft className="h-3.5 w-3.5" />
                    <span className="hidden sm:inline">{t('channelList.prevPage')}</span>
                </Button>
                <span className="text-xs text-muted-foreground whitespace-nowrap">
                    {t('channelList.pageInfo', { page, pages: totalPages })}
                </span>
                <Button
                    variant="outline"
                    size="sm"
                    className="h-7 rounded-lg px-2"
                    disabled={page >= totalPages}
                    onClick={() => onPageChange(page + 1)}
                >
                    <span className="hidden sm:inline">{t('channelList.nextPage')}</span>
                    <ChevronRight className="h-3.5 w-3.5" />
                </Button>
                <Button
                    variant="outline"
                    size="sm"
                    className="h-7 rounded-lg px-2"
                    disabled={page >= totalPages}
                    onClick={() => onPageChange(totalPages)}
                >
                    <span className="hidden sm:inline">{t('channelList.lastPage')}</span>
                    <ChevronsRight className="h-3.5 w-3.5" />
                </Button>
                <div className="flex items-center gap-1 ml-1">
                    <input
                        type="text"
                        inputMode="numeric"
                        value={jumpValue}
                        onChange={(e) => setJumpValue(e.target.value.replace(/[^0-9]/g, ''))}
                        onKeyDown={(e) => { if (e.key === 'Enter') handleJump(); }}
                        placeholder={t('channelList.jumpPlaceholder')}
                        className="h-7 w-12 rounded-lg border border-input bg-background px-1.5 text-xs text-center focus:outline-none focus:ring-1 focus:ring-ring"
                    />
                    <Button
                        variant="outline"
                        size="sm"
                        className="h-7 rounded-lg px-2 text-xs"
                        disabled={!jumpValue}
                        onClick={handleJump}
                    >
                        {t('channelList.jump')}
                    </Button>
                </div>
            </div>
            <div className="flex items-center gap-1.5">
                <span className="text-xs text-muted-foreground">{t('channelList.pageSize')}</span>
                <Select value={String(pageSize)} onValueChange={(v) => onPageSizeChange(Number(v))}>
                    <SelectTrigger className="h-7 w-[70px] rounded-lg text-xs">
                        <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="rounded-lg">
                        <SelectItem value="20" className="rounded-lg text-xs">20</SelectItem>
                        <SelectItem value="50" className="rounded-lg text-xs">50</SelectItem>
                        <SelectItem value="100" className="rounded-lg text-xs">100</SelectItem>
                    </SelectContent>
                </Select>
                <span className="text-xs text-muted-foreground">{t('channelList.pageSizeUnit')}</span>
                {onRefresh && (
                    <Tooltip side="top" sideOffset={10} align="center">
                        <TooltipTrigger asChild>
                            <button
                                type="button"
                                onClick={onRefresh}
                                disabled={refreshing}
                                aria-label={t('channelList.refresh')}
                                className="ml-1 p-1.5 rounded-lg transition-colors hover:bg-muted text-muted-foreground hover:text-foreground disabled:opacity-50"
                            >
                                <RefreshCw className={cn('size-4', refreshing && 'animate-spin')} />
                            </button>
                        </TooltipTrigger>
                        <TooltipContent>{t('channelList.refresh')}</TooltipContent>
                    </Tooltip>
                )}
            </div>
        </div>
    );
}

/**
 * 渠道日志页签组件
 * - 分页展示：默认每页 20 条，支持 20/50/100 条切换、上一页/下一页翻页
 * - SSE 实时推送新日志（仅第 1 页实时插入）
 */
function ChannelLogs() {
    const t = useTranslations('log');
    const { logs, total, page, setPage, pageSize, setPageSize, isLoading, error, refetch } = useChannelLogs({ pageSize: 20 });

    const [refreshing, setRefreshing] = useState(false);
    const handleRefresh = useCallback(async () => {
        if (refreshing) return;
        setRefreshing(true);
        try {
            await refetch();
        } finally {
            setRefreshing(false);
        }
    }, [refreshing, refetch]);

    const totalPages = useMemo(() => Math.max(1, Math.ceil(total / pageSize)), [total, pageSize]);

    return (
        <div className="flex h-full flex-col overflow-hidden">
            {error ? (
                <div className="flex flex-col items-center justify-center py-12 gap-2 text-muted-foreground">
                    <span className="text-sm">{t('channelList.loadError')}</span>
                    <span className="text-xs text-destructive">{error.message}</span>
                </div>
            ) : (
                <>
                    <div className="min-h-0 flex-1 overflow-auto">
                        <Table>
                            <TableHeader>
                                <TableRow>
                                    <TableHead>{t('channelList.time')}</TableHead>
                                    <TableHead>{t('channelList.channel')}</TableHead>
                                    <TableHead>{t('channelList.username')}</TableHead>
                                    <TableHead>{t('channelList.tokenName')}</TableHead>
                                    <TableHead>{t('channelList.model')}</TableHead>
                                    <TableHead className="text-center">{t('channelList.inputTokens')}</TableHead>
                                    <TableHead className="text-center">{t('channelList.outputTokens')}</TableHead>
                                    <TableHead>{t('channelList.cost')}</TableHead>
                                </TableRow>
                            </TableHeader>
                            <TableBody>
                                {logs.map((log) => (
                                    <ChannelLogRow key={log.id} log={log} />
                                ))}
                            </TableBody>
                        </Table>
                        {logs.length === 0 && isLoading && (
                            <div className="flex justify-center py-8 text-muted-foreground">
                                <Loader2 className="h-5 w-5 animate-spin" />
                            </div>
                        )}
                        {logs.length === 0 && !isLoading && (
                            <div className="flex justify-center py-8 text-muted-foreground">
                                {t('list.noData')}
                            </div>
                        )}
                    </div>
                    {totalPages > 1 || logs.length > 0 ? (
                        <ChannelLogPagination
                            total={total}
                            page={page}
                            totalPages={totalPages}
                            pageSize={pageSize}
                            onPageChange={setPage}
                            onPageSizeChange={setPageSize}
                            onRefresh={handleRefresh}
                            refreshing={refreshing}
                        />
                    ) : null}
                </>
            )}
        </div>
    );
}

/**
 * 日志页面主组件 - Tabs 双页签
 * - 分组日志：现有内容（SSE 实时推送）
 * - 渠道日志：渠道调用记录表格
 *
 * 注意：这里不使用 TabsContents/TabsContent（它们会用内容高度撑开容器并 overflow:hidden，
 * 导致长列表内部滚动失效）。改为由本组件锁定内容区高度（flex h-full min-h-0 flex-col），
 * 子列表在自己的滚动容器内滚动。
 */
export function Log() {
    const t = useTranslations('log');
    const [activeTab, setActiveTab] = useState('group');

    return (
        <Tabs value={activeTab} onValueChange={setActiveTab} className="flex h-full min-h-0 flex-col">
            <TabsList className="mb-4 flex w-fit shrink-0 gap-1 rounded-2xl bg-muted p-1">
                <TabsTrigger
                    value="group"
                    className="flex items-center justify-center gap-2 rounded-xl px-5 py-2 text-sm font-medium transition-colors data-[state=active]:bg-primary data-[state=active]:text-primary-foreground data-[state=inactive]:text-muted-foreground data-[state=inactive]:hover:bg-muted/80 data-[state=inactive]:hover:text-foreground"
                >
                    {t('tabs.group')}
                </TabsTrigger>
                <TabsTrigger
                    value="channel"
                    className="flex items-center justify-center gap-2 rounded-xl px-5 py-2 text-sm font-medium transition-colors data-[state=active]:bg-primary data-[state=active]:text-primary-foreground data-[state=inactive]:text-muted-foreground data-[state=inactive]:hover:bg-muted/80 data-[state=inactive]:hover:text-foreground"
                >
                    {t('tabs.channel')}
                </TabsTrigger>
            </TabsList>
            <div className="min-h-0 flex-1">
                {activeTab === 'group' ? <GroupLogs /> : <ChannelLogs />}
            </div>
        </Tabs>
    );
}
