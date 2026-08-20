import {
    MorphingDialog,
    MorphingDialogTrigger,
    MorphingDialogContainer,
    MorphingDialogContent,
} from '@/components/ui/morphing-dialog';
import { CheckCircle2, DollarSign, FlaskConical, Key, Layers, Loader2, MessageSquare, XCircle } from 'lucide-react';
import { type StatsMetricsFormatted } from '@/api/endpoints/stats';
import { type Channel, type TestModelResponse, useEnableChannel, useTestModel } from '@/api/endpoints/channel';
import { CardContent } from './CardContent';
import { useTranslations } from 'use-intl';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/animate-ui/components/animate/tooltip';
import { Switch } from '@/components/ui/switch';
import { Button } from '@/components/ui/button';
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@/components/ui/select';
import { Popover, PopoverTrigger, PopoverContent } from '@/components/ui/popover';
import { toast } from '@/components/common/Toast';
import { formatMoney } from '@/lib/utils';
import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

// localStorage 持久化前缀：记住每个渠道上次测试选择的模型
const TEST_MODEL_STORAGE_KEY = 'octopus:channel:test-model:';

/**
 * 从后端 test-model 返回的 message 中提取真实错误原因。
 * 后端失败时 message 可能是扁平 JSON（如 {"code":"INSUFFICIENT_BALANCE","message":"余额不足","traceId":"..."}）
 * 也可能是纯文本；这里一律提取出人类可读的原因，提取不到则原样返回。
 */
function extractTestError(raw: string): string {
    const fallback = raw?.trim() || '未知错误';
    if (!raw) return fallback;
    try {
        const parsed = JSON.parse(raw);
        if (parsed && typeof parsed === 'object') {
            const candidate = parsed.message ?? parsed.error ?? parsed.error_message ?? parsed.code;
            if (typeof candidate === 'string' && candidate.trim()) return candidate.trim();
        }
    } catch {
        // 非 JSON，直接按纯文本处理
    }
    return fallback;
}

