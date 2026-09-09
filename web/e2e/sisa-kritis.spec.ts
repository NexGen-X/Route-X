import { test, expect, request as pwRequest } from '@playwright/test';

// Uji sisa kritis Route-X: providers, egress, users, content filters,
// negatif rate-limit dan kuota.
//
// KONTEKS ADMIN BERSAMA: limiter login 20/IP/15 mnt menjegal suite bila
// tiap test login sendiri. File ini membuat SATU APIRequestContext di
// beforeAll, login tepat 1 kali, lalu semua test memakai ctx + header H.
// Ini pola yang sama dengan storage-state di spec UI (auth.setup.ts),
// diterjemahkan ke dunia API-only.
//
// Isolasi: semua nama berprefiks e2e-sisa-, prioritas provider 99 agar
// tidak menang routing, key khusus limit kecil, cleanup di finally agar
// staging steril walau test gagal tengah jalan.
//
// Prasyarat env: BASE_URL, E2E_ADMIN_EMAIL, E2E_ADMIN_PASSWORD.

const ASAL = process.env.BASE_URL || 'http://127.0.0.1:18080';

let ctx: any;
let H: Record<string, string>;

test.beforeAll(async () => {
  const email = process.env.E2E_ADMIN_EMAIL || '';
  const password = process.env.E2E_ADMIN_PASSWORD || '';
  test.skip(!email || !password, 'E2E_ADMIN_EMAIL/PASSWORD belum diset');
  ctx = await pwRequest.newContext({ baseURL: ASAL });
  const login = await ctx.post('/api/auth/login', {
    data: { email, password },
  });
  expect(login.ok()).toBeTruthy();
  const me = await ctx.get('/api/auth/me');
  const tokenCSRF = (await me.json()).csrf_token as string;
  H = { Origin: ASAL, 'X-CSRF-Token': tokenCSRF };
});

test.afterAll(async () => {
  await ctx?.dispose();
});

test('providers: buat, kredensial, attach, test-model, hapus bersih', async () => {
  const nama = 'e2e-sisa-prov-' + Date.now();
  let provId = '';
  let credId = '';
  let mappingId = '';
  try {
    // 1. Buat provider prioritas 99 (tidak menang routing).
    const buat = await ctx.post('/api/admin/upstreams/providers', {
      headers: H,
      data: {
        name: nama,
        kind: 'openai_compatible',
        base_url: 'http://127.0.0.1:19091',
        priority: 99,
      },
    });
    expect(buat.ok()).toBeTruthy();
    provId = (await buat.json()).id;
    expect(provId).toBeTruthy();

    // 2. Kredensial + attach model gpt-5.
    const cred = await ctx.post(
      `/api/admin/upstreams/providers/${provId}/credentials`,
      { headers: H, data: { label: 'sisa-key', api_key: '«redacted:sk-…»' } },
    );
    expect(cred.ok()).toBeTruthy();
    credId = (await cred.json()).id ?? '';

    const attach = await ctx.post(
      `/api/admin/upstreams/providers/${provId}/models`,
      { headers: H, data: { name: 'gpt-5' } },
    );
    expect(attach.ok()).toBeTruthy();
    mappingId = (await attach.json()).id ?? '';

    // 3. test-model langsung ke echo membuktikan konektivitas.
    const uji = await ctx.post(
      `/api/admin/upstreams/providers/${provId}/test-model`,
      { headers: H, data: { model: 'gpt-5' } },
    );
    expect(uji.ok()).toBeTruthy();

    // 4. Provider terdaftar di daftar.
    const daftar = await ctx.get('/api/admin/upstreams/providers');
    expect(await daftar.text()).toContain(nama);
  } finally {
    // Cleanup urut: detach model, hapus kredensial, hapus provider.
    if (mappingId && provId) {
      await ctx.delete(
        `/api/admin/upstreams/providers/${provId}/models/${mappingId}`,
        { headers: H },
      );
    }
    if (credId && provId) {
      await ctx.delete(
        `/api/admin/upstreams/providers/${provId}/credentials/${credId}`,
        { headers: H },
      );
    }
    if (provId) {
      await ctx.delete(`/api/admin/upstreams/providers/${provId}`, {
        headers: H,
      });
    }
  }
  const sesudah = await ctx.get('/api/admin/upstreams/providers');
  expect(await sesudah.text()).not.toContain(nama);
});

