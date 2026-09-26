import React from 'react';
import {
  Cpu,
  Share2,
  ShieldCheck,
  Clock,
  CheckCircle2,
  ArrowDown,
  ArrowUp,
} from 'lucide-react';
import { Badge } from '../common/Badge';
import type { SystemOverview } from '../../types';
import { formatBytes, formatUptime } from './utils';

export interface SystemTelemetrySectionProps {
  overview: SystemOverview | null;
}

export const SystemTelemetrySection: React.FC<SystemTelemetrySectionProps> = ({ overview }) => {
  const hostRAMPct =
    overview?.host_ram_total_bytes && overview.host_ram_total_bytes > 0
      ? Math.min(
          100,
          Math.max(0, ((overview.host_ram_used_bytes || 0) / overview.host_ram_total_bytes) * 100)
        )
      : 0;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2.5">
          <Cpu className="w-4 h-4 text-accent" />
          <h3 className="text-sm font-semibold text-white tracking-tight">
            Telemetri Runtime Gateway
          </h3>
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {/* Card 1: Memory */}
        <div className="bg-bg-surface border border-border rounded-card p-5 flex flex-col justify-between hover:border-border/80 transition-all shadow-sm">
          <div>
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2.5">
                <div className="p-2 rounded-inner bg-bg-surface-2 text-text-secondary border border-border">
                  <Cpu className="w-4 h-4" />
                </div>
                <div>
                  <span className="text-xs font-semibold text-white block">Memori Gateway</span>
                  <span className="text-[10px] text-text-muted font-mono">RSS Process</span>
                </div>
              </div>
              <Badge variant="neutral" className="text-[10px] font-mono">
                {overview?.go_version || '-'}
              </Badge>
            </div>

            <div className="mt-3">
              <div className="text-2xl font-bold text-white font-mono tracking-tight">
                {overview ? formatBytes(overview.process_rss_bytes) : '-'}
              </div>
            </div>

            {/* Progress bar host RAM */}
            <div className="mt-3">
              <div className="flex items-center justify-between text-[10px] text-text-secondary mb-1.5 font-mono">
                <span>{hostRAMPct.toFixed(1)}% host RAM</span>
                <span className="text-text-muted">
                  {overview
                    ? `${formatBytes(overview.host_ram_used_bytes)} / ${formatBytes(overview.host_ram_total_bytes)}`
                    : '-'}
                </span>
              </div>
              <div className="w-full h-1.5 bg-bg-surface-3 rounded-full overflow-hidden">
                <div
                  className="h-full bg-emerald-400 rounded-full transition-all duration-500"
                  style={{ width: `${hostRAMPct}%` }}
                />
              </div>
            </div>
          </div>

          {/* Breakdown rows */}
          <div className="mt-4 pt-3 border-t border-border space-y-1.5 text-xs">
            <div className="flex items-center justify-between">
              <span className="text-text-muted">RAM Kontainer</span>
              <span className="font-mono text-text-secondary">
                {overview ? formatBytes(overview.container_ram_bytes) : '-'}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-text-muted">Heap Go</span>
              <span className="font-mono text-text-secondary">
                {overview ? formatBytes(overview.go_heap_bytes) : '-'}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-text-muted">Siklus GC</span>
              <span className="font-mono text-text-secondary font-medium">
                {overview?.num_gc ?? 0}
              </span>
            </div>
          </div>
        </div>

        {/* Card 2: Network */}
        <div className="bg-bg-surface border border-border rounded-card p-5 flex flex-col justify-between hover:border-border/80 transition-all shadow-sm">
          <div>
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2.5">
                <div className="p-2 rounded-inner bg-bg-surface-2 text-text-secondary border border-border">
                  <Share2 className="w-4 h-4" />
                </div>
                <div>
                  <span className="text-xs font-semibold text-white block">Throughput I/O</span>
                  <span className="text-[10px] text-text-muted font-mono">Total Trafik Jaringan</span>
                </div>
              </div>
              <div className="px-2 py-0.5 rounded-full bg-bg-surface-2 border border-border text-[10px] font-mono text-text-secondary font-medium">
                {(overview?.net_rate_mb_s ?? 0).toFixed(1)} MB/s
              </div>
            </div>

            <div className="mt-3">
              <div className="text-2xl font-bold text-white font-mono tracking-tight">
                {overview ? formatBytes(overview.net_total_bytes) : '-'}
              </div>
              <span className="text-[10px] text-text-muted font-mono block mt-0.5">
                Sejak boot
              </span>
            </div>
          </div>

          {/* Breakdown rows with indicator bars */}
          <div className="mt-4 pt-3 border-t border-border space-y-2.5 text-xs">
            <div>
              <div className="flex items-center justify-between mb-1">
                <span className="text-text-muted flex items-center gap-1">
                  <ArrowDown className="w-3 h-3 text-cyan-400" /> Masuk (Rx)
                </span>
                <span className="font-mono text-text-secondary">
                  {overview
                    ? `${formatBytes(overview.net_recv_bytes)} · ${(overview.net_recv_rate_mb_s ?? 0).toFixed(1)} MB/s`
                    : '-'}
                </span>
              </div>
              <div className="w-full h-1 bg-bg-surface-3 rounded-full overflow-hidden">
                <div
                  className="h-full bg-cyan-400/80 rounded-full transition-all duration-500"
                  style={{
                    width: `${
                      overview?.net_total_bytes && overview.net_total_bytes > 0
                        ? Math.min(
                            100,
                            Math.round(
                              ((overview.net_recv_bytes || 0) / overview.net_total_bytes) * 100
                            )
                          )
                        : 0
                    }%`,
                  }}
                />
              </div>
            </div>

            <div>
              <div className="flex items-center justify-between mb-1">
                <span className="text-text-muted flex items-center gap-1">
                  <ArrowUp className="w-3 h-3 text-emerald-400" /> Keluar (Tx)
                </span>
                <span className="font-mono text-text-secondary">
                  {overview
                    ? `${formatBytes(overview.net_sent_bytes)} · ${(overview.net_sent_rate_mb_s ?? 0).toFixed(1)} MB/s`
                    : '-'}
                </span>
              </div>
              <div className="w-full h-1 bg-bg-surface-3 rounded-full overflow-hidden">
                <div
                  className="h-full bg-emerald-400/80 rounded-full transition-all duration-500"
                  style={{
                    width: `${
                      overview?.net_total_bytes && overview.net_total_bytes > 0
                        ? Math.min(
                            100,
                            Math.round(
                              ((overview.net_sent_bytes || 0) / overview.net_total_bytes) * 100
                            )
                          )
                        : 0
                    }%`,
                  }}
                />
              </div>
            </div>
          </div>
        </div>

        {/* Card 3: Egress Pool */}
        <div className="bg-bg-surface border border-border rounded-card p-5 flex flex-col justify-between hover:border-border/80 transition-all shadow-sm">
          <div>
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2.5">
                <div className="p-2 rounded-inner bg-bg-surface-2 text-text-secondary border border-border">
                  <ShieldCheck className="w-4 h-4" />
                </div>
                <div>
                  <span className="text-xs font-semibold text-white block">
                    Egress &amp; Tunnel Pool
                  </span>
                  <span className="text-[10px] text-text-muted font-mono">Proxies</span>
                </div>
              </div>
              <div className="px-2 py-0.5 rounded-full bg-bg-surface-2 border border-border text-[10px] font-mono text-text-secondary font-medium">
                {overview?.egress_active_mode || 'DIRECT'}
              </div>
            </div>

            <div className="mt-3">
              <div className="text-2xl font-bold text-white font-mono tracking-tight">
                {overview?.egress_total_routes ?? 0}
              </div>
              <span className="text-[10px] text-text-muted font-mono block mt-0.5">
                Jalur keluar terdaftar
              </span>
            </div>
          </div>

          {/* Breakdown rows */}
          <div className="mt-4 pt-3 border-t border-border space-y-1.5 text-xs">
            <div className="flex items-center justify-between">
              <span className="text-text-muted">HTTP/SOCKS</span>
              <span className="font-mono text-text-secondary">
                {overview?.egress_http_count ?? 0}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-text-muted">Status Tunnel</span>
              {(overview?.egress_total_routes ?? 0) > 0 ? (
                <span className="text-emerald-400 font-mono text-[11px] flex items-center gap-1 font-medium">
                  <CheckCircle2 className="w-3 h-3" /> Beroperasi
                </span>
              ) : (
                <span className="text-text-muted font-mono text-[11px] flex items-center gap-1">
                  Direct Routing
                </span>
              )}
            </div>
          </div>
        </div>

        {/* Card 4: Uptime & CPU */}
        <div className="bg-bg-surface border border-border rounded-card p-5 flex flex-col justify-between hover:border-border/80 transition-all shadow-sm">
          <div>
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2.5">
                <div className="p-2 rounded-inner bg-bg-surface-2 text-text-secondary border border-border">
                  <Clock className="w-4 h-4" />
                </div>
                <div>
                  <span className="text-xs font-semibold text-white block">Uptime &amp; Komputasi</span>
                  <span className="text-[10px] text-text-muted font-mono">
                    PID {overview?.pid || '-'}
                  </span>
                </div>
              </div>
              <Badge variant="neutral" className="text-[10px] font-mono">
                {(overview?.container_cpu_cap ?? 1).toFixed(1)} cores
              </Badge>
            </div>

            <div className="mt-3">
              <div className="text-2xl font-bold text-white font-mono tracking-tight">
                {overview ? formatUptime(overview.uptime_seconds) : '-'}
              </div>
            </div>

            {/* Progress bar Proxy CPU */}
            <div className="mt-3">
              <div className="flex items-center justify-between text-[10px] text-text-secondary mb-1.5 font-mono">
                <span>Proxy CPU</span>
                <span className="text-text-primary font-medium">
                  {(overview?.proxy_cpu_pct ?? 0).toFixed(1)}%
                </span>
              </div>
              <div className="w-full h-1.5 bg-bg-surface-3 rounded-full overflow-hidden">
                <div
                  className="h-full bg-amber-400/80 rounded-full transition-all duration-500"
                  style={{ width: `${Math.min(100, overview?.proxy_cpu_pct || 0)}%` }}
                />
              </div>
            </div>
          </div>

          {/* Breakdown rows */}
          <div className="mt-4 pt-3 border-t border-border space-y-1.5 text-xs">
            <div className="flex items-center justify-between">
              <span className="text-text-muted">Host CPU</span>
              <span className="font-mono text-text-secondary">
                {(overview?.host_cpu_pct ?? 0).toFixed(1)}%
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-text-muted">Goroutines</span>
              <span className="font-mono text-text-secondary font-medium">
                {overview?.num_goroutine ?? 0}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
