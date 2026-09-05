import React from 'react';

export interface KnownProviderPreset {
  id: string;
  name: string;
  displayName: string;
  kind: 'openai' | 'anthropic' | 'google' | 'openai-compatible' | 'custom';
  baseUrl: string;
  tag: string;
  description: string;
  color: string;
  bgColor: string;
  borderColor: string;
  apiKeyPlaceholder: string;
  apiKeyHelp: string;
  defaultPriority: number;
  defaultWeight: number;
  highlightModels: string[];
}

export const KNOWN_PROVIDERS: KnownProviderPreset[] = [
  {
    id: 'openai',
    name: 'openai-main',
    displayName: 'OpenAI',
    kind: 'openai',
    baseUrl: 'https://api.openai.com/v1',
    tag: 'GPT-4o & o3-mini',
    description: 'Penyedia model unggulan OpenAI: GPT-4o, GPT-4o-mini, o1, dan reasoning o3-mini.',
    color: '#10A37F',
    bgColor: 'rgba(16, 163, 127, 0.12)',
    borderColor: 'rgba(16, 163, 127, 0.35)',
    apiKeyPlaceholder: 'sk-proj-... atau sk-...',
    apiKeyHelp: 'Dapatkan API key dari platform.openai.com/api-keys',
    defaultPriority: 100,
    defaultWeight: 100,
    highlightModels: ['gpt-4o', 'gpt-4o-mini', 'o3-mini', 'o1'],
  },
  {
    id: 'anthropic',
    name: 'anthropic-main',
    displayName: 'Anthropic',
    kind: 'anthropic',
    baseUrl: 'https://api.anthropic.com',
    tag: 'Claude 3.7 Sonnet & Opus',
    description: 'Keluarga Claude 3.7 Sonnet (Hybrid Reasoning), 3.5 Sonnet, dan Haiku untuk coding.',
    color: '#D97706',
    bgColor: 'rgba(217, 119, 6, 0.12)',
    borderColor: 'rgba(217, 119, 6, 0.35)',
    apiKeyPlaceholder: 'sk-ant-api03-...',
    apiKeyHelp: 'Dapatkan token dari console.anthropic.com/settings/keys',
    defaultPriority: 100,
    defaultWeight: 100,
    highlightModels: ['claude-3-7-sonnet', 'claude-3-5-sonnet', 'claude-3-5-haiku'],
  },
  {
    id: 'google',
    name: 'google-gemini',
    displayName: 'Google Gemini',
    kind: 'google',
    baseUrl: 'https://generativelanguage.googleapis.com',
    tag: 'Gemini 2.5 Pro & 2.0 Flash',
    description: 'Model multimodal Google dengan jendela konteks raksasa 2M token dan kecepatan kilat.',
    color: '#3B82F6',
    bgColor: 'rgba(59, 130, 246, 0.12)',
    borderColor: 'rgba(59, 130, 246, 0.35)',
    apiKeyPlaceholder: 'AIzaSy...',
    apiKeyHelp: 'Dapatkan kunci dari aistudio.google.com/app/apikey',
    defaultPriority: 100,
    defaultWeight: 100,
    highlightModels: ['gemini-2.5-pro', 'gemini-2.0-flash', 'gemini-1.5-pro'],
  },
  {
    id: 'deepseek',
    name: 'deepseek-main',
    displayName: 'DeepSeek',
    kind: 'openai-compatible',
    baseUrl: 'https://api.deepseek.com',
    tag: 'V3 & R1 Reasoner',
    description: 'Inference super hemat dan reasoning canggih DeepSeek-R1 dan DeepSeek-V3.',
    color: '#06B6D4',
    bgColor: 'rgba(6, 182, 212, 0.12)',
    borderColor: 'rgba(6, 182, 212, 0.35)',
    apiKeyPlaceholder: 'sk-...',
    apiKeyHelp: 'Dapatkan kunci dari platform.deepseek.com/api_keys',
    defaultPriority: 90,
    defaultWeight: 100,
    highlightModels: ['deepseek-chat', 'deepseek-reasoner'],
  },
  {
    id: 'groq',
    name: 'groq-fast',
    displayName: 'Groq LPU',
    kind: 'openai-compatible',
    baseUrl: 'https://api.groq.com/openai/v1',
    tag: 'Ultra Fast (~300 tps)',
    description: 'Pemrosesan chip LPU dengan latensi ultra rendah untuk Llama-3.3 dan Mixtral.',
    color: '#F97316',
    bgColor: 'rgba(249, 115, 22, 0.12)',
    borderColor: 'rgba(249, 115, 22, 0.35)',
    apiKeyPlaceholder: 'gsk_...',
    apiKeyHelp: 'Dapatkan kunci dari console.groq.com/keys',
    defaultPriority: 95,
    defaultWeight: 100,
    highlightModels: ['llama-3.3-70b-versatile', 'llama-3.1-8b-instant', 'deepseek-r1-distill-llama-70b'],
  },
  {
    id: 'ollama',
    name: 'ollama-local',
    displayName: 'Ollama Local',
    kind: 'openai-compatible',
    baseUrl: 'http://localhost:11434',
    tag: 'Local / Self-Hosted',
    description: 'Eksekusi model mandiri secara lokal di workstation tanpa biaya API eksternal.',
    color: '#A855F7',
    bgColor: 'rgba(168, 85, 247, 0.12)',
    borderColor: 'rgba(168, 85, 247, 0.35)',
    apiKeyPlaceholder: 'ollama-local (bisa dikosongkan)',
    apiKeyHelp: 'Ollama lokal tidak mewajibkan API key secara default.',
    defaultPriority: 80,
    defaultWeight: 100,
    highlightModels: ['llama3.2:latest', 'qwen2.5-coder:latest', 'deepseek-r1:8b'],
  },
  {
    id: 'openrouter',
    name: 'openrouter-hub',
    displayName: 'OpenRouter',
    kind: 'openai-compatible',
    baseUrl: 'https://openrouter.ai/api/v1',
    tag: 'Aggregator 100+ Models',
    description: 'Pintu gerbang tunggal ke ratusan model komersial dan open-source terpopuler.',
    color: '#6366F1',
    bgColor: 'rgba(99, 102, 241, 0.12)',
    borderColor: 'rgba(99, 102, 241, 0.35)',
    apiKeyPlaceholder: 'sk-or-v1-...',
    apiKeyHelp: 'Dapatkan token dari openrouter.ai/keys',
    defaultPriority: 85,
    defaultWeight: 100,
    highlightModels: ['anthropic/claude-3.7-sonnet', 'openai/gpt-4o', 'deepseek/deepseek-r1'],
  },
  {
    id: 'mistral',
    name: 'mistral-main',
    displayName: 'Mistral AI',
    kind: 'openai-compatible',
    baseUrl: 'https://api.mistral.ai/v1',
    tag: 'Mistral Large & Codestral',
    description: 'Model AI Eropa berkinerja tinggi: Mistral Large 2, Codestral, dan Pixtral.',
    color: '#EC4899',
    bgColor: 'rgba(236, 72, 153, 0.12)',
    borderColor: 'rgba(236, 72, 153, 0.35)',
    apiKeyPlaceholder: 'secret-...',
    apiKeyHelp: 'Dapatkan kunci dari console.mistral.ai/api-keys',
    defaultPriority: 85,
    defaultWeight: 100,
    highlightModels: ['mistral-large-latest', 'codestral-latest', 'mistral-small-latest'],
  },
  {
    id: 'together',
    name: 'together-ai',
    displayName: 'Together AI',
    kind: 'openai-compatible',
    baseUrl: 'https://api.together.xyz/v1',
    tag: 'Open Source Cloud',
    description: 'Cloud inference untuk model open-source (Llama, Qwen, DeepSeek) kecepatan tinggi.',
    color: '#14B8A6',
    bgColor: 'rgba(20, 184, 166, 0.12)',
    borderColor: 'rgba(20, 184, 166, 0.35)',
    apiKeyPlaceholder: 'together-...',
    apiKeyHelp: 'Dapatkan kunci dari api.together.ai/settings/api-keys',
    defaultPriority: 80,
    defaultWeight: 100,
    highlightModels: ['meta-llama/Llama-3.3-70B-Instruct-Turbo', 'Qwen/Qwen2.5-72B-Instruct-Turbo'],
  },
  {
    id: 'perplexity',
    name: 'perplexity-ai',
    displayName: 'Perplexity AI',
    kind: 'openai-compatible',
    baseUrl: 'https://api.perplexity.ai',
    tag: 'Sonar Online Search',
    description: 'Model inferensi yang diperkaya pencarian web real-time untuk riset aktual.',
    color: '#0284C7',
    bgColor: 'rgba(2, 132, 199, 0.12)',
    borderColor: 'rgba(2, 132, 199, 0.35)',
    apiKeyPlaceholder: 'pplx-...',
    apiKeyHelp: 'Dapatkan token dari perplexity.ai/settings/api',
    defaultPriority: 80,
    defaultWeight: 100,
    highlightModels: ['sonar-pro', 'sonar', 'sonar-reasoning'],
  },
  {
    id: 'xai',
    name: 'xai-grok',
    displayName: 'xAI Grok',
    kind: 'openai-compatible',
    baseUrl: 'https://api.x.ai/v1',
    tag: 'Grok-2 & Grok Beta',
    description: 'Model penalaran dari xAI dengan pemahaman mendalam dan akses data cepat.',
    color: '#E2E8F0',
    bgColor: 'rgba(226, 232, 240, 0.12)',
    borderColor: 'rgba(226, 232, 240, 0.35)',
    apiKeyPlaceholder: 'xai-...',
    apiKeyHelp: 'Dapatkan kunci dari console.x.ai',
    defaultPriority: 80,
    defaultWeight: 100,
    highlightModels: ['grok-2', 'grok-2-mini'],
  },
  {
    id: 'cohere',
    name: 'cohere-ai',
    displayName: 'Cohere',
    kind: 'openai-compatible',
    baseUrl: 'https://api.cohere.ai/v1',
    tag: 'Command R+ & Embed',
    description: 'Model enterprise untuk RAG, summarization, dan embedding multi-bahasa.',
    color: '#10B981',
    bgColor: 'rgba(16, 185, 129, 0.12)',
    borderColor: 'rgba(16, 185, 129, 0.35)',
    apiKeyPlaceholder: 'co-...',
    apiKeyHelp: 'Dapatkan kunci dari dashboard.cohere.com/api-keys',
    defaultPriority: 75,
    defaultWeight: 100,
    highlightModels: ['command-r-plus', 'command-r'],
  },
  {
    id: 'azure',
    name: 'azure-openai',
    displayName: 'Azure OpenAI',
    kind: 'openai',
    baseUrl: 'https://RESOURCE.openai.azure.com/openai/deployments/DEPLOYMENT',
    tag: 'Enterprise Cloud',
    description: 'Layanan terkelola OpenAI di infrastruktur Microsoft Azure dengan kepatuhan enterprise.',
    color: '#0078D4',
    bgColor: 'rgba(0, 120, 212, 0.12)',
    borderColor: 'rgba(0, 120, 212, 0.35)',
    apiKeyPlaceholder: 'azure-api-key...',
    apiKeyHelp: 'Dapatkan kunci dari Azure Portal resource Keys & Endpoint.',
    defaultPriority: 90,
    defaultWeight: 100,
    highlightModels: ['gpt-4o', 'gpt-4o-mini'],
  },
];

