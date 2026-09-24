import { describe, it, expect } from 'vitest';
import { matchExistingProvider, matchPresetForProvider } from './Providers';
import { KNOWN_PROVIDERS, type KnownProviderPreset } from '../components/providers/ProviderIcons';
import type { Provider } from '../types';

const mockPreset: KnownProviderPreset = {
  id: 'openai',
  name: 'openai-main',
  displayName: 'OpenAI',
  kind: 'openai',
  baseUrl: 'https://api.openai.com/v1',
  tag: 'GPT-4o',
  description: 'OpenAI official',
  color: '#10A37F',
  bgColor: 'rgba(16, 163, 127, 0.12)',
  borderColor: 'rgba(16, 163, 127, 0.35)',
  apiKeyPlaceholder: 'sk-...',
  apiKeyHelp: 'platform.openai.com',
  defaultPriority: 100,
  defaultWeight: 100,
  highlightModels: ['gpt-4o'],
  authLoginType: 'console_token',
  authInstructions: 'Enter API key',
};

const makeProvider = (overrides: Partial<Provider>): Provider => ({
  id: 'prov-1',
  name: 'openai-main',
  display_name: 'OpenAI Production',
  kind: 'openai',
  base_url: 'https://api.openai.com/v1',
  enabled: true,
  priority: 100,
  weight: 100,
  timeout_ms: 30000,
  max_retries: 2,
  consecutive_failures: 0,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
  ...overrides,
});

describe('matchExistingProvider', () => {
  it('mengembalikan null jika preset null atau providers kosong/null', () => {
    expect(matchExistingProvider(null, [])).toBeNull();
    expect(matchExistingProvider(mockPreset, [])).toBeNull();
    expect(matchExistingProvider(null, [makeProvider({})])).toBeNull();
    expect(matchExistingProvider(mockPreset, null as any)).toBeNull();
    expect(matchExistingProvider(mockPreset, undefined as any)).toBeNull();
  });

  it('aman dari elemen array bernilai null, undefined, atau tanpa properti name', () => {
    const listWithHoles = [null, undefined, { id: 'bad' } as any];
    expect(matchExistingProvider(mockPreset, listWithHoles)).toBeNull();
  });

  it('mencocokkan persis berdasarkan name provider', () => {
    const p = makeProvider({ name: 'openai-main' });
    expect(matchExistingProvider(mockPreset, [p])).toBe(p);
  });

  it('mencocokkan berdasarkan preset id jika name provider sama dengan preset id', () => {
    const p = makeProvider({ name: 'openai' });
    expect(matchExistingProvider(mockPreset, [p])).toBe(p);
  });

  it('mencocokkan berdasarkan baseUrl dengan normalisasi trailing slash', () => {
    const pWithSlash = makeProvider({ name: 'custom-name', base_url: 'https://api.openai.com/v1/' });
    expect(matchExistingProvider(mockPreset, [pWithSlash])).toBe(pWithSlash);

    const presetWithSlash: KnownProviderPreset = {
      ...mockPreset,
      baseUrl: 'https://api.openai.com/v1/',
    };
    const pWithoutSlash = makeProvider({ name: 'custom-name', base_url: 'https://api.openai.com/v1' });
    expect(matchExistingProvider(presetWithSlash, [pWithoutSlash])).toBe(pWithoutSlash);
  });

  it('mencocokkan berdasarkan prefix nama provider', () => {
    const p = makeProvider({ name: 'openai-main-pool-backup' });
    expect(matchExistingProvider(mockPreset, [p])).toBe(p);
  });

  it('mencocokkan berdasarkan kind untuk first-party LLM', () => {
    const anthropicPreset: KnownProviderPreset = {
      ...mockPreset,
      id: 'anthropic',
      name: 'anthropic-main',
      kind: 'anthropic',
      baseUrl: 'https://api.anthropic.com/v1',
    };
    const p = makeProvider({
      name: 'claude-dedicated',
      kind: 'anthropic',
      base_url: 'https://proxy.custom.com/anthropic',
    });
    expect(matchExistingProvider(anthropicPreset, [p])).toBe(p);
  });

  it('TIDAK mencocokkan kind untuk openai_compatible atau custom generic', () => {
    const customPreset: KnownProviderPreset = {
      ...mockPreset,
      id: 'custom',
      name: 'custom-llm',
      kind: 'openai_compatible',
      baseUrl: 'https://my-vllm.internal/v1',
    };
    const existingOtherCompatible = makeProvider({
      name: 'ollama-server',
      kind: 'openai_compatible',
      base_url: 'https://ollama.internal/v1',
    });
    expect(matchExistingProvider(customPreset, [existingOtherCompatible])).toBeNull();
  });

  describe('Segregasi Google Gemini vs Google Antigravity', () => {
    const antigravityPreset = KNOWN_PROVIDERS.find((p) => p.id === 'antigravity')!;
    const geminiPreset = KNOWN_PROVIDERS.find((p) => p.id === 'google')!;

    const geminiProv = makeProvider({
      id: 'prov-gemini',
      name: 'google-gemini',
      display_name: 'Google Gemini Pro',
      kind: 'google',
      base_url: 'https://generativelanguage.googleapis.com',
    });

    const antigravityProv = makeProvider({
      id: 'prov-ag',
      name: 'antigravity-deepmind',
      display_name: 'Google Antigravity Enterprise',
      kind: 'google',
      base_url: 'https://daily-cloudcode-pa.googleapis.com',
    });

    it('preset Antigravity TIDAK mencocokkan provider Gemini', () => {
      expect(matchExistingProvider(antigravityPreset, [geminiProv])).toBeNull();
    });

    it('preset Gemini TIDAK mencocokkan provider Antigravity', () => {
      expect(matchExistingProvider(geminiPreset, [antigravityProv])).toBeNull();
    });

    it('mencocokkan preset Antigravity dengan tepat saat kedua provider ada dalam daftar', () => {
      expect(matchExistingProvider(antigravityPreset, [geminiProv, antigravityProv])).toBe(antigravityProv);
    });

    it('mencocokkan preset Gemini dengan tepat saat kedua provider ada dalam daftar', () => {
      expect(matchExistingProvider(geminiPreset, [geminiProv, antigravityProv])).toBe(geminiProv);
    });

    it('tetap mencocokkan dengan tepat meskipun urutan daftar dibalik', () => {
      expect(matchExistingProvider(antigravityPreset, [antigravityProv, geminiProv])).toBe(antigravityProv);
      expect(matchExistingProvider(geminiPreset, [antigravityProv, geminiProv])).toBe(geminiProv);
    });
  });
});

