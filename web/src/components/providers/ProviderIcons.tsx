import React from 'react';

export type AuthLoginType = 'oauth_fallback' | 'console_token' | 'local_socket';

export interface KnownProviderPreset {
  id: string;
  name: string;
  displayName: string;
  kind: 'openai' | 'anthropic' | 'google' | 'openai_compatible' | 'custom';
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
  // Metadata Alur Auth Login
  authLoginType: AuthLoginType;
  authLoginUrl?: string;
  authLoginLabel?: string;
  authInstructions: string;
  authFallbackHint?: string;
}

export const KNOWN_PROVIDERS: KnownProviderPreset[] = [
  // 1. OpenAI
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
    apiKeyHelp: 'platform.openai.com/api-keys',
    defaultPriority: 100,
    defaultWeight: 100,
    highlightModels: ['gpt-4o', 'gpt-4o-mini', 'o3-mini', 'o1'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://platform.openai.com/api-keys',
    authLoginLabel: 'Buka OpenAI Developer Platform',
    authInstructions: 'Login ke platform OpenAI, buat Secret Key baru pada menu API Keys, lalu salin dan tempelkan token di bawah ini.',
    authFallbackHint: 'sk-proj-... atau sk-...',
  },
  // 2. Anthropic
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
    apiKeyHelp: 'console.anthropic.com/settings/keys',
    defaultPriority: 100,
    defaultWeight: 100,
    highlightModels: ['claude-3-7-sonnet', 'claude-3-5-sonnet', 'claude-3-5-haiku'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://console.anthropic.com/settings/keys',
    authLoginLabel: 'Buka Anthropic Console',
    authInstructions: 'Masuk ke konsol Anthropic, buat API Key baru di menu Settings > API Keys, lalu tempelkan token yang didapatkan.',
    authFallbackHint: 'sk-ant-api03-...',
  },
  // 3. Google Gemini
  {
    id: 'google',
    name: 'google-gemini',
    displayName: 'Google Gemini',
    kind: 'google',
    baseUrl: 'https://generativelanguage.googleapis.com',
    tag: 'Gemini 2.5 Pro & 2.0 Flash',
    description: 'Model multimodal Google dengan konteks 2M token dan kecepatan inferensi terdepan.',
    color: '#3B82F6',
    bgColor: 'rgba(59, 130, 246, 0.12)',
    borderColor: 'rgba(59, 130, 246, 0.35)',
    apiKeyPlaceholder: 'AIzaSy...',
    apiKeyHelp: 'aistudio.google.com/app/apikey',
    defaultPriority: 100,
    defaultWeight: 100,
    highlightModels: ['gemini-2.5-pro', 'gemini-2.0-flash', 'gemini-1.5-pro'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://aistudio.google.com/app/apikey',
    authLoginLabel: 'Buka Google AI Studio',
    authInstructions: 'Login dengan Akun Google Anda, klik "Create API key", salin kunci token Google AI Studio, dan tempelkan di bawah ini.',
    authFallbackHint: 'AIzaSy...',
  },
  // 4. Google Antigravity (DeepMind Agentic Platform)
  {
    id: 'antigravity',
    name: 'antigravity-deepmind',
    displayName: 'Google Antigravity',
    kind: 'google',
    baseUrl: 'https://generativelanguage.googleapis.com',
    tag: 'OAuth Fallback Multi-Account',
    description: 'Platform AI otonom dari Google DeepMind. Autentikasi OAuth dengan multi-account pooling & auto-refresh token otomatis.',
    color: '#8B5CF6',
    bgColor: 'rgba(139, 92, 246, 0.12)',
    borderColor: 'rgba(139, 92, 246, 0.35)',
    apiKeyPlaceholder: 'Tempel seluruh URL redirect (http://localhost:4567/?code=...) atau authorization code...',
    apiKeyHelp: 'accounts.google.com',
    defaultPriority: 95,
    defaultWeight: 100,
    highlightModels: ['gemini-2.5-pro', 'gemini-2.0-flash', 'gemini-1.5-pro'],
    authLoginType: 'oauth_fallback',
    authLoginUrl: 'https://accounts.google.com/o/oauth2/v2/auth?client_id=1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com&redirect_uri=http://localhost:4567&response_type=code&scope=https://www.googleapis.com/auth/cloud-platform%20https://www.googleapis.com/auth/generative-language.retrieval&access_type=offline&prompt=consent',
    authLoginLabel: 'Login dengan Akun Google Antigravity',
    authInstructions: 'Klik tombol di bawah untuk membuka halaman otorisasi Google di browser. Setelah otorisasi berhasil, browser akan dialihkan ke URL lokal (mis. http://localhost:4567/?code=4/0A...). Salin seluruh URL redirect tersebut atau kode otorisasi dan tempelkan ke kolom di bawah. Backend Route-X akan otomatis menukarkan token dan mengaktifkan auto-refresh tanpa perlu konfigurasi Client ID / Secret manual.',
    authFallbackHint: 'http://localhost:4567/?code=4/0A... atau kode otorisasi 4/0A...',
  },
  // 5. Cerebras (Ultra-Fast Wafer Scale)
  {
    id: 'cerebras',
    name: 'cerebras-fast',
    displayName: 'Cerebras',
    kind: 'openai_compatible',
    baseUrl: 'https://api.cerebras.ai/v1',
    tag: 'Tercepat Dunia (~2000 tps)',
    description: 'Inference super kilat berbasis chip wafer-scale CS-3 untuk streaming coding tanpa jeda.',
    color: '#FF6B00',
    bgColor: 'rgba(255, 107, 0, 0.12)',
    borderColor: 'rgba(255, 107, 0, 0.35)',
    apiKeyPlaceholder: 'csk-...',
    apiKeyHelp: 'cloud.cerebras.ai/platform',
    defaultPriority: 98,
    defaultWeight: 100,
    highlightModels: ['llama-3.3-70b', 'llama-3.1-8b', 'deepseek-r1-distill-llama-70b'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://cloud.cerebras.ai/platform',
    authLoginLabel: 'Buka Cerebras Cloud Portal',
    authInstructions: 'Login ke Cerebras Cloud, navigasi ke tab API Keys, buat secret token baru berawalan csk- dan tempelkan di sini.',
    authFallbackHint: 'csk-...',
  },
  // 5. SambaNova (RDU 405B / 70B)
  {
    id: 'sambanova',
    name: 'sambanova-rdu',
    displayName: 'SambaNova',
    kind: 'openai_compatible',
    baseUrl: 'https://api.sambanova.ai/v1',
    tag: 'RDU Llama 405B & 70B',
    description: 'Akselerator Reconfigurable Dataflow Unit (RDU) untuk menjalankan model 405B berkecepatan tinggi.',
    color: '#E11D48',
    bgColor: 'rgba(225, 29, 72, 0.12)',
    borderColor: 'rgba(225, 29, 72, 0.35)',
    apiKeyPlaceholder: 'Masukkan SambaNova API key...',
    apiKeyHelp: 'cloud.sambanova.ai/apis',
    defaultPriority: 96,
    defaultWeight: 100,
    highlightModels: ['Meta-Llama-3.1-405B-Instruct', 'Meta-Llama-3.3-70B-Instruct', 'DeepSeek-R1'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://cloud.sambanova.ai/apis',
    authLoginLabel: 'Buka SambaNova Cloud',
    authInstructions: 'Login ke portal SambaNova Cloud, salin API Token Anda di menu APIs, kemudian tempelkan di bawah ini.',
    authFallbackHint: 'UUID / token string',
  },
  // 6. Fireworks AI
  {
    id: 'fireworks',
    name: 'fireworks-main',
    displayName: 'Fireworks AI',
    kind: 'openai_compatible',
    baseUrl: 'https://api.fireworks.ai/inference/v1',
    tag: 'Compound Inference Cepat',
    description: 'Platform inferensi serverless ultra-cepat favorit developer perkakas Cursor & Cline.',
    color: '#8B5CF6',
    bgColor: 'rgba(139, 92, 246, 0.12)',
    borderColor: 'rgba(139, 92, 246, 0.35)',
    apiKeyPlaceholder: 'fw_...',
    apiKeyHelp: 'fireworks.ai/account/api-keys',
    defaultPriority: 95,
    defaultWeight: 100,
    highlightModels: ['accounts/fireworks/models/deepseek-r1', 'accounts/fireworks/models/qwen2p5-coder-32b-instruct'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://fireworks.ai/account/api-keys',
    authLoginLabel: 'Buka Fireworks Dashboard',
    authInstructions: 'Buka Fireworks AI Account, generate API key baru berawalan fw_ dan tempelkan di bawah ini.',
    authFallbackHint: 'fw_...',
  },
  // 7. Alibaba Qwen (DashScope)
  {
    id: 'qwen',
    name: 'qwen-dashscope',
    displayName: 'Alibaba Qwen',
    kind: 'openai_compatible',
    baseUrl: 'https://dashscope-intl.aliyuncs.com/compatible-mode/v1',
    tag: 'Qwen 2.5 Coder No.1',
    description: 'Pencipta model open-weights coding terbaik dunia Qwen 2.5 Coder dan penalaran Qwen-Max.',
    color: '#FF6A00',
    bgColor: 'rgba(255, 106, 0, 0.12)',
    borderColor: 'rgba(255, 106, 0, 0.35)',
    apiKeyPlaceholder: 'sk-...',
    apiKeyHelp: 'dashscope.console.aliyun.com',
    defaultPriority: 94,
    defaultWeight: 100,
    highlightModels: ['qwen2.5-coder-32b-instruct', 'qwen-max', 'qwen-plus'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://dashscope.console.aliyun.com/apiKey',
    authLoginLabel: 'Buka Alibaba DashScope Console',
    authInstructions: 'Login ke konsol Alibaba Cloud DashScope, buat API Key baru pada menu API-KEY Management, lalu tempelkan token di sini.',
    authFallbackHint: 'sk-...',
  },
  // 8. SiliconFlow (SiliconCloud)
  {
    id: 'siliconflow',
    name: 'siliconflow-main',
    displayName: 'SiliconFlow',
    kind: 'openai_compatible',
    baseUrl: 'https://api.siliconflow.cn/v1',
    tag: 'DeepSeek R1/V3 Cloud',
    description: 'Cloud GPU teroptimasi berbiaya rendah dengan throughput tinggi untuk DeepSeek dan Qwen.',
    color: '#0EA5E9',
    bgColor: 'rgba(14, 165, 233, 0.12)',
    borderColor: 'rgba(14, 165, 233, 0.35)',
    apiKeyPlaceholder: 'sk-...',
    apiKeyHelp: 'cloud.siliconflow.cn/account/ak',
    defaultPriority: 92,
    defaultWeight: 100,
    highlightModels: ['deepseek-ai/DeepSeek-R1', 'deepseek-ai/DeepSeek-V3', 'Qwen/Qwen2.5-72B-Instruct'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://cloud.siliconflow.cn/account/ak',
    authLoginLabel: 'Buka SiliconCloud Portal',
    authInstructions: 'Login ke SiliconCloud, buat API Key pada menu Access Keys, lalu salin dan tempelkan token tersebut di sini.',
    authFallbackHint: 'sk-...',
  },
  // 9. DeepSeek
  {
    id: 'deepseek',
    name: 'deepseek-main',
    displayName: 'DeepSeek',
    kind: 'openai_compatible',
    baseUrl: 'https://api.deepseek.com',
    tag: 'V3 & R1 Reasoner',
    description: 'Inference super hemat dan penalaran canggih resmi dari DeepSeek-R1 dan DeepSeek-V3.',
    color: '#06B6D4',
    bgColor: 'rgba(6, 182, 212, 0.12)',
    borderColor: 'rgba(6, 182, 212, 0.35)',
    apiKeyPlaceholder: 'sk-...',
    apiKeyHelp: 'platform.deepseek.com/api_keys',
    defaultPriority: 90,
    defaultWeight: 100,
    highlightModels: ['deepseek-chat', 'deepseek-reasoner'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://platform.deepseek.com/api_keys',
    authLoginLabel: 'Buka DeepSeek Platform',
    authInstructions: 'Login ke DeepSeek Open Platform, generate API Key baru, lalu salin dan tempelkan di bawah ini.',
    authFallbackHint: 'sk-...',
  },
  // 10. Groq
  {
    id: 'groq',
    name: 'groq-fast',
    displayName: 'Groq LPU',
    kind: 'openai_compatible',
    baseUrl: 'https://api.groq.com/openai/v1',
    tag: 'LPU Latensi Rendah',
    description: 'Pemrosesan chip LPU dengan latensi instan untuk Llama-3.3, Mixtral, dan DeepSeek-R1 Distill.',
    color: '#F97316',
    bgColor: 'rgba(249, 115, 22, 0.12)',
    borderColor: 'rgba(249, 115, 22, 0.35)',
    apiKeyPlaceholder: 'gsk_...',
    apiKeyHelp: 'console.groq.com/keys',
    defaultPriority: 95,
    defaultWeight: 100,
    highlightModels: ['llama-3.3-70b-versatile', 'llama-3.1-8b-instant', 'deepseek-r1-distill-llama-70b'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://console.groq.com/keys',
    authLoginLabel: 'Buka Groq Console',
    authInstructions: 'Login ke Groq Console, klik "Create API Key", lalu salin kunci yang dimulai dengan gsk_ ke kotak di bawah.',
    authFallbackHint: 'gsk_...',
  },
  // 11. Hugging Face (OAuth Fallback / User Access Token)
  {
    id: 'huggingface',
    name: 'huggingface-hub',
    displayName: 'Hugging Face',
    kind: 'openai_compatible',
    baseUrl: 'https://api-inference.huggingface.co/v1',
    tag: 'Hub Komunitas Terbesar',
    description: 'Inference API serverless ke ribuan model open-source komunitas Hugging Face.',
    color: '#FFD21E',
    bgColor: 'rgba(255, 210, 30, 0.12)',
    borderColor: 'rgba(255, 210, 30, 0.35)',
    apiKeyPlaceholder: 'hf_...',
    apiKeyHelp: 'huggingface.co/settings/tokens',
    defaultPriority: 85,
    defaultWeight: 100,
    highlightModels: ['meta-llama/Llama-3.3-70B-Instruct', 'deepseek-ai/DeepSeek-R1'],
    authLoginType: 'oauth_fallback',
    authLoginUrl: 'https://huggingface.co/settings/tokens',
    authLoginLabel: 'Otorisasi di Hugging Face',
    authInstructions: 'Klik tombol otorisasi untuk membuka Hugging Face. Buat token baru atau salin URL redirect fallback / User Access Token Anda ke input di bawah ini.',
    authFallbackHint: 'hf_... atau URL callback https://huggingface.co/...',
  },
  // 12. Cloudflare Workers AI
  {
    id: 'cloudflare',
    name: 'cloudflare-ai',
    displayName: 'Cloudflare AI',
    kind: 'openai_compatible',
    baseUrl: 'https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1',
    tag: 'Global Edge Network',
    description: 'Inference terdistribusi di ratusan edge data center Cloudflare di seluruh dunia.',
    color: '#F48120',
    bgColor: 'rgba(244, 129, 32, 0.12)',
    borderColor: 'rgba(244, 129, 32, 0.35)',
    apiKeyPlaceholder: 'Cloudflare API Token...',
    apiKeyHelp: 'dash.cloudflare.com/profile/api-tokens',
    defaultPriority: 85,
    defaultWeight: 100,
    highlightModels: ['@cf/meta/llama-3.3-70b-instruct', '@cf/deepseek-ai/deepseek-r1-distill-qwen-32b'],
    authLoginType: 'oauth_fallback',
    authLoginUrl: 'https://dash.cloudflare.com/profile/api-tokens',
    authLoginLabel: 'Buka Cloudflare API Tokens',
    authInstructions: 'Buka Cloudflare Dashboard, buat API Token dengan izin Workers AI Read/Write, lalu tempelkan token atau URL callback di bawah.',
    authFallbackHint: 'API Token Cloudflare',
  },
  // 13. OpenRouter
  {
    id: 'openrouter',
    name: 'openrouter-hub',
    displayName: 'OpenRouter',
    kind: 'openai_compatible',
    baseUrl: 'https://openrouter.ai/api/v1',
    tag: 'Meta Aggregator 100+',
    description: 'Pintu gerbang tunggal fleksibel ke seluruh model komersial dan open-source.',
    color: '#6366F1',
    bgColor: 'rgba(99, 102, 241, 0.12)',
    borderColor: 'rgba(99, 102, 241, 0.35)',
    apiKeyPlaceholder: 'sk-or-v1-...',
    apiKeyHelp: 'openrouter.ai/keys',
    defaultPriority: 85,
    defaultWeight: 100,
    highlightModels: ['anthropic/claude-3.7-sonnet', 'openai/gpt-4o', 'deepseek/deepseek-r1'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://openrouter.ai/keys',
    authLoginLabel: 'Buka OpenRouter Keys',
    authInstructions: 'Login ke OpenRouter, buat API Key baru pada menu Keys, lalu tempelkan token sk-or-v1- di bawah ini.',
    authFallbackHint: 'sk-or-v1-...',
  },
  // 14. Mistral AI
  {
    id: 'mistral',
    name: 'mistral-main',
    displayName: 'Mistral AI',
    kind: 'openai_compatible',
    baseUrl: 'https://api.mistral.ai/v1',
    tag: 'Mistral Large 2 & Codestral',
    description: 'Model AI Eropa berkinerja tinggi: Mistral Large 2, Codestral, dan Pixtral.',
    color: '#EC4899',
    bgColor: 'rgba(236, 72, 153, 0.12)',
    borderColor: 'rgba(236, 72, 153, 0.35)',
    apiKeyPlaceholder: 'secret-...',
    apiKeyHelp: 'console.mistral.ai/api-keys',
    defaultPriority: 85,
    defaultWeight: 100,
    highlightModels: ['mistral-large-latest', 'codestral-latest', 'mistral-small-latest'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://console.mistral.ai/api-keys',
    authLoginLabel: 'Buka Mistral Console',
    authInstructions: 'Login ke La Plateforme Mistral, buat API Key baru di menu Codestral/API Keys, lalu tempelkan di sini.',
    authFallbackHint: 'secret-...',
  },
  // 15. Novita AI
  {
    id: 'novita',
    name: 'novita-ai',
    displayName: 'Novita AI',
    kind: 'openai_compatible',
    baseUrl: 'https://api.novita.ai/v3/openai',
    tag: 'GPU Throughput Tinggi',
    description: 'Infrastruktur GPU serverless cepat dan terjangkau untuk model DeepSeek dan Llama.',
    color: '#059669',
    bgColor: 'rgba(5, 150, 105, 0.12)',
    borderColor: 'rgba(5, 150, 105, 0.35)',
    apiKeyPlaceholder: 'Masukkan Novita key...',
    apiKeyHelp: 'novita.ai/settings/key-management',
    defaultPriority: 85,
    defaultWeight: 100,
    highlightModels: ['deepseek/deepseek-r1', 'meta-llama/llama-3.3-70b-instruct'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://novita.ai/settings/key-management',
    authLoginLabel: 'Buka Novita Key Management',
    authInstructions: 'Login ke Novita AI, generate Key baru pada menu Key Management, lalu tempelkan di kotak bawah.',
    authFallbackHint: 'Key string',
  },
  // 16. Together AI
  {
    id: 'together',
    name: 'together-ai',
    displayName: 'Together AI',
    kind: 'openai_compatible',
    baseUrl: 'https://api.together.xyz/v1',
    tag: 'Open Source Cloud',
    description: 'Cloud inference untuk model open-source (Llama, Qwen, DeepSeek) kecepatan tinggi.',
    color: '#14B8A6',
    bgColor: 'rgba(20, 184, 166, 0.12)',
    borderColor: 'rgba(20, 184, 166, 0.35)',
    apiKeyPlaceholder: 'together-...',
    apiKeyHelp: 'api.together.ai/settings/api-keys',
    defaultPriority: 80,
    defaultWeight: 100,
    highlightModels: ['meta-llama/Llama-3.3-70B-Instruct-Turbo', 'Qwen/Qwen2.5-72B-Instruct-Turbo'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://api.together.ai/settings/api-keys',
    authLoginLabel: 'Buka Together AI Settings',
    authInstructions: 'Login ke dashboard Together AI, salin API key akun Anda di menu Settings > API Keys.',
    authFallbackHint: 'together-...',
  },
  // 17. Perplexity AI
  {
    id: 'perplexity',
    name: 'perplexity-ai',
    displayName: 'Perplexity AI',
    kind: 'openai_compatible',
    baseUrl: 'https://api.perplexity.ai',
    tag: 'Sonar Real-Time Web',
    description: 'Model inferensi yang diperkaya kemampuan pencarian web real-time untuk riset aktual.',
    color: '#0284C7',
    bgColor: 'rgba(2, 132, 199, 0.12)',
    borderColor: 'rgba(2, 132, 199, 0.35)',
    apiKeyPlaceholder: 'pplx-...',
    apiKeyHelp: 'perplexity.ai/settings/api',
    defaultPriority: 80,
    defaultWeight: 100,
    highlightModels: ['sonar-pro', 'sonar', 'sonar-reasoning'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://www.perplexity.ai/settings/api',
    authLoginLabel: 'Buka Perplexity Settings',
    authInstructions: 'Buka pengaturan akun Perplexity, navigasi ke tab API, buat API Key baru dan tempelkan di bawah ini.',
    authFallbackHint: 'pplx-...',
  },
  // 18. xAI (Grok)
  {
    id: 'xai',
    name: 'xai-grok',
    displayName: 'xAI Grok',
    kind: 'openai_compatible',
    baseUrl: 'https://api.x.ai/v1',
    tag: 'Grok-2 & Grok Beta',
    description: 'Model penalaran cerdas dari xAI dengan pemahaman mendalam dan akses data real-time.',
    color: '#E2E8F0',
    bgColor: 'rgba(226, 232, 240, 0.12)',
    borderColor: 'rgba(226, 232, 240, 0.35)',
    apiKeyPlaceholder: 'xai-...',
    apiKeyHelp: 'console.x.ai',
    defaultPriority: 80,
    defaultWeight: 100,
    highlightModels: ['grok-2', 'grok-2-mini'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://console.x.ai',
    authLoginLabel: 'Buka xAI Console',
    authInstructions: 'Login ke konsol xAI di console.x.ai, buat API Key baru, lalu salin dan tempelkan di bawah ini.',
    authFallbackHint: 'xai-...',
  },
  // 19. Cohere
  {
    id: 'cohere',
    name: 'cohere-ai',
    displayName: 'Cohere',
    kind: 'openai_compatible',
    baseUrl: 'https://api.cohere.ai/v1',
    tag: 'Command R+ & Embeddings',
    description: 'Model enterprise untuk RAG, summarization, dan embedding multi-bahasa.',
    color: '#10B981',
    bgColor: 'rgba(16, 185, 129, 0.12)',
    borderColor: 'rgba(16, 185, 129, 0.35)',
    apiKeyPlaceholder: 'co-...',
    apiKeyHelp: 'dashboard.cohere.com/api-keys',
    defaultPriority: 75,
    defaultWeight: 100,
    highlightModels: ['command-r-plus', 'command-r'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://dashboard.cohere.com/api-keys',
    authLoginLabel: 'Buka Cohere Dashboard',
    authInstructions: 'Login ke dashboard Cohere, salin Production Key atau Trial Key akun Anda ke kotak di bawah.',
    authFallbackHint: 'co-...',
  },
  // 20. Azure OpenAI
  {
    id: 'azure',
    name: 'azure-openai',
    displayName: 'Azure OpenAI',
    kind: 'openai',
    baseUrl: 'https://RESOURCE.openai.azure.com/openai/deployments/DEPLOYMENT',
    tag: 'Enterprise Managed Cloud',
    description: 'Layanan terkelola OpenAI di infrastruktur Microsoft Azure dengan kepatuhan enterprise.',
    color: '#0078D4',
    bgColor: 'rgba(0, 120, 212, 0.12)',
    borderColor: 'rgba(0, 120, 212, 0.35)',
    apiKeyPlaceholder: 'azure-api-key...',
    apiKeyHelp: 'portal.azure.com (Keys & Endpoint)',
    defaultPriority: 90,
    defaultWeight: 100,
    highlightModels: ['gpt-4o', 'gpt-4o-mini'],
    authLoginType: 'console_token',
    authLoginUrl: 'https://portal.azure.com',
    authLoginLabel: 'Buka Azure Portal',
    authInstructions: 'Login ke Azure Portal, buka Azure OpenAI Resource Anda, salin Key 1 pada menu Keys and Endpoint.',
    authFallbackHint: 'Key 1 atau Key 2 Azure',
  },
  // 21. Ollama Local
  {
    id: 'ollama',
    name: 'ollama-local',
    displayName: 'Ollama Local',
    kind: 'openai_compatible',
    baseUrl: 'http://localhost:11434',
    tag: 'Offline / Self-Hosted',
    description: 'Eksekusi model mandiri secara lokal di workstation tanpa biaya API eksternal.',
    color: '#A855F7',
    bgColor: 'rgba(168, 85, 247, 0.12)',
    borderColor: 'rgba(168, 85, 247, 0.35)',
    apiKeyPlaceholder: 'ollama-local (bisa dikosongkan)',
    apiKeyHelp: 'Default port: 11434',
    defaultPriority: 80,
    defaultWeight: 100,
    highlightModels: ['llama3.2:latest', 'qwen2.5-coder:latest', 'deepseek-r1:8b'],
    authLoginType: 'local_socket',
    authInstructions: 'Pastikan daemon Ollama berjalan di komputer atau server ini (port 11434). Tidak memerlukan API key eksternal.',
    authFallbackHint: 'http://localhost:11434',
  },
  // 22. LM Studio
  {
    id: 'lmstudio',
    name: 'lmstudio-local',
    displayName: 'LM Studio',
    kind: 'openai_compatible',
    baseUrl: 'http://localhost:1234/v1',
    tag: 'Desktop Server (1234)',
    description: 'Server API lokal kompatibel OpenAI yang dijalankan dari aplikasi desktop LM Studio.',
    color: '#38BDF8',
    bgColor: 'rgba(56, 189, 248, 0.12)',
    borderColor: 'rgba(56, 189, 248, 0.35)',
    apiKeyPlaceholder: 'lm-studio (opsional)',
    apiKeyHelp: 'Default port: 1234',
    defaultPriority: 80,
    defaultWeight: 100,
    highlightModels: ['local-model'],
    authLoginType: 'local_socket',
    authInstructions: 'Buka aplikasi desktop LM Studio, aktifkan "Local Server" pada port 1234. Route-X akan terhubung langsung tanpa API key.',
    authFallbackHint: 'http://localhost:1234/v1',
  },
  // 23. vLLM Engine
  {
    id: 'vllm',
    name: 'vllm-server',
    displayName: 'vLLM Engine',
    kind: 'openai_compatible',
    baseUrl: 'http://localhost:8000/v1',
    tag: 'High-Throughput GPU',
    description: 'Engine inferensi produksi GPU mandiri dengan PagedAttention dan throughput tinggi.',
    color: '#10B981',
    bgColor: 'rgba(16, 185, 129, 0.12)',
    borderColor: 'rgba(16, 185, 129, 0.35)',
    apiKeyPlaceholder: 'vllm-token (jika diatur)',
    apiKeyHelp: 'Default port: 8000',
    defaultPriority: 80,
    defaultWeight: 100,
    highlightModels: ['vllm-model'],
    authLoginType: 'local_socket',
    authInstructions: 'Pastikan container atau server vLLM berjalan dengan endpoint OpenAI kompatibel di port 8000.',
    authFallbackHint: 'http://localhost:8000/v1',
  },
];

// ==========================================
// SVG Vector Icons for all Known AI Providers
// ==========================================

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

export const CerebrasIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className}>
    <rect x="3" y="3" width="18" height="18" rx="3" fill="none" stroke="currentColor" strokeWidth="2" />
    <circle cx="12" cy="12" r="4" />
    <path d="M12 3v3m0 12v3M3 12h3m12 0h3" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
  </svg>
);

export const SambaNovaIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className}>
    <path d="M12 2L2 7l10 5 10-5-10-5zm0 9l-8-4v8l8 4 8-4v-8l-8 4z" />
  </svg>
);

export const FireworksIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={className}>
    <path d="M12 2v4m0 12v4M4.93 4.93l2.83 2.83m8.48 8.48l2.83 2.83M2 12h4m12 0h4M4.93 19.07l2.83-2.83m8.48-8.48l2.83-2.83" />
    <circle cx="12" cy="12" r="3" fill="currentColor" />
  </svg>
);

export const QwenIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className}>
    <path d="M12 2a10 10 0 0 0-8.94 14.5L2 22l5.5-1.06A10 10 0 1 0 12 2zm1 14h-2v-2h2v2zm0-4h-2V7h2v5z" />
  </svg>
);

export const SiliconFlowIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={className}>
    <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
    <path d="M5 22h14" />
  </svg>
);

export const HuggingFaceIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className}>
    <path d="M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2zm-3 8a1.5 1.5 0 1 1 1.5-1.5A1.5 1.5 0 0 1 9 10zm6 0a1.5 1.5 0 1 1 1.5-1.5A1.5 1.5 0 0 1 15 10zm-3 7a5 5 0 0 1-4.24-2.35.75.75 0 1 1 1.28-.78 3.5 3.5 0 0 0 5.92 0 .75.75 0 1 1 1.28.78A5 5 0 0 1 12 17z" />
  </svg>
);

export const CloudflareIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className}>
    <path d="M19.35 10.04C18.67 6.59 15.64 4 12 4 9.11 4 6.6 5.64 5.35 8.04 2.34 8.36 0 10.91 0 14c0 3.31 2.69 6 6 6h13c2.76 0 5-2.24 5-5 0-2.64-2.05-4.78-4.65-4.96z" />
  </svg>
);

export const NovitaIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className}>
    <path d="M12 2L2 7v10l10 5 10-5V7L12 2zm0 2.8l7.5 3.75L12 12.3 4.5 8.55 12 4.8zM4 10.25l7 3.5v7l-7-3.5v-7zm9 10.5v-7l7-3.5v7l-7 3.5z" />
  </svg>
);

export const LMStudioIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={className}>
    <rect x="2" y="3" width="20" height="14" rx="2" />
    <line x1="8" y1="21" x2="16" y2="21" />
    <line x1="12" y1="17" x2="12" y2="21" />
    <path d="M7 8l3 3-3 3" />
    <line x1="12" y1="14" x2="16" y2="14" />
  </svg>
);

