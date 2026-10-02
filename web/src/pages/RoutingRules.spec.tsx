import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { RoutingRules } from './RoutingRules';
import type { Model, Provider, RoutingRule } from '../types';

// Harness: seluruh dependensi eksternal (api client + toast context) dimock agar
// test halaman ini hanya mengukur perilaku form & deteksi mode, bukan jaringan.

const buatModel = (id: string, model_id: string, display_name: string): Model => ({
  id,
  model_id,
  display_name,
  capabilities: [],
  enabled: true,
  routing_priority: 0,
  created_at: '',
  updated_at: '',
});

const MODEL_GPT5 = buatModel('m-gpt5', 'gpt-5', 'GPT-5');
const MODEL_GEMINI = buatModel('m-gemini', 'gemini-2.5-flash', 'Gemini 2.5 Flash');
const MODEL_LLAMA = buatModel('m-llama', 'llama-3', 'Llama 3');

const PROVIDER_A: Provider = {
  id: 'prov-a',
  name: 'Provider A',
  display_name: 'Provider A',
  kind: 'openai',
  base_url: 'https://a.test',
  enabled: true,
  priority: 1,
  weight: 1,
  timeout_ms: 10000,
  max_retries: 2,
  consecutive_failures: 0,
  created_at: '',
  updated_at: '',
};

// vi.mock harus hoisting; payloadStore dipakai test untuk membaca payload yang
// dikirim ke api.routing.create/update.
const payloadStore = {
  create: null as Record<string, unknown> | null,
  update: null as Record<string, unknown> | null,
};

vi.mock('../api/client', () => ({
  api: {
    routing: {
      list: vi.fn(async () => ({ items: [] as RoutingRule[] })),
      create: vi.fn(async (data: Record<string, unknown>) => {
        payloadStore.create = data;
        return { id: 'r-baru' } as RoutingRule;
      }),
      update: vi.fn(async (_id: string, data: Record<string, unknown>) => {
        payloadStore.update = data;
        return { id: _id } as RoutingRule;
      }),
      delete: vi.fn(async () => undefined),
      toggle: vi.fn(async () => ({}) as RoutingRule),
      setProviders: vi.fn(async () => ({}) as RoutingRule),
    },
    models: {
      list: vi.fn(async () => ({ items: [MODEL_GPT5, MODEL_GEMINI, MODEL_LLAMA] })),
      addAlias: vi.fn(async () => ({})),
    },
    providers: {
      list: vi.fn(async () => ({ items: [PROVIDER_A] })),
    },
    breakers: {
      list: vi.fn(async () => ({ items: [] })),
      reset: vi.fn(async () => undefined),
    },
  },
}));

vi.mock('../context/ToastContext', () => ({
  useToast: () => ({
    toast: { success: vi.fn(), error: vi.fn(), warn: vi.fn(), info: vi.fn() },
    showToast: vi.fn(),
    confirmModal: vi.fn(async () => true),
  }),
}));

