import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { CommandPalette } from './CommandPalette';

// B-489: palet perintah disamakan dengan registri CLI hidup —
// entri tool yang sudah dicabut registri (Antigravity CLI, Cursor,
// Continue.dev) tidak lagi ditawarkan, dan memilih tool benar-
// benar memanggil onSelectCLI (dulu App tidak pernah
// meneruskannya, jadi keenam entri CLI mati di tempat).

vi.mock('../../context/ToastContext', () => ({
  useToast: () => ({
    toast: { success: vi.fn(), error: vi.fn(), warn: vi.fn(), info: vi.fn() },
    confirmModal: vi.fn(async () => true),
  }),
}));

vi.mock('../../lib/cliCount', () => ({
  muatJumlahToolCli: async () => 11,
}));

describe('CommandPalette — entri CLI sesuai registri hidup (B-489)', () => {
  it('tidak menawarkan tool yang dicabut registri', () => {
    render(
      <CommandPalette isOpen={true} onClose={vi.fn()} onNavigate={vi.fn()} onSelectCLI={vi.fn()} />
    );
    expect(screen.queryByText(/Antigravity CLI/)).toBeNull();
    expect(screen.queryByText(/Cursor/)).toBeNull();
    expect(screen.queryByText(/Continue\.dev/)).toBeNull();
  });

  it('menawarkan kesebelas tool hidup dan memilihnya memanggil onSelectCLI + navigasi', () => {
    const onNavigate = vi.fn();
    const onSelectCLI = vi.fn();
    const onClose = vi.fn();
    render(
      <CommandPalette
        isOpen={true}
        onClose={onClose}
        onNavigate={onNavigate}
        onSelectCLI={onSelectCLI}
      />
    );
    for (const nama of [
      'Claude Code',
      'OpenCode',
      'GitHub Copilot CLI',
      'Goose',
      'Aider',
      'Qwen Code',
      'Crush',
      'Hermes Agent',
      'Grok Build',
      'Cline CLI',
      'Codex CLI',
    ]) {
      expect(screen.getByText(`Konfigurasi: ${nama}`)).toBeInTheDocument();
    }
    fireEvent.click(screen.getByText('Konfigurasi: Codex CLI'));
    expect(onNavigate).toHaveBeenCalledWith('/cli-integrations');
    expect(onSelectCLI).toHaveBeenCalledWith('codex');
    expect(onClose).toHaveBeenCalled();
  });
});