// SVG Vector Icons for each known AI Provider
export const OpenAIIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" className={className}>
    <path d="M12 2a10 10 0 0 1 10 10c0 5.523-4.477 10-10 10S2 17.523 2 12 6.477 2 12 2z" />
    <path d="M12 6v6l4.5 2.5" />
    <path d="M8 10l4 2-4 2" />
  </svg>
);

export const AnthropicIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className}>
    <path d="M13.8 3h3.4L24 21h-3.6l-1.9-5.1H9.5L7.6 21H4L11.2 3h2.6zm2.4 10.2L12.6 6.8 9.8 13.2h6.4z" />
  </svg>
);

export const GoogleGeminiIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className}>
    <path d="M12 0C12 6.627 6.627 12 0 12c6.627 0 12 5.373 12 12 0-6.627 5.373-12 12-12-6.627 0-12-5.373-12-12z" />
  </svg>
);

export const DeepSeekIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className}>
    <path d="M2.5 14.5c1.5-4 5.5-7 10-7 4 0 7.5 2 9.5 5-2 1-4.5 1.5-7 1.5-4.5 0-8.5-1.5-12.5.5zm19.5 2c-3 3-7.5 4.5-12 4.5-3 0-5.5-1-7.5-2.5 4-1 8.5-.5 12.5-1.5 2.5-.5 5-1 7-.5z" />
    <circle cx="15.5" cy="10.5" r="1.5" />
  </svg>
);

