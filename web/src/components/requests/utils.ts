import type { RequestLog, RequestPayload } from '../../types';

export type RequestTabFilter = 'all' | 'success' | 'errors';
export type StatusBadgeVariant = 'success' | 'warn' | 'error' | 'neutral';

/**
 * `request_body`/`response_body` dikirim backend sebagai JSON mentah (json.RawMessage).
 * Render aman: prettify bila berupa object, tampilkan apa adanya bila string,
 * dan placeholder bila kosong.
 */
export const formatRawPayload = (raw: unknown): string => {
  if (raw === null || raw === undefined) return '(kosong atau tidak direkam)';
  if (typeof raw === 'string') return raw.trim() || '(kosong atau tidak direkam)';
  try {
    return JSON.stringify(raw, null, 2);
  } catch {
    return String(raw);
  }
};

/**
 * Ambil teks prompt pertama dari body request OpenAI-style untuk perintah cURL.
 * Struktur OpenAI: { "messages": [{ "role": "user", "content": "..." }] }.
 */
export const extractPromptText = (body: unknown): string => {
  if (!body || typeof body !== 'object') return '';
  const messages = (body as { messages?: unknown }).messages;
  if (!Array.isArray(messages)) return '';
  for (const msg of messages) {
    if (!msg || typeof msg !== 'object') continue;
    const content = (msg as { content?: unknown }).content;
    if (typeof content === 'string' && content.trim()) return content;
  }
  return '';
};

export const getStatusBadgeVariant = (code: number): StatusBadgeVariant => {
  if (code >= 200 && code < 300) return 'success';
  if (code >= 400 && code < 500) return 'warn';
  if (code >= 500) return 'error';
  return 'neutral';
};

export const getStatusBadgeLabel = (code: number): string => {
  if (code >= 200 && code < 300) return `${code} OK`;
  if (code >= 400 && code < 500) return `${code} WARN`;
  if (code >= 500) return `${code} ERR`;
  return `${code}`;
};

export const generateCurlCommand = (
  req: RequestLog,
  payload?: RequestPayload | null,
  baseUrl?: string
): string => {
  const host =
    baseUrl ||
    (typeof window !== 'undefined'
      ? `${window.location.protocol}//${window.location.host}`
      : 'http://localhost:8080');
  const gwURL = `${host}/v1/chat/completions`;
  const promptText = extractPromptText(payload?.request_body) || 'Hello Route-X';
  const model = req.model_id || req.requested_model || 'gpt-4o';
  return `curl -X POST "${gwURL}" \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer rx_live_personal_gateway" \\\n  -d '{\n    "model": ${JSON.stringify(model)},\n    "messages": [{"role": "user", "content": ${JSON.stringify(promptText)}}]\n  }'`;
};
