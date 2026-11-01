import type { APIKey } from '../../types';

export interface ScopeOption {
  id: string; // unique key: `${providerId || 'none'}::${modelId}`
  modelId: string;
  modelSlug: string;
  modelDisplayName: string;
  providerId: string;
  providerName: string;
  providerDisplayName: string;
  family?: string;
  contextWindow?: number;
  capabilities?: string[];
}

export type ExpirationPreset = 'never' | '30d' | '60d' | '90d' | '1y' | 'custom';

export interface APIKeyCardProps {
  apiKey: APIKey;
  onOpenAllowed: (key: APIKey) => void;
  onRotate: (id: string) => void;
  onRevoke: (id: string) => void;
  // Anggaran nyata per kunci dari entitas Budget (scope 'api_key'); absen = kunci
  // tidak punya anggaran terdaftar, batang anggaran tidak ditampilkan.
  budget?: {
    spentUsd: number;
    limitUsd: number;
    periodLabel: string;
    thresholdPct: number;
  };
}