test('egress: buat pool, probe gagal terkontrol, hapus', async () => {
  const nama = 'e2e-sisa-egress-' + Date.now();
  let poolId = '';
  try {
    // Proxy ke port mati: probe harus menjawab unhealthy, bukan 500.
    const buat = await ctx.post('/api/admin/upstreams/egress-pools', {
      headers: H,
      data: {
        name: nama,
        kind: 'http',
        proxy_url: 'http://127.0.0.1:19999',
      },
    });
    expect(buat.ok()).toBeTruthy();
    poolId = (await buat.json()).id;
    expect(poolId).toBeTruthy();

    const probe = await ctx.post(
      `/api/admin/upstreams/egress-pools/${poolId}/test`,
      { headers: H, data: {} },
    );
    expect(probe.ok()).toBeTruthy();
    const hasil = await probe.json();
    expect(hasil.status ?? hasil.data?.status).toBe('unhealthy');
  } finally {
    if (poolId) {
      await ctx.delete(`/api/admin/upstreams/egress-pools/${poolId}`, {
        headers: H,
      });
    }
  }
  const sesudah = await ctx.get('/api/admin/upstreams/egress-pools');
  expect(await sesudah.text()).not.toContain(nama);
});

test('users: buat sekali pakai, beri peran, login, update, hapus', async () => {
  const email = `e2e-sisa-user-${Date.now()}@local`;
  const pwBaru = 'SisaUser2026!Kuat#1';
  let userId = '';
  try {
    const buat = await ctx.post('/api/admin/access/users', {
      headers: H,
      data: {
        email,
        name: 'Sisa User',
        password: pwBaru + 'Xx9',
        must_change_password: false,
      },
    });
    expect(buat.ok()).toBeTruthy();
    userId = (await buat.json()).id;
    expect(userId).toBeTruthy();

    // Beri peran Viewer.
    const roles = await ctx.get('/api/admin/access/roles');
    const daftarPeran = ((await roles.json()).items ?? []) as any[];
    const viewer = daftarPeran.find((r) => r.name === 'Viewer');
    expect(viewer?.id).toBeTruthy();
    const grant = await ctx.post(`/api/admin/access/users/${userId}/roles`, {
      headers: H,
      data: { role_id: viewer.id },
    });
    expect(grant.ok()).toBeTruthy();

    // Login sebagai user baru membuktikan kredensial berfungsi.
    // Konteks TERPISAH: login di ctx admin akan menimpa cookie sesi
    // dan semua panggilan sesudahnya kehilangan permission.
    const ctxBaru = await pwRequest.newContext({ baseURL: ASAL });
    const loginBaru = await ctxBaru.post('/api/auth/login', {
      data: { email, password: pwBaru + 'Xx9' },
    });
    expect(loginBaru.ok()).toBeTruthy();
    await ctxBaru.dispose();

    // Update nama lewat admin.
    const ubah = await ctx.put(`/api/admin/access/users/${userId}`, {
      headers: H,
      data: { name: 'Sisa User Baru' },
    });
    expect(ubah.ok()).toBeTruthy();
    const baca = await ctx.get(`/api/admin/access/users/${userId}`);
    expect(await baca.text()).toContain('Sisa User Baru');
  } finally {
    if (userId) {
      await ctx.delete(`/api/admin/access/users/${userId}`, {
        headers: H,
      });
    }
  }
  const sesudah = await ctx.get('/api/admin/access/users');
  expect(await sesudah.text()).not.toContain(email);
});