export const VLLMIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={className}>
    <path d="M4.5 16.5c-1.5 1.26-2 5-2 5s3.74-.5 5-2c.71-.84.7-2.13-.09-2.91a2.18 2.18 0 0 0-2.91-.09z" />
    <path d="M12 15l-3-3a22 22 0 0 1 2-3.95A12.88 12.88 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.35 22.35 0 0 1-4 2z" />
    <path d="M9 12H4s.55-3.03 2-4.5c1.62-1.63 5-2 5-2" />
    <path d="M12 15v5s3.03-.55 4.5-2c1.63-1.62 2-5 2-5" />
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

export const CustomProviderIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.8"
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
  >
    <rect x="4" y="4" width="16" height="16" rx="3.5" stroke="currentColor" strokeWidth="1.8" />
    <rect x="8.5" y="8.5" width="7" height="7" rx="1.5" fill="currentColor" fillOpacity="0.25" stroke="currentColor" strokeWidth="1.5" />
    <path d="M9 1v3M15 1v3M9 20v3M15 20v3M1 9h3M1 15h3M20 9h3M20 15h3" strokeWidth="1.8" />
    <circle cx="12" cy="12" r="1" fill="currentColor" />
  </svg>
);

export const AntigravityIcon: React.FC<{ className?: string }> = ({ className = 'w-5 h-5' }) => (
  <svg
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.8"
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
  >
    {/* Inti partikel agen gravitational */}
    <circle cx="12" cy="12" r="2.5" fill="currentColor" />
    {/* Cincin orbit gravitasi ganda bersilangan */}
    <ellipse cx="12" cy="12" rx="9" ry="4.5" transform="rotate(-30 12 12)" />
    <ellipse cx="12" cy="12" rx="9" ry="4.5" transform="rotate(30 12 12)" />
    {/* Pulsar ascensi anti-gravitasi kutub atas & bawah */}
    <circle cx="12" cy="3" r="1" fill="currentColor" />
    <circle cx="12" cy="21" r="1" fill="currentColor" />
  </svg>
);

