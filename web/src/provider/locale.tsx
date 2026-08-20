import { useEffect, useState, type ReactNode } from 'react';
import { IntlProvider } from 'use-intl';
import { useSettingStore, type Locale } from '@/stores/setting';

import zh_hansMessages from '@/locales/zh_hans.json';
import zh_hantMessages from '@/locales/zh_hant.json';
import enMessages from '@/locales/en.json';

const messages: Record<Locale, typeof zh_hansMessages> = { // 各语言对应的客户端消息集合。
    zh_hans: zh_hansMessages,
    zh_hant: zh_hantMessages,
    en: enMessages,
};

// 内部 locale 标识（zh_hans/zh_hant）是下划线格式，不是合法的 BCP 47 语言标签。
// 若原样传给 IntlProvider，use-intl 用 intl-messageformat 编译带参数的消息
// （如 "共 {total} 条"、"第 {page} / {pages} 页"）时会抛
// RangeError: Invalid language tag: zh_hans，导致这些消息回退显示 key 原文。
// 这里统一转为标准标签：zh_hans → zh-Hans，zh_hant → zh-Hant，en 不变。
const toBCP47 = (locale: Locale) => locale.replace('_', '-');

export function LocaleProvider({ children }: { children: ReactNode }) {
    const { locale } = useSettingStore();
    const [currentLocale, setCurrentLocale] = useState<Locale>('zh_hans');

    useEffect(() => {
        setCurrentLocale(locale);
    }, [locale]);

    return (
        <IntlProvider
            locale={toBCP47(currentLocale)}
            messages={messages[currentLocale] ?? zh_hansMessages}
            timeZone="Asia/Shanghai"
        >
            {children}
        </IntlProvider>
    );
}