test('content filter: blokir respons RAHASIA lalu lolos setelah hapus', async () => {
  const nama = 'e2e-sisa-filter-' + Date.now();
  let filterId = '';
  // Key khusus dengan limit besar agar bukan limiter yang menjawab.
  let keyId = '';
  let keyMentah = '';
  // Routing gpt-5 jatuh ke echo-sisa (prioritas 1, jawab "siap") sebelum
  // echo-konten (prioritas 10). Agar respons RAHASIA-E2E dari echo-konten
  // yang diuji filter, echo-sisa dimatikan sementara lalu dinyalakan lagi.
  let echoSisaId = '';
  try {
    const buatKey = await ctx.post('/api/admin/access/api-keys', {
      headers: H,
      data: { name: 'e2e-sisa-filterkey-' + Date.now(), rate_limit_rpm: 6000 },
    });
    expect(buatKey.ok()).toBeTruthy();
    const badanKey = await buatKey.json();
    keyId = badanKey.key?.id ?? badanKey.id;
    keyMentah = badanKey.raw_key ?? badanKey.key?.raw_key ?? '';
    expect(keyId).toBeTruthy();
    expect(keyMentah).toBeTruthy();

    // Filter respons: blokir bila respons mengandung «redacted:sk_live_…».
    const buat = await ctx.post('/api/admin/gateway/content-filters', {
      headers: H,
      data: {
        name: nama,
        kind: 'blocked_pattern',
        priority: 1,
        applies_to: 'response',
        action: 'block',
        pattern: '«redacted:sk_live_…»',
        pattern_type: 'substring',
        max_eval_ms: 1000,
      },
    });
    expect(buat.ok()).toBeTruthy();
    filterId = (await buat.json()).id;
    expect(filterId).toBeTruthy();

    // Engine filter memuat salinan aturan tiap 5 detik (TTL): tunggu lewat.
    await new Promise((r) => setTimeout(r, 7000));

    // Matikan echo-sisa agar routing jatuh ke echo-konten.
    const daftarProv = await ctx.get('/api/admin/upstreams/providers');
    const provs = ((await daftarProv.json()).items ?? []) as any[];
    const echoSisa = provs.find((p) => p.name === 'echo-sisa');
    expect(echoSisa?.id).toBeTruthy();
    echoSisaId = echoSisa.id;
    const mati = await ctx.put(`/api/admin/upstreams/providers/${echoSisaId}`, {
      headers: H,
      data: { enabled: false },
    });
    expect(mati.ok()).toBeTruthy();

    // Prompt unik + cache-buster: tiap chat prompt berbeda agar tidak HIT cache.
    const unik = Date.now();
    const diblokir = await ctx.post('/v1/chat/completions', {
      headers: { Authorization: `Bearer ${keyMentah}` },
      data: {
        model: 'gpt-5',
        messages: [{ role: 'user', content: `minta RAHASIA-E2E ${unik}` }],
      },
    });
    // Kebijakan blokir menolak dengan 4xx, bukan 200.
    expect(diblokir.status()).toBeGreaterThanOrEqual(400);
    expect(diblokir.status()).toBeLessThan(500);
  } finally {
    // Nyalakan lagi echo-sisa agar staging kembali normal.
    if (echoSisaId) {
      await ctx.put(`/api/admin/upstreams/providers/${echoSisaId}`, {
        headers: H,
        data: { enabled: true },
      });
    }
    if (filterId) {
      await ctx.delete(`/api/admin/gateway/content-filters/${filterId}`, {
        headers: H,
      });
    }
    if (keyId) {
      await ctx.post(`/api/admin/access/api-keys/${keyId}/revoke`, {
        headers: H,
      });
    }
  }
  // Setelah filter dihapus + TTL lewat, prompt yang sama lolos 200.
  // Key lama sudah direvoke di finally: pakai key BARU agar yang diuji
  // murni "filter sudah tidak ada", bukan status key.
  await new Promise((r) => setTimeout(r, 7000));
  const buatKey2 = await ctx.post('/api/admin/access/api-keys', {
    headers: H,
    data: { name: 'e2e-sisa-filterkey2-' + Date.now(), rate_limit_rpm: 6000 },
  });
  expect(buatKey2.ok()).toBeTruthy();
  const badan2 = await buatKey2.json();
  const keyId2 = badan2.key?.id ?? badan2.id;
  const keyMentah2 = badan2.raw_key ?? badan2.key?.raw_key ?? '';
  expect(keyMentah2).toBeTruthy();
  try {
    const lolos = await ctx.post('/v1/chat/completions', {
      headers: { Authorization: `Bearer ${keyMentah2}` },
      data: {
        model: 'gpt-5',
        messages: [{ role: 'user', content: `minta RAHASIA-E2E ${Date.now()}` }],
      },
    });
    expect(lolos.status()).toBe(200);
  } finally {
    if (keyId2) {
      await ctx.post(`/api/admin/access/api-keys/${keyId2}/revoke`, {
        headers: H,
      });
    }
  }
});

