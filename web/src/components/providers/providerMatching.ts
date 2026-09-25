import type { Provider } from '../../types';
import {
  KNOWN_PROVIDERS,
  type KnownProviderPreset,
} from './ProviderIcons';

/**
 * Mencocokkan entitas Provider ke KnownProviderPreset dengan klasifikasi multi-pass
 * dan pemisahan tegas antara Google Gemini vs Google Antigravity.
 */
export const matchPresetForProvider = (
  provider: Provider | null | undefined
): KnownProviderPreset | null => {
  if (!provider || typeof provider !== 'object') return null;

  const provName = (provider.name || '').trim().toLowerCase();
  const provDisplay = (provider.display_name || '').trim().toLowerCase();
  const provBaseUrl = (provider.base_url || '').trim().replace(/\/+$/, '');
  const provKind = provider.kind;

  // Pass 1: Prioritas Tertinggi — Deteksi Eksplisit Antigravity
  if (provName.includes('antigravity') || provDisplay.includes('antigravity')) {
    const agPreset = KNOWN_PROVIDERS.find((p) => p.id === 'antigravity');
    if (agPreset) return agPreset;
  }

  // Pass 2: Kecocokan Persis (Exact Match)
  const byExact = KNOWN_PROVIDERS.find((p) => {
    const pName = p.name.toLowerCase();
    const pId = p.id.toLowerCase();
    const pDisplay = p.displayName.toLowerCase();

    if (provName && (pName === provName || pId === provName || pDisplay === provName)) {
      return true;
    }
    if (provDisplay && (pDisplay === provDisplay || pName === provDisplay || pId === provDisplay)) {
      return true;
    }
    return false;
  });
  if (byExact) return byExact;

  // Pass 3: Kecocokan Prefix Nama Provider
  const byPrefix = KNOWN_PROVIDERS.find((p) => {
    if (!provName) return false;
    // Proteksi: Google Gemini tidak boleh mengklaim Antigravity
    if (p.id === 'google' && (provName.includes('antigravity') || provDisplay.includes('antigravity'))) {
      return false;
    }
    const pName = p.name.toLowerCase();
    const pId = p.id.toLowerCase();
    return (
      provName.startsWith(pName) ||
      provName.startsWith(pId) ||
      pId === provName.split('-')[0]
    );
  });
  if (byPrefix) return byPrefix;

  // Pass 4: Kecocokan BaseURL (dinormalisasi)
  if (provBaseUrl) {
    const byBaseUrl = KNOWN_PROVIDERS.find((p) => {
      const pBase = (p.baseUrl || '').trim().replace(/\/+$/, '');
      if (!pBase || pBase !== provBaseUrl) return false;
      // Proteksi segregasi BaseURL
      if (p.id === 'google' && (provName.includes('antigravity') || provDisplay.includes('antigravity'))) {
        return false;
      }
      return true;
    });
    if (byBaseUrl) return byBaseUrl;
  }

  // Pass 5: Kind Fallback Spesifik
  if (provKind && !['openai_compatible', 'custom'].includes(provKind)) {
    if (provKind === 'google') {
      // Kasus Antigravity sudah tersaring di Pass 1 dan Pass 4, sisanya adalah Gemini
      const geminiPreset = KNOWN_PROVIDERS.find((p) => p.id === 'google');
      if (geminiPreset) return geminiPreset;
    } else {
      const byKind = KNOWN_PROVIDERS.find((p) => p.kind === provKind);
      if (byKind) return byKind;
    }
  }

  return null;
};

/**
 * Mencocokkan apakah preset yang dipilih sudah memiliki entitas provider aktif di sistem.
 */
export const matchExistingProvider = (
  preset: KnownProviderPreset | null,
  providersList: Provider[]
): Provider | null => {
  if (!preset || !providersList || !Array.isArray(providersList) || providersList.length === 0) return null;

  const isPresetAntigravity = preset.id === 'antigravity';
  const isPresetGemini = preset.id === 'google';

  const isProvAntigravity = (p: Provider): boolean => {
    if (!p) return false;
    const n = (p.name || '').toLowerCase();
    const dn = (p.display_name || '').toLowerCase();
    return n.includes('antigravity') || dn.includes('antigravity');
  };

  const passesGoogleSegregation = (p: Provider): boolean => {
    if (!p) return false;
    if (isPresetAntigravity && !isProvAntigravity(p)) return false;
    if (isPresetGemini && isProvAntigravity(p)) return false;
    return true;
  };

  // 1. Kecocokan persis pada name atau preset ID
  const byExactName = providersList.find(
    (p) =>
      p &&
      passesGoogleSegregation(p) &&
      (p.name === preset.name || (preset.id && p.name === preset.id))
  );
  if (byExactName) return byExactName;

  // 2. Kecocokan pada BaseURL (dinormalisasi tanpa trailing slash)
  const normPresetBase = (preset.baseUrl || '').trim().replace(/\/+$/, '');
  if (normPresetBase) {
    const byBaseUrl = providersList.find(
      (p) =>
        p &&
        passesGoogleSegregation(p) &&
        (p.base_url || '').trim().replace(/\/+$/, '') === normPresetBase
    );
    if (byBaseUrl) return byBaseUrl;
  }

  // 3. Kecocokan prefix nama provider
  const byPrefix = providersList.find(
    (p) =>
      p &&
      passesGoogleSegregation(p) &&
      typeof p.name === 'string' &&
      ((preset.name && p.name.startsWith(preset.name)) ||
        (preset.id && p.name.startsWith(preset.id)))
  );
  if (byPrefix) return byPrefix;

  // 4. Kecocokan dialect/kind untuk first-party LLM (bukan generic compatible/custom)
  if (preset.kind && !['openai_compatible', 'custom'].includes(preset.kind)) {
    const byKind = providersList.find((p) => {
      if (!p || p.kind !== preset.kind) return false;
      if (preset.kind === 'google') {
        if (isPresetAntigravity) return isProvAntigravity(p);
        if (isPresetGemini) return !isProvAntigravity(p);
      }
      return true;
    });
    if (byKind) return byKind;
  }

  return null;
};
