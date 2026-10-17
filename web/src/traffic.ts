const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"];

// 累计量保持整数精度，只有单位换算后的有限小数参与显示。
export function formatBytes(value: string): string {
  const bytes = BigInt(value);
  let divisor = 1n;
  let unit = 0;
  while (bytes >= divisor * 1024n && unit < units.length - 1) {
    divisor *= 1024n;
    unit++;
  }
  if (unit === 0) return `${bytes} B`;
  const tenths = (bytes * 10n + divisor / 2n) / divisor;
  return `${tenths / 10n}.${tenths % 10n} ${units[unit]}`;
}

export function formatSpeed(value: number): string {
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}/s`;
}

// 十进制 GiB 直接换算成整数字节，拒绝无法用整数字节表示的输入。
export function gibToBytes(value: string): string | undefined {
  if (!/^(0|[1-9][0-9]*)(\.[0-9]{1,30})?$/.test(value)) return undefined;
  const [whole = "0", fraction = ""] = value.split(".");
  const scale = 10n ** BigInt(fraction.length);
  const numerator = (BigInt(whole) * scale + BigInt(fraction || "0")) * 1073741824n;
  if (numerator % scale !== 0n) return undefined;
  const bytes = numerator / scale;
  return bytes > 0n && bytes <= 9223372036854775807n ? String(bytes) : undefined;
}
