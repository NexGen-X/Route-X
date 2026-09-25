import type { RoutingRule, Model } from '../../types';
import {
  LABEL_STRATEGI,
  modeAturan,
  parsePipeline,
  parsePipelineDariTag,
} from '../../lib/rulePipeline';

export interface RuleModeInfo {
  mode: 'model_only' | 'combo_routing' | 'routing';
  label: string;
  badgeVariant: 'info' | 'lime' | 'warn';
  detail: string;
  alias?: string | null;
  pipelineCount?: number;
}

export const getRuleMode = (r: RoutingRule, models: Model[]): RuleModeInfo => {
  // Sumber kebenaran mode adalah kolom pipeline (migrasi 0014), BUKAN max_attempts.
  // Memakai max_attempts adalah bug K2: aturan combo dengan anggaran 1 salah dibaca
  // model_only, lalu simpan berikutnya menghapus provider_ids/weights aturan tersebut.
  // Tag [combo:...] di description adalah cadangan untuk aturan produksi lama
  // (mis. raute-x) yang belum dikonversi ke kolom jsonb.
  if (modeAturan(r) === 'combo') {
    const modelUtama = models.find((m) => m.id === r.match_model_id);
    const pipeline = parsePipeline(r) ?? parsePipelineDariTag(r.description, modelUtama?.model_id);
    const aliasTag = (r.description || '').match(/\[combo:alias=([^\]]+)\]/);
    const alias = r.virtual_alias ?? (aliasTag ? aliasTag[1] : null);
    const jumlah = pipeline?.models.length ?? 1;
    return {
      mode: 'combo_routing',
      label: 'COMBO PIPELINE',
      badgeVariant: 'lime',
      detail: alias
        ? `Virtual Endpoint: ${alias} · ${jumlah} model`
        : `${jumlah} model · ${pipeline ? LABEL_STRATEGI[pipeline.strategy].label : 'Combo'}`,
      alias,
      pipelineCount: jumlah,
    };
  }
  if (r.max_attempts === 1 || (r.description || '').includes('[model_only]')) {
    return { mode: 'model_only', label: 'MODEL ONLY', badgeVariant: 'info', detail: 'Direct 1:1 Passthrough' };
  }
  return { mode: 'routing', label: 'ROUTING', badgeVariant: 'warn', detail: `${r.max_attempts} percobaan failover` };
};