test('negatif rate-limit: key 2 RPM, request ke-3 ditolak 429', async () => {
  let keyId = '';
  let keyMentah = '';
  try {
    const buatKey = await ctx.post('/api/admin/access/api-keys', {
      headers: H,
      data: { name: 'e2e-sisa-ratelimit-' + Date.now(), rate_limit_rpm: 2 },
    });
    expect(buatKey.ok()).toBeTruthy();
    const badanKey = await buatKey.json();
    keyId = badanKey.key?.id ?? badanKey.id;
    keyMentah = badanKey.raw_key ?? badanKey.key?.raw_key ?? '';
    expect(keyId).toBeTruthy();
    expect(keyMentah).toBeTruthy();

    const kirim = (unik: string) =>
      ctx.post('/v1/chat/completions', {
        headers: { Authorization: `Bearer ${keyMentah}` },
        data: {
          model: 'gpt-5',
          messages: [{ role: 'user', content: `halo limit ${unik}` }],
        },
      });
    const r1 = await kirim(`a${Date.now()}`);
    expect(r1.status()).toBe(200);
    const r2 = await kirim(`b${Date.now()}`);
    expect(r2.status()).toBe(200);
    // Ke-3 dalam semenit: bukti limiter bekerja.
    const r3 = await kirim(`c${Date.now()}`);
    expect(r3.status()).toBe(429);
  } finally {
    if (keyId) {
      await ctx.post(`/api/admin/access/api-keys/${keyId}/revoke`, {
        headers: H,
      });
    }
  }
});

