export function validateProxyUrl(urlStr: string): string | null {
  if (!urlStr) return null;
  
  // Basic sanity check to prevent obvious local host names
  const localPatterns = [
    /localhost/i,
    /127\.0\.0\.1/,
    /0x7f\.0\.0\.1/i,
    /^10\./,
    /^192\.168\./,
    /^172\.(1[6-9]|2[0-9]|3[0-1])\./,
    /^0\.0\.0\.0/
  ];

  try {
    const u = new URL(urlStr);
    const host = u.hostname;
    for (const p of localPatterns) {
      if (p.test(host)) {
        return `Hostname atau IP tidak diizinkan (potensi risiko SSRF): ${host}`;
      }
    }
  } catch (err) {
    // URL parsing failed, but let server handle format validation if needed, or we can reject.
    return "Format URL proxy tidak valid.";
  }
  return null;
}
