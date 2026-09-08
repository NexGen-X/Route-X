const DECIMAL_RE = /^(-?)(\d+)(?:\.(\d+))?$/;

export function formatUSD(value?: string, fractionDigits = 4): string {
  const match = (value || '0').trim().match(DECIMAL_RE);
  if (!match) return '$0.' + '0'.repeat(fractionDigits);

  const [, sign, rawWhole, rawFraction = ''] = match;
  const whole = rawWhole.replace(/^0+(?=\d)/, '');
  const fraction = (rawFraction + '0'.repeat(fractionDigits)).slice(0, fractionDigits);
  return `$${sign}${whole}.${fraction}`;
}

export function percentageOfDecimal(spent: string, limit: string): number {
  const toScaled = (value: string): bigint | null => {
    const match = value.trim().match(DECIMAL_RE);
    if (!match || match[1] === '-') return null;
    const fraction = (match[3] || '').padEnd(8, '0').slice(0, 8);
    return BigInt(match[2]) * 100000000n + BigInt(fraction);
  };

  const numerator = toScaled(spent || '0');
  const denominator = toScaled(limit || '0');
  if (numerator === null || denominator === null || denominator === 0n) return 0;
  const rounded = (numerator * 100n + denominator / 2n) / denominator;
  return Number(rounded > 100n ? 100n : rounded);
}
