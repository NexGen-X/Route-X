import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { InputBilangan } from './InputBilangan';

// Audit3 V-FF-02: pola parseInt(...) || bawaan menelan 0 dan teks kosong.
// Komponen ini harus: membiarkan kolom dikosongkan, meneruskan 0 yang sah,
// dan menolak teks tak sah TANPA mengubah nilai induk.
describe('InputBilangan', () => {
  it('meneruskan bilangan bulat sah termasuk 0', () => {
    const onChange = vi.fn();
    render(<InputBilangan ariaLabel="Angka" value={100} onChange={onChange} min={0} />);
    const input = screen.getByLabelText('Angka');
    fireEvent.change(input, { target: { value: '0' } });
    expect(onChange).toHaveBeenCalledWith(0);
    fireEvent.change(input, { target: { value: '42' } });
    expect(onChange).toHaveBeenCalledWith(42);
  });

  it('kolom boleh dikosongkan; nilai induk tidak berubah dan galat tampil', () => {
    const onChange = vi.fn();
    render(<InputBilangan ariaLabel="Angka" value={100} onChange={onChange} />);
    const input = screen.getByLabelText('Angka') as HTMLInputElement;
    fireEvent.change(input, { target: { value: '' } });
    expect(input.value).toBe('');
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole('alert')).toHaveTextContent('tidak boleh kosong');
  });

  it('menolak desimal dan nilai di luar rentang tanpa meneruskannya', () => {
    const onChange = vi.fn();
    render(<InputBilangan ariaLabel="Angka" value={3} onChange={onChange} min={1} max={10} />);
    const input = screen.getByLabelText('Angka');
    fireEvent.change(input, { target: { value: '1.5' } });
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole('alert')).toHaveTextContent('bilangan bulat');
    fireEvent.change(input, { target: { value: '11' } });
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole('alert')).toHaveTextContent('maksimal 10');
    fireEvent.change(input, { target: { value: '0' } });
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole('alert')).toHaveTextContent('minimal 1');
  });

  it('mengikuti perubahan nilai eksternal dari induk', () => {
    const onChange = vi.fn();
    const { rerender } = render(<InputBilangan ariaLabel="Angka" value={5} onChange={onChange} />);
    rerender(<InputBilangan ariaLabel="Angka" value={9} onChange={onChange} />);
    expect((screen.getByLabelText('Angka') as HTMLInputElement).value).toBe('9');
  });
});
