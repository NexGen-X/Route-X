import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { Login } from './Login';
import { api, ApiError } from '../api/client';

const mockLogin = vi.fn();

vi.mock('../context/AuthContext', () => ({
  useAuth: () => ({
    login: mockLogin,
    principal: null,
    user: null,
    isLoading: false,
    logout: vi.fn(),
  }),
}));

vi.mock('../api/client', () => {
  class MockApiError extends Error {
    constructor(
      public status: number,
      public code: string,
      message: string,
      public param?: string
    ) {
      super(message);
      this.name = 'ApiError';
    }
  }

  return {
    api: {
      auth: {
        setupHint: vi.fn(),
      },
    },
    ApiError: MockApiError,
  };
});

describe('Login Page — Futuristic AI Gateway Portal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (api.auth.setupHint as ReturnType<typeof vi.fn>).mockResolvedValue({
      has_default_admin: false,
      default_email: null,
    });
  });

  it('merender judul Route-X Gateway, logo vektor, dan input form kredensial', async () => {
    render(<Login />);

    expect(screen.getByRole('heading', { name: /route-x/i })).toBeInTheDocument();
    expect(screen.getByText(/gateway/i)).toBeInTheDocument();
    expect(screen.getByLabelText('Alamat Email')).toBeInTheDocument();
    expect(screen.getByLabelText('Kata Sandi')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /masuk ke konsol/i })).toBeInTheDocument();
    expect(screen.getByText(/argon2id enforced/i)).toBeInTheDocument();
  });

  it('menampilkan kartu setup awal ketika terdapat default admin dan tombol salin mengisi form', async () => {
    (api.auth.setupHint as ReturnType<typeof vi.fn>).mockResolvedValue({
      has_default_admin: true,
      default_email: 'admin@routex.local',
    });

    render(<Login />);

    await waitFor(() => {
      expect(screen.getByText('Setup Awal')).toBeInTheDocument();
    });

    const emailInput = screen.getByLabelText('Alamat Email') as HTMLInputElement;
    expect(emailInput.value).toBe('');

    const copyBtn = screen.getByRole('button', { name: /salin dan isi email otomatis/i });
    fireEvent.click(copyBtn);

    expect(emailInput.value).toBe('admin@routex.local');
  });

  it('dapat mengubah visibilitas password saat tombol eye diklik', () => {
    render(<Login />);

    const passwordInput = screen.getByPlaceholderText('••••••••••••') as HTMLInputElement;
    expect(passwordInput.type).toBe('password');

    const toggleBtn = screen.getByLabelText(/lihat kata sandi/i);
    fireEvent.click(toggleBtn);

    expect(passwordInput.type).toBe('text');
  });

  it('memanggil login handler dengan email dan password yang dimasukkan', async () => {
    mockLogin.mockResolvedValueOnce(undefined);
    render(<Login />);

    const emailInput = screen.getByLabelText('Alamat Email');
    const passwordInput = screen.getByPlaceholderText('••••••••••••');
    const submitBtn = screen.getByRole('button', { name: /masuk ke konsol/i });

    fireEvent.change(emailInput, { target: { value: 'operator@routex.io' } });
    fireEvent.change(passwordInput, { target: { value: 'Sup3rS3cr3t!' } });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(mockLogin).toHaveBeenCalledWith('operator@routex.io', 'Sup3rS3cr3t!', false);
    });
  });

  it('menampilkan pesan error jika otentikasi gagal', async () => {
    mockLogin.mockRejectedValueOnce(new ApiError(401, 'INVALID_CREDENTIALS', 'Kredensial tidak valid'));
    render(<Login />);

    const emailInput = screen.getByLabelText('Alamat Email');
    const passwordInput = screen.getByPlaceholderText('••••••••••••');
    const submitBtn = screen.getByRole('button', { name: /masuk ke konsol/i });

    fireEvent.change(emailInput, { target: { value: 'wrong@routex.io' } });
    fireEvent.change(passwordInput, { target: { value: 'wrongpass' } });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent('Kredensial tidak valid');
    });
  });
});
