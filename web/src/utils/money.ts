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

  // Pembulatan half-up pada STRING desimal (audit3 V-FJ-08): versi lama
  // memotong pecahan begitu saja, sehingga biaya selalu tampil lebih kecil
  // dari nilai sebenarnya (1.999 pada 2 digit menjadi $1.99, bukan $2.00).
  // Bekerja pada string — bukan Number — agar presisi nilai uang tidak
  // hilang ke galat biner. Digit pertama yang terpotong >= '5' menaikkan
  // digit terakhir yang dipertahankan, merambat sampai ke bagian bulat.
  const wholeDigits = rawWhole.replace(/^0+(?=\d)/, '').split('').map((c) => c.charCodeAt(0) - 48);
  const keptFraction = rawFraction.slice(0, digits).padEnd(digits, '0');
  const fracDigits = keptFraction.split('').map((c) => c.charCodeAt(0) - 48);
  const dropped = rawFraction.length > digits ? rawFraction[digits] : undefined;
  if (dropped !== undefined && dropped >= '5') {
    let carry = 1;
    for (let i = fracDigits.length - 1; i >= 0 && carry === 1; i--) {
      const next = fracDigits[i] + 1;
      fracDigits[i] = next % 10;
      carry = next === 10 ? 1 : 0;
    }
    for (let i = wholeDigits.length - 1; i >= 0 && carry === 1; i--) {
      const next = wholeDigits[i] + 1;
      wholeDigits[i] = next % 10;
      carry = next === 10 ? 1 : 0;
    }
    if (carry === 1) wholeDigits.unshift(1);
  }

  const groupedWhole = wholeDigits.join('').replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  if (digits === 0) return `$${sign}${groupedWhole}`;

  const fraction = fracDigits.join('').replace(/0+$/, '');
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
