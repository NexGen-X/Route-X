import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Providers } from './Providers';
import { ToastProvider } from '../context/ToastContext';
import type { Provider } from '../types';

// B-446: berganti provider di drawer — rincian provider sebelumnya
// (model/kredensial) wajib hilang SEGERA, sebelum muatan provider
// baru tiba; lencana hitung tab pun belum terukur ('—', B-448).

const buatProvider = (id: string, nama: string): Provider => ({
  id,
  name: nama,
  display_name: nama,
  kind: 'openai',
  base_url: 'https://api.example.com/v1',
  enabled: true,
  priority: 100,
  weight: 100,
  timeout_ms: 30000,
  max_retries: 2,
  consecutive_failures: 0,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
});

const PROV_A = buatProvider('prov-a', 'Provider Alpha');
const PROV_B = buatProvider('prov-b', 'Provider Beta');

const mockListAll = vi.fn(async () => ({ items: [PROV_A, PROV_B] as Provider[] }));
const mockCredentialsList = vi.fn();
const mockModelsList = vi.fn();
const mockOAuthSessions = vi.fn(async (_id: string) => ({ items: [] as unknown[] }));

vi.mock('../api/client', () => ({
  api: {
    providers: {
      listAll: () => mockListAll(),
      listOAuthSessions: (id: string) => mockOAuthSessions(id),
      models: (id: string) => mockModelsList(id),
    },
    egress: {
      list: async () => ({ items: [] }),
    },
    credentials: {
      list: (id: string) => mockCredentialsList(id),
    },
  },
}));

const renderHalaman = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <Providers />
      </ToastProvider>
    </QueryClientProvider>
  );
};

describe('Providers — drawer berganti provider (B-446/B-448)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListAll.mockResolvedValue({ items: [PROV_A, PROV_B] });
    mockOAuthSessions.mockResolvedValue({ items: [] });
    mockCredentialsList.mockResolvedValue({ items: [] });
    mockModelsList.mockResolvedValue({ items: [] });
  });

  it('rincian provider A hilang saat drawer B dibuka dengan muatan tertahan', async () => {
    mockModelsList.mockImplementation(async (id: string) => {
      if (id === 'prov-a') {
        return {
          items: [
            {
              id: 'map-a1',
              provider_id: 'prov-a',
              upstream_model_name: 'model-milik-alpha',
              model_id: 'model-1',
            },
          ],
        };
      }
      // Muatan provider B tertahan tidak pernah selesai selama uji.
      return new Promise(() => {});
    });
    renderHalaman();

    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Kelola model dan akun provider Provider Alpha',
      })
    );
    expect(await screen.findByText('model-milik-alpha')).toBeInTheDocument();

    // Tutup drawer A, lalu buka drawer B.
    fireEvent.click(screen.getByRole('button', { name: 'Tutup' }));
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Kelola model dan akun provider Provider Beta',
      })
    );

    // Baris model milik A TIDAK boleh tampil di drawer B, dan
    // ringkasan header belum terukur ('—'), bukan angka milik A.
    expect(await screen.findByText('— Model • — Key')).toBeInTheDocument();
    expect(screen.queryByText('model-milik-alpha')).toBeNull();
    expect(screen.queryByText('1 Model • 0 Key')).toBeNull();
  });
});