export const GroqIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={className}>
    <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
  </svg>
);

export const OllamaIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className}>
    <path d="M9 3c-1.1 0-2 .9-2 2v2H5a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-2V5a2 2 0 0 0-2-2H9zm0 2h6v2H9V5zm-2 6a1.5 1.5 0 1 1 0 3 1.5 1.5 0 0 1 0-3zm10 0a1.5 1.5 0 1 1 0 3 1.5 1.5 0 0 1 0-3zm-5 4a3 3 0 0 1 3 2H9a3 3 0 0 1 3-2z" />
  </svg>
);

export const MistralIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className}>
    <rect x="2" y="3" width="4" height="4" />
    <rect x="18" y="3" width="4" height="4" />
    <rect x="2" y="8" width="8" height="4" />
    <rect x="14" y="8" width="8" height="4" />
    <rect x="2" y="13" width="20" height="4" />
    <rect x="6" y="18" width="4" height="4" />
    <rect x="14" y="18" width="4" height="4" />
  </svg>
);

export const OpenRouterIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={className}>
    <circle cx="6" cy="6" r="3" />
    <circle cx="18" cy="18" r="3" />
    <path d="M9 6h6a3 3 0 0 1 3 3v6" />
    <path d="M6 9v6a3 3 0 0 0 3 3h6" />
  </svg>
);