// Helper for dynamic provider icon selection
export const ProviderBrandIcon: React.FC<{
  providerIdOrKind: string;
  name?: string;
  className?: string;
  isCustom?: boolean;
}> = ({ providerIdOrKind, name = '', className = 'w-5 h-5', isCustom }) => {
  if (isCustom) {
    return <CustomProviderIcon className={className} />;
  }

  const query = `${providerIdOrKind} ${name}`.toLowerCase();

  if (query.includes('custom') || query.includes('manual')) {
    return <CustomProviderIcon className={className} />;
  }
  if (query.includes('antigravity') || query.includes('agy')) {
    return <AntigravityIcon className={className} />;
  }
  if (query.includes('anthropic') || query.includes('claude')) {
    return <AnthropicIcon className={className} />;
  }
  if (query.includes('gemini') || query.includes('google')) {
    return <GoogleGeminiIcon className={className} />;
  }
  if (query.includes('cerebras')) {
    return <CerebrasIcon className={className} />;
  }
  if (query.includes('sambanova')) {
    return <SambaNovaIcon className={className} />;
  }
  if (query.includes('fireworks')) {
    return <FireworksIcon className={className} />;
  }
  if (query.includes('qwen') || query.includes('dashscope') || query.includes('alibaba')) {
    return <QwenIcon className={className} />;
  }
  if (query.includes('siliconflow') || query.includes('siliconcloud')) {
    return <SiliconFlowIcon className={className} />;
  }
  if (query.includes('huggingface') || query.includes('hf')) {
    return <HuggingFaceIcon className={className} />;
  }
  if (query.includes('cloudflare')) {
    return <CloudflareIcon className={className} />;
  }
  if (query.includes('novita')) {
    return <NovitaIcon className={className} />;
  }
  if (query.includes('lmstudio') || query.includes('lm-studio')) {
    return <LMStudioIcon className={className} />;
  }
  if (query.includes('vllm')) {
    return <VLLMIcon className={className} />;
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
  if (
    query.includes('openai-main') ||
    name.toLowerCase().includes('openai') ||
    providerIdOrKind.toLowerCase() === 'openai'
  ) {
    return <OpenAIIcon className={className} />;
  }
  return <CustomProviderIcon className={className} />;
};