test('negatif kuota: harga mahal + budget 0,01 USD memblokir 402', async () => {
  let keyId = '';
  let keyMentah = '';
  let budgetId = '';
  // Harga default model 0 sehingga budget tak pernah habis: pasang harga
  // mahal sementara pada mapping echo-sisa lalu kembalikan ke 0.
  const daftarProv = await ctx.get('/api/admin/upstreams/providers');
  const provs = ((await daftarProv.json()).items ?? []) as any[];
  const echoSisa = provs.find((p) => p.name === 'echo-sisa');
  expect(echoSisa?.id).toBeTruthy();
  const daftarMap = await ctx.get(
    `/api/admin/upstreams/providers/${echoSisa.id}/models`,
  );
  const maps = ((await daftarMap.json()).items ?? []) as any[];
  const mapSisa = maps.find((m) => m.upstream_model_name === 'gpt-5');
  expect(mapSisa?.id).toBeTruthy();
  // Harga per-mapping provider_model, bukan per-model kanonik: kalau hanya satu
  // yang mahal, routing bisa jatuh ke yang murah dan kuota tak terpicu.
  let mapKonten: any = null;
  {
    const daftarK = await ctx.get('/api/admin/upstreams/providers');
    const provsK = ((await daftarK.json()).items ?? []) as any[];
    const konten = provsK.find((p: any) => p.name === 'echo-konten');
    if (konten?.id) {
      const dm = await ctx.get(`/api/admin/upstreams/providers/${konten.id}/models`);
      const ms = ((await dm.json()).items ?? []) as any[];
      mapKonten = ms.find((m: any) => m.upstream_model_name === 'gpt-5') ?? ms[0] ?? null;
    }
  }
  const setHarga = async (usd: string) => {
    const r1 = await ctx.post(`/api/admin/upstreams/models/mappings/${mapSisa.id}/pricing`, {
      headers: H,
      data: { input_per_1m_usd: usd, output_per_1m_usd: usd, cached_input_per_1m_usd: usd },
    });
    if (!r1.ok()) return r1;
    if (mapKonten?.id && mapKonten.id !== mapSisa.id) {
      return ctx.post(`/api/admin/upstreams/models/mappings/${mapKonten.id}/pricing`, {
        headers: H,
        data: { input_per_1m_usd: usd, output_per_1m_usd: usd, cached_input_per_1m_usd: usd },
      });
    }
    return r1;
  };
  try {
    const harga = await setHarga('1000');
    expect(harga.ok()).toBeTruthy();
    // Cuplikan harga ber-TTL 30 detik (lihat pricing.go): tunggu lewat agar
    // request memakai harga mahal, bukan harga 0 sebelumnya.
    await new Promise((r) => setTimeout(r, 35000));

    const buatKey = await ctx.post('/api/admin/access/api-keys', {
      headers: H,
      data: { name: 'e2e-sisa-kuota-' + Date.now(), rate_limit_rpm: 6000 },
    });
    expect(buatKey.ok()).toBeTruthy();
    const badanKey = await buatKey.json();
    keyId = badanKey.key?.id ?? badanKey.id;
    keyMentah = badanKey.raw_key ?? badanKey.key?.raw_key ?? '';
    expect(keyId).toBeTruthy();
    expect(keyMentah).toBeTruthy();

    // Budget per key 1 sen, aksi block: 1 request @1000 USD/1M token
    // (~12 token = 1,2 sen) langsung melampaui.
    const buat = await ctx.post('/api/admin/gateway/budgets', {
      headers: H,
      data: {
        name: 'e2e-sisa-budget-' + Date.now(),
        scope: 'api_key',
        scope_id: keyId,
        period: 'monthly',
        limit_usd: '0.01',
        alert_threshold_pct: 80,
        action_on_exceed: 'block',
      },
    });
    expect(buat.ok()).toBeTruthy();
    budgetId = (await buat.json()).id;
    await new Promise((r) => setTimeout(r, 7000));

    let terblokir = false;
    // Salinan anggaran ber-TTL 5 detik (lihat billing.go): spend yang baru
    // dicatat belum terlihat Check sampai salinan dimuat ulang. Jeda antar
    // request memastikan tiap percobaan membaca spend terbaru.
    for (let i = 0; i < 4; i++) {
      if (i > 0) {
        await new Promise((r) => setTimeout(r, 6000));
      }
      const r = await ctx.post('/v1/chat/completions', {
        headers: { Authorization: `Bearer ${keyMentah}` },
        data: {
          model: 'gpt-5',
          messages: [{ role: 'user', content: `kuota ${Date.now()}-${i}` }],
        },
      });
      if (r.status() === 429 || r.status() === 402) {
        terblokir = true;
        break;
      }
      expect(r.status()).toBe(200);
    }
    expect(terblokir).toBe(true);
  } finally {
    if (budgetId) {
      await ctx.delete(`/api/admin/gateway/budgets/${budgetId}`, {
        headers: H,
      });
    }
    if (keyId) {
      await ctx.post(`/api/admin/access/api-keys/${keyId}/revoke`, {
        headers: H,
      });
    }
    // Kembalikan harga ke 0 agar tidak mencemari test lain.
    await setHarga('0');
  }
});