// jsdom tidak punya layout, jadi focus-trap-react melempar "must have at least
// one tabbable node" saat drawer dibuka. Drawer hanya butuh children-nya terender
// untuk test ini — perilaku fokus asli divalidasi manual, bukan di sini.
vi.mock('focus-trap-react', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

const api = (await import('../api/client')).api;

// renderHalaman memuat rules ke mock sebelum render, karena loadRules dipanggil
// sekali di useEffect. Badge mode di kartu jadi sinyal "data sudah siap".
const renderHalaman = async (rules: RoutingRule[]) => {
  vi.mocked(api.routing.list).mockResolvedValue({ items: rules });
  render(<RoutingRules />);
  await waitFor(() =>
    expect(screen.getAllByText(/COMBO PIPELINE|MODEL ONLY|MULTI-PROVIDER/i).length).toBeGreaterThan(0),
  );
};

const bukaDrawerBuat = () => {
  // Saat daftar kosong, tombol "Tambah Aturan Pertama" juga cocok dengan pola ini.
  // Header selalu dirender lebih dulu, jadi indeks 0 tepat.
  fireEvent.click(screen.getAllByRole('button', { name: /Tambah Aturan/i })[0]);
};

const bukaDrawerEdit = () => {
  fireEvent.click(screen.getByRole('button', { name: /Edit Aturan/i }));
};

const pilihModeCombo = async () => {
  await waitFor(() => screen.getByRole('button', { name: /Combo Cascade/i }));
  fireEvent.click(screen.getByRole('button', { name: /Combo Cascade/i }));
  await waitFor(() => expect(screen.getByTestId('combo-builder')).toBeInTheDocument());
};

const pilihModel = async (labelSelect: string, namaOpsi: string) => {
  fireEvent.click(screen.getByLabelText(labelSelect));
  await waitFor(() => screen.getByRole('option', { name: new RegExp(namaOpsi) }));
  fireEvent.click(screen.getByRole('option', { name: new RegExp(namaOpsi) }));
};

describe('RoutingRules — deteksi mode (perbaikan K2)', () => {
  beforeEach(() => {
    payloadStore.create = null;
    payloadStore.update = null;
    vi.clearAllMocks();
  });

  it('rule combo jsonb dengan max_attempts 1 tetap terbaca COMBO, bukan model_only', async () => {
    // Bug K2: max_attempts===1 dulu dipakai menentukan mode, sehingga rule combo
    // dengan anggaran 1 salah dibaca model_only dan simpan berikutnya menghapus
    // provider/weights. Sumber kebenaran sekarang adalah kolom pipeline.
    const rule: RoutingRule = {
      id: 'r-combo-1',
      name: 'murah-cerdas',
      description: 'Combo Prioritas: GPT-5 -> Gemini 2.5 Flash',
      priority: 50,
      match_model_id: MODEL_GPT5.id,
      match_capabilities: [],
      strategy: 'priority',
      max_attempts: 1,
      backoff_ms: 200,
      failure_threshold: 5,
      open_duration_ms: 30000,
      half_open_probes: 2,
      enabled: true,
      created_at: '',
      updated_at: '',
      pipeline: { strategy: 'priority', attempts: 1, models: ['gpt-5', 'gemini-2.5-flash'] },
      virtual_alias: 'murah-cerdas',
    };

    await renderHalaman([rule]);

    expect(screen.getByText('COMBO PIPELINE')).toBeInTheDocument();
    // Alias tampil di kartu dalam elemen <code>, terpisah dari nama aturan (h4).
    expect(screen.getByText('murah-cerdas', { selector: 'code' })).toBeInTheDocument();
    expect(screen.queryByText('MODEL ONLY')).not.toBeInTheDocument();
  });

  it('rule produksi lama berbasis tag tetap terbaca COMBO dengan alias dari tag', async () => {
    // Aturan raute-x di produksi: alias + resep combo ada di description, kolom
    // jsonb belum terisi. Kompatibilitas ini wajib berjalan sampai dikonversi.
    const rule: RoutingRule = {
      id: 'r-raute-x',
      name: 'raute-x',
      description:
        '[combo:alias=raute-x] [combo:tier2=gemini-2.5-flash] [combo:pipeline=[{"tier":2,"model":"gemini-2.5-flash","providers":["prov-a"]}] Smart Tiered Cascade',
      priority: 60,
      match_model_id: MODEL_GPT5.id,
      match_capabilities: [],
      strategy: 'priority',
      max_attempts: 4,
      backoff_ms: 200,
      failure_threshold: 5,
      open_duration_ms: 30000,
      half_open_probes: 2,
      enabled: true,
      created_at: '',
      updated_at: '',
    };

    await renderHalaman([rule]);

    expect(screen.getByText('COMBO PIPELINE')).toBeInTheDocument();
    expect(screen.getByText('raute-x', { selector: 'code' })).toBeInTheDocument();
    // Rantai model di kartu membaca pipeline (jsonb) lalu jatuh ke tag: kedua
    // model harus tampil, bukan hanya match_model_id.
    expect(screen.getByText('T1: GPT-5')).toBeInTheDocument();
    expect(screen.getByText('T2: Gemini 2.5 Flash')).toBeInTheDocument();
  });
});

describe('RoutingRules — form combo (create)', () => {
  beforeEach(() => {
    payloadStore.create = null;
    payloadStore.update = null;
    vi.clearAllMocks();
  });

  it('menyimpan resep ke pipeline + virtual_alias tanpa menulis tag description', async () => {
    await renderHalaman([]);
    bukaDrawerBuat();
    await pilihModeCombo();

    // Isi dua model. Slot pertama belum ada sebelum "Tambah Model" diklik karena
    // resep baru dimulai kosong (pipelineKosong).
    fireEvent.click(screen.getByLabelText('Tambah Model'));
    await pilihModel('Model urutan 1', 'GPT-5');
    fireEvent.click(screen.getByLabelText('Tambah Model'));
    await pilihModel('Model urutan 2', 'Gemini 2.5 Flash');

    // Alias.
    fireEvent.change(screen.getByLabelText('Virtual Endpoint (Alias)'), {
      target: { value: 'hemat-cerdas' },
    });

    fireEvent.click(screen.getByRole('button', { name: /Simpan Aturan/i }));
    await waitFor(() => expect(payloadStore.create).not.toBeNull());

    const payload = payloadStore.create as Record<string, unknown>;
    const pipeline = payload.pipeline as { strategy: string; attempts: number; models: string[] };

    expect(pipeline.models).toEqual(['gpt-5', 'gemini-2.5-flash']);
    expect(pipeline.strategy).toBe('priority');
    expect(payload.virtual_alias).toBe('hemat-cerdas');
    expect(payload.match_model_id).toBe(MODEL_GPT5.id);
    // Anggaran dibawa dari resep, bukan (tier+1)*2 seperti rumus lama.
    expect(payload.max_attempts).toBe(pipeline.attempts);

    // Deskripsi tidak boleh membawa tag [combo:...] — jalur baru menyimpan resep
    // di kolom jsonb, bukan lagi sebagai tag.
    const deskripsi = String(payload.description ?? '');
    expect(deskripsi).not.toMatch(/\[combo:/);
    expect(deskripsi).toContain('Combo');

    // Alias didaftarkan sebagai kolom aturan, bukan panggilan terpisah ke model.
    expect(api.models.addAlias).not.toHaveBeenCalled();
  });

  it('menolak menyimpan combo tanpa model', async () => {
    await renderHalaman([]);
    bukaDrawerBuat();
    await pilihModeCombo();

    // Tidak menambah model apa pun — validasi wajib menahan submit.
    fireEvent.click(screen.getByRole('button', { name: /Simpan Aturan/i }));

    await waitFor(() => expect(payloadStore.create).toBeNull());
  });
});

describe('RoutingRules — form combo (edit, perbaikan K2)', () => {
  beforeEach(() => {
    payloadStore.create = null;
    payloadStore.update = null;
    vi.clearAllMocks();
  });

  it('mempertahankan provider_ids/weights yang tidak diedit saat menyimpan combo', async () => {
    // Inti perbaikan K2: handler lama menimpa seluruh payload per mode. Operator
    // yang hanya mengganti nama aturan combo kehilangan provider_ids. Payload
    // sekarang dimulai dari keadaan tersimpan, lalu hanya menimpa field yang
    // dikelola mode combo.
    const rule: RoutingRule = {
      id: 'r-combo-edit',
      name: 'murah-cerdas',
      description: 'Combo Prioritas: GPT-5 -> Gemini 2.5 Flash',
      priority: 50,
      match_model_id: MODEL_GPT5.id,
      match_capabilities: [],
      strategy: 'priority',
      max_attempts: 3,
      backoff_ms: 200,
      failure_threshold: 5,
      open_duration_ms: 30000,
      half_open_probes: 2,
      enabled: true,
      created_at: '',
      updated_at: '',
      providers: [{ provider_id: 'prov-a', position: 1, weight: 7 }],
      pipeline: { strategy: 'priority', attempts: 3, models: ['gpt-5', 'gemini-2.5-flash'] },
      virtual_alias: 'murah-cerdas',
    };

    await renderHalaman([rule]);
    bukaDrawerEdit();
    await waitFor(() => expect(screen.getByTestId('combo-builder')).toBeInTheDocument());

    // Simpan langsung tanpa mengubah apa pun — cukup untuk memverifikasi bahwa
    // payload dimulai dari keadaan tersimpan.
    fireEvent.click(screen.getByRole('button', { name: /Simpan Perubahan/i }));

    await waitFor(() => expect(payloadStore.update).not.toBeNull());
    const payload = payloadStore.update as Record<string, unknown>;

    // provider_ids & weights dari aturan tersimpan tetap utuh.
    expect(payload.provider_ids).toEqual(['prov-a']);
    expect(payload.weights).toEqual({ 'prov-a': 7 });
    // Resep & alias dikirim ulang utuh.
    expect((payload.pipeline as { models: string[] }).models).toEqual([
      'gpt-5',
      'gemini-2.5-flash',
    ]);
    expect(payload.virtual_alias).toBe('murah-cerdas');
  });

  it('mengonversi aturan berbasis tag ke kolom jsonb saat disimpan', async () => {
    // Edit rule produksi lama (tag) lalu simpan: resep pindah ke pipeline jsonb,
    // alias pindah ke virtual_alias, dan description dibersihkan dari tag.
    const rule: RoutingRule = {
      id: 'r-raute-x-edit',
      name: 'raute-x',
      description:
        '[combo:alias=raute-x] [combo:tier2=gemini-2.5-flash] [combo:pipeline=[{"tier":2,"model":"gemini-2.5-flash","providers":["prov-a"]}] Smart Tiered Cascade',
      priority: 60,
      match_model_id: MODEL_GPT5.id,
      match_capabilities: [],
      strategy: 'priority',
      max_attempts: 4,
      backoff_ms: 200,
      failure_threshold: 5,
      open_duration_ms: 30000,
      half_open_probes: 2,
      enabled: true,
      created_at: '',
      updated_at: '',
    };

    await renderHalaman([rule]);
    bukaDrawerEdit();
    await waitFor(() => expect(screen.getByTestId('combo-builder')).toBeInTheDocument());

    // ComboBuilder memuat kedua model dari tag (Tier 1 = match_model_id).
    expect(screen.getByLabelText('Model urutan 1')).toHaveTextContent('GPT-5');
    expect(screen.getByLabelText('Model urutan 2')).toHaveTextContent('Gemini 2.5 Flash');
    expect(screen.getByLabelText('Virtual Endpoint (Alias)')).toHaveValue('raute-x');

    fireEvent.click(screen.getByRole('button', { name: /Simpan Perubahan/i }));
    await waitFor(() => expect(payloadStore.update).not.toBeNull());
    const payload = payloadStore.update as Record<string, unknown>;

    expect((payload.pipeline as { models: string[] }).models).toEqual([
      'gpt-5',
      'gemini-2.5-flash',
    ]);
    expect(payload.virtual_alias).toBe('raute-x');
    expect(String(payload.description ?? '')).not.toMatch(/\[combo:/);
  });

  it('membersihkan pipeline & alias saat beralih dari combo ke model_only', async () => {
    // Backend menolak alias tanpa pipeline (alias yatim) dengan 400. Beralih ke
    // model_only wajib mengosongkan keduanya sekaligus, bukan hanya pipeline.
    const rule: RoutingRule = {
      id: 'r-combo-ke-mo',
      name: 'murah-cerdas',
      description: 'Combo Prioritas: GPT-5 -> Gemini 2.5 Flash',
      priority: 50,
      match_model_id: MODEL_GPT5.id,
      match_capabilities: [],
      strategy: 'priority',
      max_attempts: 3,
      backoff_ms: 200,
      failure_threshold: 5,
      open_duration_ms: 30000,
      half_open_probes: 2,
      enabled: true,
      created_at: '',
      updated_at: '',
      pipeline: { strategy: 'priority', attempts: 3, models: ['gpt-5', 'gemini-2.5-flash'] },
      virtual_alias: 'murah-cerdas',
    };

    await renderHalaman([rule]);
    bukaDrawerEdit();
    await waitFor(() => screen.getByRole('button', { name: /Model Only/i }));

    // Tombol toggle mode di drawer edit (bukan badge kartu, yang bukan <button>).
    fireEvent.click(screen.getByRole('button', { name: /Model Only/i }));
    fireEvent.click(screen.getByRole('button', { name: /Simpan Perubahan/i }));

    await waitFor(() => expect(payloadStore.update).not.toBeNull());
    const payload = payloadStore.update as Record<string, unknown>;

    expect(payload.pipeline).toBeNull();
    expect(payload.virtual_alias).toBe('');
  });
});
