import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { api, ApiError, setCsrfToken } from './client';

// Spec inti klien API (audit3 grup 13 — B-057): timeout, normalisasi error,
// pengecualian 401 berbasis pathname, pemulihan CSRF, dedup GET, dan
// penanda terpotong fetchAllPages.

function responsJson(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function responsTeks(status: number, body: string, contentType = 'text/html'): Response {
  return new Response(body, { status, headers: { 'Content-Type': contentType } });
}

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
  setCsrfToken('token-awal');
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('normalisasi error (V-FF-27)', () => {
  it('badan error HTML dari proxy menjadi ApiError generik berstatus asli, bukan teks mentah', async () => {
    fetchMock.mockResolvedValue(responsTeks(502, '<html><body>Bad Gateway</body></html>'));
    await expect(api.providers.list()).rejects.toMatchObject({
      status: 502,
      code: 'unknown_error',
      message: 'Terjadi kesalahan sistem.',
    });
  });

  it('envelope error JSON server dipertahankan kode dan pesannya', async () => {
    fetchMock.mockResolvedValue(
      responsJson(400, { error: { code: 'validasi_gagal', message: 'Nama wajib diisi' } })
    );
    await expect(api.providers.list()).rejects.toMatchObject({
      status: 400,
      code: 'validasi_gagal',
      message: 'Nama wajib diisi',
    });
  });

  it('respons sukses berbadan JSON cacat tetap kesalahan, bukan data karangan', async () => {
    fetchMock.mockResolvedValue(responsTeks(200, 'bukan json{', 'application/json'));
    await expect(api.providers.list()).rejects.toMatchObject({
      code: 'invalid_json_response',
    });
  });

  it('respons error berbadan JSON cacat melaporkan status aslinya', async () => {
    fetchMock.mockResolvedValue(responsTeks(500, 'bukan json{', 'application/json'));
    await expect(api.providers.list()).rejects.toMatchObject({
      status: 500,
      code: 'unknown_error',
    });
  });
});

describe('penanganan 401 (V-FF-26)', () => {
  it('401 dari endpoint biasa memancarkan routex:unauthorized', async () => {
    const pendengar = vi.fn();
    window.addEventListener('routex:unauthorized', pendengar);
    fetchMock.mockResolvedValue(responsJson(401, { error: { code: 'unauthorized', message: 'Sesi berakhir' } }));
    await expect(api.providers.list()).rejects.toMatchObject({ status: 401 });
    expect(pendengar).toHaveBeenCalledTimes(1);
    window.removeEventListener('routex:unauthorized', pendengar);
  });

  it('401 dari /api/auth/me sendiri TIDAK memancarkan event (anti loop logout)', async () => {
    const pendengar = vi.fn();
    window.addEventListener('routex:unauthorized', pendengar);
    fetchMock.mockResolvedValue(responsJson(401, { error: { code: 'unauthorized', message: 'Sesi berakhir' } }));
    await expect(api.auth.me()).rejects.toMatchObject({ status: 401 });
    expect(pendengar).not.toHaveBeenCalled();
    window.removeEventListener('routex:unauthorized', pendengar);
  });
});

describe('pemulihan CSRF (V-FF-31)', () => {
  it('403 csrf_invalid menyegarkan token dari /me lalu mengulang request satu kali', async () => {
    fetchMock
      .mockResolvedValueOnce(responsJson(403, { error: { code: 'csrf_invalid', message: 'Token CSRF tidak valid' } }))
      .mockResolvedValueOnce(responsJson(200, { csrf_token: 'token-baru' }))
      .mockResolvedValueOnce(responsJson(200, { id: 'p-1', name: 'penyedia' }));

    const hasil = await api.providers.create({ name: 'penyedia' });
    expect(hasil).toMatchObject({ id: 'p-1' });
    expect(fetchMock).toHaveBeenCalledTimes(3);

    const panggilanUlang = fetchMock.mock.calls[2];
    expect(panggilanUlang[0]).toBe('/api/admin/upstreams/providers');
    const headers = panggilanUlang[1].headers as Headers;
    expect(headers.get('X-CSRF-Token')).toBe('token-baru');
  });

  it('403 csrf_invalid kedua kalinya dilempar sebagai error (tanpa loop)', async () => {
    fetchMock
      .mockResolvedValueOnce(responsJson(403, { error: { code: 'csrf_invalid', message: 'Token CSRF tidak valid' } }))
      .mockResolvedValueOnce(responsJson(200, { csrf_token: 'token-baru' }))
      .mockResolvedValueOnce(responsJson(403, { error: { code: 'csrf_invalid', message: 'Token CSRF tidak valid' } }));

    await expect(api.providers.create({ name: 'penyedia' })).rejects.toMatchObject({
      status: 403,
      code: 'csrf_invalid',
    });
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it('403 berkode lain tidak memicu penyegaran token', async () => {
    fetchMock.mockResolvedValue(
      responsJson(403, { error: { code: 'forbidden', message: 'Tidak berhak' } })
    );
    await expect(api.providers.create({ name: 'penyedia' })).rejects.toMatchObject({
      status: 403,
      code: 'forbidden',
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});

describe('dedup GET (V-FF-32)', () => {
  it('dua GET identik yang bersamaan berbagi satu fetch', async () => {
    let selesaikan: (r: Response) => void = () => {};
    fetchMock.mockImplementation(
      () => new Promise<Response>((resolve) => { selesaikan = resolve; })
    );
    const a = api.providers.list();
    const b = api.providers.list();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    selesaikan(responsJson(200, { items: [] }));
    await expect(a).resolves.toEqual({ items: [] });
    await expect(b).resolves.toEqual({ items: [] });

    // Sesudah selesai, panggilan berikutnya fetch lagi (bukan cache abadi).
    fetchMock.mockResolvedValue(responsJson(200, { items: [] }));
    await api.providers.list();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});

describe('timeout (V-FF-23)', () => {
  it('fetch yang menggantung dibatalkan pada tenggat dan menjadi request_timeout', async () => {
    vi.useFakeTimers();
    fetchMock.mockImplementation((_url: string, init: RequestInit) => {
      return new Promise<Response>((_resolve, reject) => {
        init.signal?.addEventListener('abort', () => {
          reject(new DOMException('The operation was aborted', 'AbortError'));
        });
      });
    });
    const janji = api.providers.list();
    const penolakan = expect(janji).rejects.toMatchObject({
      status: 0,
      code: 'request_timeout',
    });
    await vi.advanceTimersByTimeAsync(30_000);
    await penolakan;
  });
});

describe('fetchAllPages (V-FF-28)', () => {
  it('kursor yang berputar menghentikan penarikan dengan penanda terpotong', async () => {
    const peringatan = vi.spyOn(console, 'warn').mockImplementation(() => {});
    fetchMock.mockImplementation(() =>
      Promise.resolve(responsJson(200, { items: [{ id: 'x' }], next_cursor: 'kursor-sama' }))
    );
    const hasil = await api.providers.listAll();
    expect(hasil.terpotong).toBe(true);
    expect(hasil.items).toHaveLength(2);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(peringatan).toHaveBeenCalled();
    peringatan.mockRestore();
  });

  it('rantai kursor normal selesai tanpa penanda terpotong', async () => {
    fetchMock
      .mockResolvedValueOnce(responsJson(200, { items: [{ id: '1' }], next_cursor: 'c2' }))
      .mockResolvedValueOnce(responsJson(200, { items: [{ id: '2' }] }));
    const hasil = await api.providers.listAll();
    expect(hasil.terpotong).toBeUndefined();
    expect(hasil.items).toHaveLength(2);
  });
});

describe('ApiError tetap terbawa untuk kegagalan jaringan', () => {
  it('fetch menolak menjadi network_error', async () => {
    fetchMock.mockRejectedValue(new TypeError('Failed to fetch'));
    await expect(api.providers.list()).rejects.toBeInstanceOf(ApiError);
    await expect(api.providers.list()).rejects.toMatchObject({ code: 'network_error' });
  });
});
