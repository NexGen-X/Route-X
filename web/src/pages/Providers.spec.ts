import { describe, it, expect } from 'vitest';
import { matchExistingProvider } from './Providers';
import type { KnownProviderPreset } from '../components/providers/ProviderIcons';
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
});
