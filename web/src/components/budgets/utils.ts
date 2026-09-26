import type { APIKey, Model } from '../../types';
import type { ScopeDisplayInfo } from './types';

/**
 * Memformat dan menentukan representasi visual dari cakupan (scope) budget atau rate limit.
 */
export function getScopeDisplay(
  scope: string,
  scopeId?: string,
  apiKeys: APIKey[] = [],
  models: Model[] = []
): ScopeDisplayInfo {
  if (!scopeId || scope === 'global' || scopeId === 'global') {
    return { label: 'Global Gateway', isRaw: false };
  }
  if (scope === 'api_key') {
    const key = apiKeys.find((k) => k.id === scopeId);
    return { label: key ? key.name : `${scopeId.slice(0, 8)}...`, isRaw: !key };
  }
  if (scope === 'model') {
    const m = models.find((mod) => mod.model_id === scopeId);
    return { label: m ? (m.display_name || m.model_id) : scopeId, isRaw: false };
  }
  if (scope === 'ip') {
    return { label: `IP: ${scopeId}`, isRaw: false };
  }
  return { label: scopeId, isRaw: true };
}

/**
 * Type guard dan helper aman untuk mengekstrak pesan kesalahan dari unknown error.
 */
export function getErrorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  if (typeof err === 'object' && err !== null && 'message' in err) {
    const msg = (err as Record<string, unknown>).message;
    if (typeof msg === 'string') {
      return msg;
    }
  }
  if (typeof err === 'string') {
    return err;
  }
  return String(err);
}