export const PerplexityIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={className}>
    <line x1="12" y1="2" x2="12" y2="22" />
    <line x1="2" y1="12" x2="22" y2="12" />
    <line x1="4.93" y1="4.93" x2="19.07" y2="19.07" />
    <line x1="19.07" y1="4.93" x2="4.93" y2="19.07" />
    <circle cx="12" cy="12" r="3" />
  </svg>
);

export const TogetherAIIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={className}>
    <circle cx="8" cy="12" r="5" />
    <circle cx="16" cy="12" r="5" />
  </svg>
);

export const XAIIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className}>
    <path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z" />
  </svg>
);

export const CohereIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className}>
    <circle cx="8" cy="8" r="4" />
    <circle cx="16" cy="12" r="5" />
    <circle cx="9" cy="17" r="3" />
  </svg>
);

export const AzureIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className}>
    <path d="M13.05 4.24l-4.5 7.82 5.4 6.74H6.26l-3.3 4.2h14.54l-4.45-18.76z" />
    <path d="M15.48 7.37L10.3 16.3h7.32l-2.14-8.93z" />
  </svg>
);

// Helper for dynamic provider icon selection
export const ProviderBrandIcon: React.FC<{
  providerIdOrKind: string;
  name?: string;
  className?: string;
}> = ({ providerIdOrKind, name = '', className = 'w-5 h-5' }) => {
  const query = `${providerIdOrKind} ${name}`.toLowerCase();

  if (query.includes('anthropic') || query.includes('claude')) {
    return <AnthropicIcon className={className} />;
  }
  if (query.includes('gemini') || query.includes('google')) {
    return <GoogleGeminiIcon className={className} />;
  }
  if (query.includes('deepseek')) {
    return <DeepSeekIcon className={className} />;
  }
  if (query.includes('groq')) {
    return <GroqIcon className={className} />;
  }
  if (query.includes('ollama')) {
    return <OllamaIcon className={className} />;
  }
  if (query.includes('openrouter')) {
    return <OpenRouterIcon className={className} />;
  }
  if (query.includes('mistral') || query.includes('codestral')) {
    return <MistralIcon className={className} />;
  }
  if (query.includes('together')) {
    return <TogetherAIIcon className={className} />;
  }
  if (query.includes('perplexity') || query.includes('sonar')) {
    return <PerplexityIcon className={className} />;
  }
  if (query.includes('xai') || query.includes('grok')) {
    return <XAIIcon className={className} />;
  }
  if (query.includes('cohere')) {
    return <CohereIcon className={className} />;
  }
  if (query.includes('azure')) {
    return <AzureIcon className={className} />;
  }
  // Default to OpenAI or OpenAI compatible
  return <OpenAIIcon className={className} />;
};
