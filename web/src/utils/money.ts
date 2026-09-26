const DECIMAL_RE = /^(-?)(\d+)(?:\.(\d+))?$/;
const MAX_FRACTION_DIGITS = 20;

export function formatUSD(value?: string | number | null, fractionDigits = 4): string {
  const digits = Number.isInteger(fractionDigits)
    ? Math.min(MAX_FRACTION_DIGITS, Math.max(0, fractionDigits))
    : 4;
  const normalized = typeof value === 'number'
    ? (Number.isFinite(value) ? String(value) : '')
    : String(value ?? '0').trim();
  const match = normalized.match(DECIMAL_RE);
  if (!match) return digits === 0 ? '$0' : '$0.' + '0'.repeat(digits);

  const [, sign, rawWhole, rawFraction = ''] = match;
  const whole = rawWhole.replace(/^0+(?=\d)/, '');
  const groupedWhole = whole.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  if (digits === 0) return `$${sign}${groupedWhole}`;

  let fraction = (rawFraction + '0'.repeat(digits)).slice(0, digits);
  fraction = fraction.replace(/0+$/, '');
  return fraction.length > 0 ? `$${sign}${groupedWhole}.${fraction}` : `$${sign}${groupedWhole}`;
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
