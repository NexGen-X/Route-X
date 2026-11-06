import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { SystemTelemetrySection } from './SystemTelemetrySection';

// Spec B-313: saat overview belum tiba/gagal, sub-angka telemetri tidak
// boleh tampil sebagai nilai terukur karangan (terutama bawaan
// container_cpu_cap ?? 1 → "1.0 cores").
describe('SystemTelemetrySection — overview kosong (B-313)', () => {
  it('overview null menampilkan penanda belum terukur, bukan angka karangan', () => {
    render(<SystemTelemetrySection overview={null} />);

    expect(screen.queryByText('1.0 cores')).not.toBeInTheDocument();
    expect(screen.queryByText('0.0 MB/s')).not.toBeInTheDocument();
    expect(screen.queryByText('DIRECT')).not.toBeInTheDocument();
    expect(screen.queryByText('0.0%')).not.toBeInTheDocument();
    expect(screen.queryByText('Beroperasi')).not.toBeInTheDocument();
    // Penanda jujur yang sama dengan nilai utama komponen ini.
    expect(screen.getAllByText('—').length).toBeGreaterThan(0);
  });
});
