import type { CLITool, Model, RoutingRule } from '../../types';
import { parsePipeline } from '../../lib/rulePipeline';

export interface ToolConfig {
  mode: 'model_only' | 'routing' | 'combo';
  target: string;
  apiKey: string;
}

export interface DiagStatus {
  testing: boolean;
  latency?: number;
  ok?: boolean;
  message?: string;
}

export interface ComboOption {
  value: string;
  label: string;
  description: string;
}

export const extractDynamicComboOptions = (rules: RoutingRule[]): ComboOption[] => {
  const comboRules = rules.filter((r) => {
    if (parsePipeline(r) !== null) return true;
    if (r.virtual_alias && r.virtual_alias.trim() !== '') return true;
    return (r.description || '').includes('[combo:');
  });
  if (comboRules.length === 0) {
    return [];
  }
  return comboRules.map((r) => {
    const pipe = parsePipeline(r);
    const tagMatch = (r.description || '').match(/\[combo:alias=([^\]]+)\]/);
    const alias =
      (r.virtual_alias && r.virtual_alias.trim()) ||
      (tagMatch ? tagMatch[1].trim() : r.name);

    let description = 'Smart Tiered Cascade Rule';
    if (pipe && pipe.models && pipe.models.length > 0) {
      description = `Combo (${pipe.strategy}): ${pipe.models.join(' -> ')}`;
    } else {
      const cleanDesc = (r.description || '')
        .replace(/\[combo:[^\]]+\]/g, '')
        .replace(/\[routing\]/g, '')
        .trim();
      if (cleanDesc) {
        description = cleanDesc;
      }
    }

    return {
      value: alias,
      label: `${r.name} (${alias})`,
      description,
    };
  });
};

export const getToolCompleteSnippet = (
  tool: CLITool,
  config: ToolConfig | undefined,
  models: Model[],
  userApiKey: string,
  publicBaseURL: string,
): string => {
  const target = config?.target || tool.active_target || models[0]?.model_id || '';
  const isClaude = tool.id === 'claude';
  const gwURL = isClaude ? publicBaseURL : `${publicBaseURL}/v1`;
  const key = config?.apiKey || userApiKey || '<KUNCI_API_ROUTEX_ANDA>';

  const lines: string[] = [];

  // 1. Base URL
  for (const [k] of Object.entries(tool.env_vars || {})) {
    if (
      k.includes('BASE') ||
      k.includes('HOST') ||
      k.includes('ENDPOINT') ||
      k.includes('PATH')
    ) {
      lines.push(`export ${k}='${gwURL}'`);
    }
  }

  // 2. Target Model / Rule
  for (const [k] of Object.entries(tool.env_vars || {})) {
    if (k.includes('MODEL') || k.includes('DEFAULT')) {
      lines.push(`export ${k}='${target}'`);
    }
  }

  // 3. API Key
  const keyVar = tool.env_var_api_key || 'OPENAI_API_KEY';
  lines.push(`export ${keyVar}='${key}'`);

  return lines.join('\n');
};
