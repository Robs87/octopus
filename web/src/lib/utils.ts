import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}


function formatNumber(num: number | undefined, compare: number[], units: string[]): { value: string, unit: string } {
  if (num === undefined) return { value: "0.00", unit: units[0] };
  else if (num >= compare[0]) return { value: (num / compare[0]).toFixed(2), unit: units[1] };
  else if (num >= compare[1]) return { value: (num / compare[1]).toFixed(2), unit: units[2] };
  else if (num >= compare[2]) return { value: (num / compare[2]).toFixed(2), unit: units[3] };
  else if (num >= compare[3]) return { value: (num / compare[3]).toFixed(2), unit: units[4] };
  else return { value: (num).toFixed(2), unit: units[5] };
}

export function formatCount(num: number | undefined): { raw: number, formatted: { value: string, unit: string } } {
  return {
    raw: num ?? 0,
    formatted: formatNumber(num, [1000000000, 1000000, 1000, 1], ['', 'B', 'M', 'K', '', '']),
  };
}
export function formatMoney(num: number | undefined): { raw: number, formatted: { value: string, unit: string } } {
  if (num === undefined) return { raw: 0, formatted: { value: '0.00', unit: '$' } };
  const n = num ?? 0;
  // 大额金额保持 K/M/B 单位（两位小数）
  if (n >= 1000000000) return { raw: n, formatted: { value: (n / 1000000000).toFixed(2), unit: 'B$' } };
  if (n >= 1000000) return { raw: n, formatted: { value: (n / 1000000).toFixed(2), unit: 'M$' } };
  if (n >= 1000) return { raw: n, formatted: { value: (n / 1000).toFixed(2), unit: 'K$' } };
  if (n === 0) return { raw: 0, formatted: { value: '0.00', unit: '$' } };
  // 小于 1 的金额最多展示小数点后 6 位：去掉尾部多余的 0；
  // 若 6 位内无法表示（极小值）则保留 0.000000 占位，避免误读为 0
  if (n < 1) {
    let s = n.toFixed(6);
    if (parseFloat(s) === 0) return { raw: n, formatted: { value: '0.000000', unit: '$' } };
    s = s.replace(/0+$/, '').replace(/\.$/, '');
    return { raw: n, formatted: { value: s, unit: '$' } };
  }
  // 常规金额（1 <= n < 1000）四位有效数字，避免 toFixed(2) 丢失精度
  return { raw: n, formatted: { value: n.toPrecision(4), unit: '$' } };
}

export function formatTime(ms: number | undefined): { raw: number, formatted: { value: string, unit: string } } {
  return {
    raw: ms ?? 0,
    formatted: formatNumber(ms, [86400000, 3600000, 60000, 1000], ['', 'd', 'h', 'm', 's', 'ms']),
  };
}

/**
 * 对 API Key / 密钥名称做前端脱敏展示：
 * 只保留前 4 位和后 4 位，中间用 5 个 * 号代替。
 * 长度 <= 4 时直接返回原值（太短无脱敏意义）。
 */
export function maskKey(key: string | undefined | null): string {
  if (!key) return '';
  const s = key.trim();
  if (s.length <= 4) return s;
  const front = s.slice(0, 4);
  const back = s.slice(-4);
  return `${front}*****${back}`;
}