export function Card({ channel, stats, layout = 'grid' }: { channel: Channel; stats: StatsMetricsFormatted; layout?: 'grid' | 'list' }) {
    const t = useTranslations('channel.card');
    const tForm = useTranslations('channel.form');
    const tSections = useTranslations('channel.detail.sections');
    const tMetrics = useTranslations('channel.detail.metrics');
    const enableChannel = useEnableChannel();
    const testModel = useTestModel();
    const queryClient = useQueryClient();
    const isListLayout = layout === 'list';

    const splitModels = (models: string) =>
        models
            .split(',')
            .map((item) => item.trim())
            .filter(Boolean);

    // 当前渠道已配置的模型（model + custom_model 合并去重），供下拉选择
    const modelOptions = [...new Set([...splitModels(channel.model), ...splitModels(channel.custom_model)])];

    // 模型选择持久化到 localStorage（按渠道隔离），刷新/重进页面后自动恢复上次选择
    const [selectedModel, setSelectedModel] = useState<string>(() => {
        try {
            const saved = localStorage.getItem(`${TEST_MODEL_STORAGE_KEY}${channel.id}`);
            return saved && modelOptions.includes(saved) ? saved : '';
        } catch {
            return '';
        }
    });

    const handleModelChange = (model: string) => {
        setSelectedModel(model);
        try {
            localStorage.setItem(`${TEST_MODEL_STORAGE_KEY}${channel.id}`, model);
        } catch {
            // localStorage 不可用时静默降级为内存态
        }
    };

    // 测试按钮的选项菜单开关（测试当前渠道 / 测试所有 key）
    const [testMenuOpen, setTestMenuOpen] = useState(false);

    const modelCount = new Set([
        ...splitModels(channel.model),
        ...splitModels(channel.custom_model),
    ]).size;
    const enabledKeyCount = channel.keys.filter((item) => item.enabled).length;

    // 渠道所有 key 设置的额度之和（$），用于总成本分数展示：消耗/总额度
    const totalQuota = channel.keys.reduce((sum, k) => sum + (k.quota ?? 0), 0);
    // 消耗金额用所有 key 的 total_cost 之和（与额度同一数据源），
    // 这样测试/转发产生的费用都会实时反映到卡片上；stats.total_cost 仅统计转发，不含测试。
    const totalCost = channel.keys.reduce((sum, k) => sum + (k.total_cost ?? 0), 0);
    const quotaCostLabel = totalQuota > 0
        ? `${formatMoney(totalCost).formatted.value}${formatMoney(totalCost).formatted.unit}/${formatMoney(totalQuota).formatted.value}${formatMoney(totalQuota).formatted.unit}`
        : `${formatMoney(totalCost).formatted.value}${formatMoney(totalCost).formatted.unit}`;

    const handleEnableChange = (checked: boolean) => {
        enableChannel.mutate(
            { id: channel.id, enabled: checked },
            {
                onSuccess: () => {
                    toast.success(checked ? t('toast.enabled') : t('toast.disabled'));
                },
                onError: (error) => {
                    toast.error(error.message);
                },
            }
        );
    };

    const handleTestModel = () => {
        if (!selectedModel) {
            toast.warning('请先选择模型', {
                description: `渠道「${channel.name}」请在模型下拉框中选择要测试的模型`,
                position: 'top-left',
            });
            return;
        }
        testModel.mutate(
            { channel_id: channel.id, model: selectedModel },
            {
                onSuccess: (data) => {
                    if (data.success) {
                        // 成功提示：硬编码中文，不依赖 i18n 解析
                        toast.success(
                            '测试成功',
                            {
                                description: `渠道「${channel.name}」· 模型 ${selectedModel} · 耗时 ${((data.latency_ms ?? 0) / 1000).toFixed(2)} 秒`,
                                position: 'top-left',
                            }
                        );
                    } else {
                        // 失败提示：直接显示后端真实错误原因（如「余额不足」），绝不回退成键名
                        const reason = extractTestError(data.message);
                        toast.error(
                            '测试失败',
                            {
                                description: `渠道「${channel.name}」· 模型 ${selectedModel}\n原因：${reason}`,
                                position: 'top-left',
                            }
                        );
                    }
                },
                onError: (error) => {
                    const reason = error.message ? extractTestError(error.message) : '请求异常';
                    toast.error(
                        '测试失败',
                        {
                            description: `渠道「${channel.name}」· 模型 ${selectedModel}\n原因：${reason}`,
                            position: 'top-left',
                        }
                    );
                },
                // 无论测试成功/失败，都重新拉取渠道列表：
                // 测试可能触发后端自动停用（如余额不足），需要让列表与编辑界面立即反映最新状态。
                onSettled: () => {
                    queryClient.invalidateQueries({ queryKey: ['channels', 'list'] });
                },
            }
        );
    };

    // 逐个测试渠道下所有启用的 key，展示通过/失败汇总及失败原因
    const handleTestAllKeys = () => {
        if (!selectedModel) {
            toast.warning('请先选择模型', {
                description: `渠道「${channel.name}」请在模型下拉框中选择要测试的模型`,
                position: 'top-left',
            });
            return;
        }
        testModel.mutate(
            { channel_id: channel.id, model: selectedModel, test_all_keys: true },
            {
                onSuccess: (data) => {
                    const total = data.total ?? 0;
                    const passed = data.passed ?? 0;
                    if (data.success && total > 0) {
                        toast.success('测试完成', {
                            description: `渠道「${channel.name}」· 模型 ${selectedModel}\n${passed}/${total} 个 key 全部通过`,
                            position: 'top-left',
                        });
                        return;
                    }
                    if (total === 0) {
                        toast.error('测试失败', {
                            description: `渠道「${channel.name}」· 模型 ${selectedModel}\n原因：没有可用的 key`,
                            position: 'top-left',
                        });
                        return;
                    }
                    // 部分/全部失败：列出失败 key 及原因（最多展示前 5 个）
                    const failed = (data.results ?? []).filter((r) => !r.success);
                    const lines = failed
                        .slice(0, 5)
                        .map((r) => `· ${r.key_name}：${extractTestError(r.message)}`);
                    if (failed.length > 5) lines.push(`…共 ${failed.length} 个失败`);
                    toast.error(`测试失败：${passed}/${total} 个 key 通过`, {
                        description: `渠道「${channel.name}」· 模型 ${selectedModel}\n${lines.join('\n')}`,
                        position: 'top-left',
                    });
                },
                onError: (error) => {
                    const reason = error.message ? extractTestError(error.message) : '请求异常';
                    toast.error('测试失败', {
                        description: `渠道「${channel.name}」· 模型 ${selectedModel}\n原因：${reason}`,
                        position: 'top-left',
                    });
                },
                onSettled: () => {
                    queryClient.invalidateQueries({ queryKey: ['channels', 'list'] });
                },
            }
        );
    };

    return (
        <MorphingDialog>
            <MorphingDialogTrigger className="w-full">
                <article className="flex flex-col gap-4 rounded-3xl border border-border bg-card text-card-foreground p-4 transition-all duration-300">
                    <header className="relative flex items-center gap-2">
                        <Tooltip side="top" sideOffset={10} align="center">
                            <TooltipTrigger asChild>
                                {/* 渠道名称：≤4 字完整显示（永不截断）；>4 字在 JS 层截为前 4 字+省略号，CSS truncate 仅作极窄兜底 */}
                                <h3
                                    className={`flex-1 text-lg font-bold ${channel.name.length > 4 ? 'truncate min-w-0' : 'shrink-0 whitespace-nowrap'}`}
                                >
                                    {channel.name.length > 4 ? `${channel.name.slice(0, 4)}…` : channel.name}
                                </h3>
                            </TooltipTrigger>
                            <TooltipContent key={channel.name}>{channel.name}</TooltipContent>
                        </Tooltip>
                        {/* 模型选择 + 可用性测试（点击不触发卡片弹层） */}
                        <div className="flex shrink-0 items-center gap-1.5" onClick={(e) => e.stopPropagation()}>
                            <Select value={selectedModel} onValueChange={handleModelChange}>
                                <SelectTrigger size="sm" className="w-[130px] border-0! bg-card! shadow-none! hover:bg-black/5! dark:hover:bg-white/10!">
                                    <SelectValue placeholder={t('selectModel')} />
                                </SelectTrigger>
                                <SelectContent>
                                    {modelOptions.length === 0 ? (
                                        <SelectItem value="__no_model__" disabled>
                                            {t('noModel')}
                                        </SelectItem>
                                    ) : (
                                        modelOptions.map((model) => (
                                            <SelectItem key={model} value={model}>
                                                {model}
                                            </SelectItem>
                                        ))
                                    )}
                                </SelectContent>
                            </Select>
                            <Popover open={testMenuOpen} onOpenChange={setTestMenuOpen}>
                                <PopoverTrigger asChild>
                                    <Button
                                        size="sm"
                                        variant="ghost"
                                        className="p-1.5 rounded-lg transition-colors hover:bg-muted text-muted-foreground hover:text-foreground"
                                        disabled={testModel.isPending}
                                        aria-label={t('test')}
                                    >
                                        {testModel.isPending ? (
                                            <Loader2 className="size-4 animate-spin" />
                                        ) : (
                                            <FlaskConical className="size-4" />
                                        )}
                                    </Button>
                                </PopoverTrigger>
                                <PopoverContent align="end" side="bottom" sideOffset={8} className="w-60 p-1">
                                    <div className="flex flex-col gap-0.5">
                                        <button
                                            type="button"
                                            className="flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm hover:bg-muted"
                                            onClick={() => {
                                                setTestMenuOpen(false);
                                                handleTestModel();
                                            }}
                                        >
                                            <FlaskConical className="size-4 shrink-0 text-primary" />
                                            <span>测试当前渠道</span>
                                            <span className="ml-auto text-xs text-muted-foreground">默认规则</span>
                                        </button>
                                        <button
                                            type="button"
                                            className="flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm hover:bg-muted"
                                            onClick={() => {
                                                setTestMenuOpen(false);
                                                handleTestAllKeys();
                                            }}
                                        >
                                            <Key className="size-4 shrink-0 text-primary" />
                                            <span>测试所有 key</span>
                                            <span className="ml-auto text-xs text-muted-foreground">逐个测试</span>
                                        </button>
                                    </div>
                                </PopoverContent>
                            </Popover>
                        </div>
                        <Switch
                            checked={channel.enabled}
                            onCheckedChange={handleEnableChange}
                            disabled={enableChannel.isPending}
                            onClick={(e) => e.stopPropagation()}
                            className="shrink-0"
                        />
                    </header>

                    {isListLayout ? (
                        <dl className="grid grid-cols-2 gap-2 lg:grid-cols-6">
                            <div className="rounded-2xl border border-border/70 bg-background/80 p-2">
                                <dt className="mb-1 flex items-center gap-1 text-xs text-muted-foreground">
                                    <MessageSquare className="size-3.5 text-primary" />
                                    {t('requestCount')}
                                </dt>
                                <dd className="text-sm font-semibold">
                                    {stats.request_count.formatted.value}
                                    <span className="ml-1 text-xs text-muted-foreground">{stats.request_count.formatted.unit}</span>
                                </dd>
                            </div>
                            <div className="rounded-2xl border border-border/70 bg-background/80 p-2">
                                <dt className="mb-1 flex items-center gap-1 text-xs text-muted-foreground">
                                    <Layers className="size-3.5 text-primary" />
                                    {tForm('model')}
                                </dt>
                                <dd className="text-sm font-semibold">{modelCount}</dd>
                            </div>
                            <div className="rounded-2xl border border-border/70 bg-background/80 p-2">
                                <dt className="mb-1 flex items-center gap-1 text-xs text-muted-foreground">
                                    <Key className="size-3.5 text-primary" />
                                    {tSections('keys')}
                                </dt>
                                <dd className="text-sm font-semibold">{enabledKeyCount}/{channel.keys.length}</dd>
                            </div>
                            <div className="rounded-2xl border border-border/70 bg-background/80 p-2">
                                <dt className="mb-1 flex items-center gap-1 text-xs text-muted-foreground">
                                    <CheckCircle2 className="size-3.5 text-emerald-500" />
                                    {tMetrics('successRequests')}
                                </dt>
                                <dd className="text-sm font-semibold">{stats.request_success.formatted.value}</dd>
                            </div>
                            <div className="rounded-2xl border border-border/70 bg-background/80 p-2">
                                <dt className="mb-1 flex items-center gap-1 text-xs text-muted-foreground">
                                    <XCircle className="size-3.5 text-destructive" />
                                    {tMetrics('failedRequests')}
                                </dt>
                                <dd className="text-sm font-semibold">{stats.request_failed.formatted.value}</dd>
                            </div>
                            <div className="rounded-2xl border border-border/70 bg-background/80 p-2">
                                <dt className="mb-1 flex items-center gap-1 text-xs text-muted-foreground">
                                    <DollarSign className="size-3.5 text-primary" />
                                    {t('totalCost')}
                                </dt>
                                <dd className="text-sm font-semibold">
                                    {quotaCostLabel}
                                </dd>
                            </div>
                        </dl>
                    ) : (
                        <dl className="grid grid-cols-1 gap-3">
                            <div className="flex items-center justify-between rounded-2xl border border-border/70 bg-background/80 p-2">
                                <div className="flex items-center gap-3">
                                    <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
                                        <MessageSquare className="h-5 w-5" />
                                    </span>
                                    <dt className="text-sm text-muted-foreground">{t('requestCount')}</dt>
                                </div>
                                <dd className="text-base">
                                    {stats.request_count.formatted.value}
                                    <span className="ml-1 text-xs text-muted-foreground">{stats.request_count.formatted.unit}</span>
                                </dd>
                            </div>

                            <div className="flex items-center justify-between rounded-2xl border border-border/70 bg-background/80 p-2">
                                <div className="flex items-center gap-3">
                                    <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
                                        <DollarSign className="h-5 w-5" />
                                    </span>
                                    <dt className="text-sm text-muted-foreground">{t('totalCost')}</dt>
                                </div>
                                <dd className="text-base">
                                    {quotaCostLabel}
                                </dd>
                            </div>
                        </dl>
                    )}

                </article>
            </MorphingDialogTrigger>

            <MorphingDialogContainer>
                <MorphingDialogContent className="w-full md:max-w-xl bg-card text-card-foreground px-4 py-2 rounded-3xl max-h-[90vh] overflow-y-auto">
                    <CardContent channel={channel} stats={stats} />
                </MorphingDialogContent>
            </MorphingDialogContainer>
        </MorphingDialog>
    );
}
