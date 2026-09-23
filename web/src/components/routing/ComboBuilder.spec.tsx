import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ComboBuilder } from './ComboBuilder';
import {
  pipelineKosong,
  ATTEMPTS_MIN,
  MODELS_MAX,
  type ComboPipeline,
} from '../../lib/rulePipeline';
import type { Model } from '../../types';

const buatModel = (model_id: string, display_name: string): Model => ({
  id: `uuid-${model_id}`,
  model_id,
  display_name,
  capabilities: [],
  enabled: true,
  routing_priority: 0,
  created_at: '',
  updated_at: '',
});

const MODELS = [
  buatModel('gpt-5', 'GPT-5'),
  buatModel('gemini-2-5-pro', 'Gemini 2.5 Pro'),
  buatModel('claude-haiku-4-5', 'Claude Haiku 4.5'),
];

const bukaPilihan = async (label: string) => {
  fireEvent.click(screen.getByLabelText(label));
};

describe('ComboBuilder render dasar', () => {
  it('menampilkan label strategi dan opsi default priority', () => {
    render(
      <ComboBuilder
        value={pipelineKosong()}
        onChange={() => {}}
        alias=""
        onAliasChange={() => {}}
        models={MODELS}
      />,
    );
    expect(screen.getByLabelText('Strategi')).toBeInTheDocument();
    expect(screen.getByText('Urutkan kandidat gabungan per prioritas provider.')).toBeInTheDocument();
  });

  it('menampilkan hint "Belum ada model" untuk resep kosong', () => {
    render(
      <ComboBuilder
        value={pipelineKosong()}
        onChange={() => {}}
        alias=""
        onAliasChange={() => {}}
        models={MODELS}
      />,
    );
    expect(screen.getByText(/Belum ada model/)).toBeInTheDocument();
  });
});

describe('ComboBuilder interaksi', () => {
  it('ganti strategi memanggil onChange dengan strategi baru', async () => {
    const onChange = vi.fn();
    render(
      <ComboBuilder
        value={pipelineKosong()}
        onChange={onChange}
        alias=""
        onAliasChange={() => {}}
        models={MODELS}
      />,
    );
    await bukaPilihan('Strategi');
    fireEvent.click(screen.getByRole('option', { name: /Round Robin/ }));
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ strategy: 'round_robin' }),
    );
  });

  it('tambah model memanggil onChange dengan entri kosong baru', () => {
    const onChange = vi.fn();
    render(
      <ComboBuilder
        value={pipelineKosong()}
        onChange={onChange}
        alias=""
        onAliasChange={() => {}}
        models={MODELS}
      />,
    );
    fireEvent.click(screen.getByLabelText('Tambah Model'));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ models: [''] }));
  });

  it('hapus model memanggil onChange tanpa entri itu', () => {
    const onChange = vi.fn();
    const value: ComboPipeline = { strategy: 'priority', attempts: ATTEMPTS_MIN, models: ['gpt-5'] };
    render(
      <ComboBuilder value={value} onChange={onChange} alias="" onAliasChange={() => {}} models={MODELS} />,
    );
    fireEvent.click(screen.getByLabelText('Hapus model urutan 1'));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ models: [] }));
  });

  it('tombol tambah disabled saat sudah 8 model (MODELS_MAX)', () => {
    const value: ComboPipeline = {
      strategy: 'priority',
      attempts: ATTEMPTS_MIN,
      models: Array.from({ length: MODELS_MAX }, (_, i) => `model-${i}`),
    };
    render(
      <ComboBuilder value={value} onChange={() => {}} alias="" onAliasChange={() => {}} models={MODELS} />,
    );
    expect(screen.getByLabelText('Tambah Model')).toBeDisabled();
  });

  it('model sudah dipilih di entri lain tidak bisa dipilih lagi (onChange tidak dipanggil)', async () => {
    const onChange = vi.fn();
    const value: ComboPipeline = {
      strategy: 'priority',
      attempts: ATTEMPTS_MIN,
      models: ['gpt-5', ''],
    };
    render(
      <ComboBuilder value={value} onChange={onChange} alias="" onAliasChange={() => {}} models={MODELS} />,
    );
    // Entri 2: pilih 'gpt-5' yang sudah dipakai entri 1 — harus disabled.
    await bukaPilihan('Model urutan 2');
    fireEvent.click(screen.getByRole('option', { name: /GPT-5/ }));
    expect(onChange).not.toHaveBeenCalled();
  });

  it('memilih model di entri kosong memanggil onChange', async () => {
    const onChange = vi.fn();
    const value: ComboPipeline = {
      strategy: 'priority',
      attempts: ATTEMPTS_MIN,
      models: ['gpt-5', ''],
    };
    render(
      <ComboBuilder value={value} onChange={onChange} alias="" onAliasChange={() => {}} models={MODELS} />,
    );
    await bukaPilihan('Model urutan 2');
    fireEvent.click(screen.getByRole('option', { name: /Gemini 2.5 Pro/ }));
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ models: ['gpt-5', 'gemini-2-5-pro'] }),
    );
  });

  it('ubah attempts memanggil onChange dengan angka baru', () => {
    const onChange = vi.fn();
    const value: ComboPipeline = { strategy: 'priority', attempts: ATTEMPTS_MIN, models: ['gpt-5'] };
    render(
      <ComboBuilder value={value} onChange={onChange} alias="" onAliasChange={() => {}} models={MODELS} />,
    );
    fireEvent.change(screen.getByLabelText('Anggaran Attempts'), { target: { value: '5' } });
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ attempts: 5 }));
  });

  it('ubah alias memanggil onAliasChange', () => {
    const onAliasChange = vi.fn();
    render(
      <ComboBuilder
        value={pipelineKosong()}
        onChange={() => {}}
        alias=""
        onAliasChange={onAliasChange}
        models={MODELS}
      />,
    );
    fireEvent.change(screen.getByLabelText('Virtual Endpoint (Alias)'), {
      target: { value: 'murah-cerdas' },
    });
    expect(onAliasChange).toHaveBeenCalledWith('murah-cerdas');
  });
});

describe('ComboBuilder validasi', () => {
  it('menampilkan error inline untuk resep tanpa model + alias yatim', () => {
    render(
      <ComboBuilder
        value={pipelineKosong()}
        onChange={() => {}}
        alias="murah-cerdas"
        onAliasChange={() => {}}
        models={MODELS}
      />,
    );
    const alert = screen.getByTestId('combo-builder-errors');
    expect(alert).toBeInTheDocument();
    expect(alert.textContent).toContain('minimal');
  });

  it('tidak menampilkan error untuk resep sah tanpa alias', () => {
    const value: ComboPipeline = { strategy: 'priority', attempts: 3, models: ['gpt-5'] };
    render(
      <ComboBuilder value={value} onChange={() => {}} alias="" onAliasChange={() => {}} models={MODELS} />,
    );
    expect(screen.queryByTestId('combo-builder-errors')).not.toBeInTheDocument();
  });

  it('mengutamakan errors dari prop ketika diberikan induk', () => {
    render(
      <ComboBuilder
        value={{ strategy: 'priority', attempts: 3, models: ['gpt-5'] }}
        onChange={() => {}}
        alias=""
        onAliasChange={() => {}}
        models={MODELS}
        errors={['Error final dari induk']}
      />,
    );
    const alert = screen.getByTestId('combo-builder-errors');
    expect(alert.textContent).toContain('Error final dari induk');
  });
});
