import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Layers } from 'lucide-react';
import { EmptyState } from './EmptyState';

describe('EmptyState (Fase 3 P5)', () => {
  it('merender judul, deskripsi, dan aksi yang bisa diklik', () => {
    const onKlik = vi.fn();
    render(
      <EmptyState
        icon={<Layers className="w-5 h-5" />}
        title="Belum Ada Data"
        description="Deskripsi keadaan kosong."
        action={<button onClick={onKlik}>Tambah Pertama</button>}
      />,
    );
    expect(screen.getByText('Belum Ada Data')).toBeInTheDocument();
    expect(screen.getByText('Deskripsi keadaan kosong.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Tambah Pertama' }));
    expect(onKlik).toHaveBeenCalledTimes(1);
  });

  it('tetap utuh tanpa deskripsi dan aksi', () => {
    render(<EmptyState icon={<Layers className="w-5 h-5" />} title="Kosong Saja" />);
    expect(screen.getByText('Kosong Saja')).toBeInTheDocument();
    expect(screen.queryByRole('button')).toBeNull();
  });
});