const makePresetTestProvider = (overrides: Partial<Provider>): Provider => ({
  id: 'prov-test',
  name: 'test-provider',
  display_name: 'Test Provider',
  kind: 'custom',
  base_url: '',
  enabled: true,
  priority: 100,
  weight: 100,
  timeout_ms: 30000,
  max_retries: 2,
  consecutive_failures: 0,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
  ...overrides,
});

describe('matchPresetForProvider', () => {
  it('mengembalikan null jika provider null, undefined, atau bukan objek', () => {
    expect(matchPresetForProvider(null)).toBeNull();
    expect(matchPresetForProvider(undefined)).toBeNull();
    expect(matchPresetForProvider('invalid' as any)).toBeNull();
  });

  it('Pass 1: mendeteksi Antigravity berdasarkan nama provider', () => {
    const p = makePresetTestProvider({ name: 'antigravity-deepmind', kind: 'google' });
    const matched = matchPresetForProvider(p);
    expect(matched?.id).toBe('antigravity');
  });

  it('Pass 1: mendeteksi Antigravity pada nama gabungan google-antigravity (bukan Gemini)', () => {
    const p = makePresetTestProvider({ name: 'google-antigravity', kind: 'google' });
    const matched = matchPresetForProvider(p);
    expect(matched?.id).toBe('antigravity');
  });

  it('Pass 1: mendeteksi Antigravity berdasarkan display_name', () => {
    const p = makePresetTestProvider({
      name: 'custom-deepmind',
      display_name: 'Google Antigravity Enterprise',
      kind: 'google',
    });
    const matched = matchPresetForProvider(p);
    expect(matched?.id).toBe('antigravity');
  });

  it('Pass 2: exact match untuk Google Gemini', () => {
    const p = makePresetTestProvider({ name: 'google-gemini', kind: 'google' });
    const matched = matchPresetForProvider(p);
    expect(matched?.id).toBe('google');
  });

  it('Pass 2: exact match untuk provider first-party lain (mis. OpenAI)', () => {
    const p = makePresetTestProvider({ name: 'openai-main', kind: 'openai' });
    const matched = matchPresetForProvider(p);
    expect(matched?.id).toBe('openai');
  });

  it('Pass 3: prefix match untuk Gemini tanpa klaim Antigravity', () => {
    const p = makePresetTestProvider({ name: 'google-gemini-cluster-2', kind: 'google' });
    const matched = matchPresetForProvider(p);
    expect(matched?.id).toBe('google');
  });

  it('Pass 4: baseUrl match untuk Antigravity', () => {
    const p = makePresetTestProvider({
      name: 'corp-agent',
      base_url: 'https://daily-cloudcode-pa.googleapis.com',
    });
    const matched = matchPresetForProvider(p);
    expect(matched?.id).toBe('antigravity');
  });

  it('Pass 4: baseUrl match untuk Gemini dengan normalisasi trailing slash', () => {
    const p = makePresetTestProvider({
      name: 'corp-gemini',
      base_url: 'https://generativelanguage.googleapis.com/',
    });
    const matched = matchPresetForProvider(p);
    expect(matched?.id).toBe('google');
  });

  it('Pass 5: kind fallback untuk Google non-Antigravity diarahkan ke Gemini', () => {
    const p = makePresetTestProvider({ name: 'unnamed-instance', kind: 'google' });
    const matched = matchPresetForProvider(p);
    expect(matched?.id).toBe('google');
  });

  it('Pass 5: kind fallback untuk provider first-party lain (mis. Anthropic)', () => {
    const p = makePresetTestProvider({ name: 'custom-claude', kind: 'anthropic' });
    const matched = matchPresetForProvider(p);
    expect(matched?.id).toBe('anthropic');
  });

  it('Pass 5: kind generic (openai_compatible / custom) mengembalikan null', () => {
    const p = makePresetTestProvider({ name: 'local-vllm', kind: 'openai_compatible' });
    expect(matchPresetForProvider(p)).toBeNull();
  });
});
