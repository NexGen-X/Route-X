/**
 * Result of token extraction from an input string (e.g. redirect URL, callback hash, or raw key).
 */
export interface AuthExtractionResult {
  token: string;
  source: 'query_param' | 'hash_param' | 'direct' | 'empty';
  paramName?: string;
  cleanHint?: string;
}

/**
 * Intelligent extraction of API key, authorization code, or OAuth access token
 * from user input which might be a full callback redirect URL, hash fragment, or direct string.
 */
export function extractAuthTokenFromInput(input: string): AuthExtractionResult {
  const trimmed = (input || '').trim();
  if (!trimmed) {
    return { token: '', source: 'empty' };
  }

  // Common parameter names for auth tokens/codes across major AI providers
  const candidateKeys = [
    'code',
    'token',
    'access_token',
    'api_key',
    'apiKey',
    'key',
    'session_token',
    'auth_token',
    'id_token',
  ];

  // Check prefix forms like "code=xxx" or "token=xxx"
  for (const key of candidateKeys) {
    if (trimmed.toLowerCase().startsWith(`${key.toLowerCase()}=`)) {
      const val = trimmed.slice(key.length + 1).trim();
      if (val) {
        return {
          token: val,
          source: 'query_param',
          paramName: key,
          cleanHint: `${key}=${val.length > 16 ? val.slice(0, 8) + '...' : val}`,
        };
      }
    }
  }

  // Attempt URL parsing
  if (
    trimmed.startsWith('http://') ||
    trimmed.startsWith('https://') ||
    trimmed.startsWith('localhost:') ||
    trimmed.startsWith('127.0.0.1:') ||
    trimmed.includes('callback?') ||
    trimmed.includes('oauth') ||
    trimmed.includes('?code=') ||
    trimmed.includes('?token=') ||
    trimmed.includes('#access_token=') ||
    trimmed.includes('#token=') ||
    trimmed.includes('#code=')
  ) {
    try {
      let urlStr = trimmed;
      if (!urlStr.startsWith('http://') && !urlStr.startsWith('https://')) {
        urlStr = 'http://' + urlStr;
      }
      const parsed = new URL(urlStr);

      // 1. Search params
      for (const key of candidateKeys) {
        const val = parsed.searchParams.get(key);
        if (val && val.trim()) {
          return {
            token: val.trim(),
            source: 'query_param',
            paramName: key,
            cleanHint: `URL param "${key}"`,
          };
        }
      }

      // 2. Hash params (e.g. #access_token=... or #code=...)
      if (parsed.hash && parsed.hash.length > 1) {
        const hashStr = parsed.hash.replace(/^#/, '');
        const hashParams = new URLSearchParams(hashStr);
        for (const key of candidateKeys) {
          const val = hashParams.get(key);
          if (val && val.trim()) {
            return {
              token: val.trim(),
              source: 'hash_param',
              paramName: key,
              cleanHint: `Hash fragment "${key}"`,
            };
          }
        }
      }
    } catch {
      // If URL parsing fails, fallback to direct string below
    }
  }

  // Direct string / token
  return {
    token: trimmed,
    source: 'direct',
    cleanHint: 'Token langsung',
  };
}
