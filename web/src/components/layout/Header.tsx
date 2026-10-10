import React from 'react';
import { Menu, ShieldCheck, Search, BookOpen } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { Badge } from '../common/Badge';
import { Tooltip } from '../common/Tooltip';

export interface HeaderProps {
  title: string;
  subtitle?: string;
  onOpenMobileMenu: () => void;
  onOpenCommandPalette?: () => void;
}

export const Header: React.FC<HeaderProps> = ({
  title,
  subtitle,
  onOpenMobileMenu,
  onOpenCommandPalette,
}) => {
  const { principal } = useAuth();

  return (
    <header className="sticky top-0 z-30 h-16 border-b border-white/[0.06] bg-bg-base/70 backdrop-blur-xl px-4 sm:px-6 flex items-center justify-between transition-all">
      {/* Kiri: Mobile Hamburger + Breadcrumb/Title yang tajam */}
      <div className="flex items-center gap-3 sm:gap-4 min-w-0">
        <button
          type="button"
          onClick={onOpenMobileMenu}
          className="lg:hidden min-w-[44px] min-h-[44px] flex items-center justify-center p-2 text-text-secondary hover:text-white rounded-xl hover:bg-white/[0.06] shrink-0 cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
          aria-label="Buka menu navigasi"
        >
          <Menu className="w-5 h-5" aria-hidden="true" />
        </button>

        <div className="min-w-0 flex items-center gap-2">
          <h1 className="text-sm sm:text-base font-semibold text-text-primary tracking-tight truncate">
            {title}
          </h1>
          {subtitle && (
            <>
              <span className="text-white/20 text-xs select-none" aria-hidden="true">
                /
              </span>
              <span className="hidden md:inline-block text-xs text-text-muted truncate font-mono">
                {subtitle}
              </span>
            </>
          )}
        </div>
      </div>

      {/* Tengah / Kanan: Live Telemetry Pill & Quick Actions */}
      <div className="flex items-center gap-2.5 sm:gap-3 shrink-0">
        {/* Live gateway telemetry status pill dengan radar berdenyut (pulsing dot) & emerald glow */}
        <div
          role="status"
          aria-label="Status Telemetri Gateway"
          className="hidden md:inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-bg-surface-1/60 border border-white/[0.08] backdrop-blur-md text-xs font-mono shadow-sm hover:border-white/[0.16] transition-colors"
        >
          <span className="relative flex h-2 w-2" aria-hidden="true">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
            <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500 shadow-[0_0_8px_#10B981]" />
          </span>
          <span className="text-emerald-400 font-medium">Gateway Online</span>
          <span className="text-white/20" aria-hidden="true">•</span>
          <span className="text-text-muted">v1.4.0</span>
          <span className="text-white/20" aria-hidden="true">•</span>
          <span className="text-cyan-400 font-medium">Active</span>
        </div>

        {/* Search button bergaya Spotlight dengan border kaca dan keycap badge (⌘K) */}
        {onOpenCommandPalette && (
          <>
            <Tooltip content="Cari Cepat (⌘K)" position="bottom">
              <button
                type="button"
                onClick={onOpenCommandPalette}
                className="sm:hidden min-w-[44px] min-h-[44px] flex items-center justify-center p-2.5 rounded-xl bg-bg-surface-1/60 hover:bg-bg-surface-2 border border-white/[0.08] hover:border-white/[0.16] text-text-secondary hover:text-white transition-all shadow-sm cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
                aria-label="Buka Command Palette"
              >
                <Search className="w-4 h-4 text-text-muted" aria-hidden="true" />
              </button>
            </Tooltip>
            <button
              type="button"
              onClick={onOpenCommandPalette}
              className="hidden sm:inline-flex items-center gap-2.5 px-3 py-1.5 rounded-xl bg-bg-surface-1/60 hover:bg-bg-surface-2 hover:border-accent/40 border border-white/[0.08] text-xs text-text-secondary hover:text-white transition-all shadow-sm group cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
              aria-label="Buka Command Palette (Ctrl+K atau ⌘K)"
            >
              <Search
                className="w-3.5 h-3.5 text-text-muted group-hover:text-cyan-400 transition-colors"
                aria-hidden="true"
              />
              <span className="text-text-muted group-hover:text-text-secondary text-xs">Cari cepat...</span>
              <kbd
                className="text-[10px] font-mono px-1.5 py-0.5 rounded-md bg-white/[0.06] border border-white/[0.1] text-text-muted group-hover:border-cyan-500/30 group-hover:text-text-secondary transition-colors"
                aria-hidden="true"
              >
                ⌘K
              </kbd>
            </button>
          </>
        )}

        {/* Tombol Aksi Cepat: Dokumentasi API */}
        <a
          href="/docs"
          target="_blank"
          rel="noopener noreferrer"
          className="hidden xl:inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-bg-surface-1/40 hover:bg-bg-surface-2 border border-white/[0.06] hover:border-white/[0.12] text-xs font-mono text-text-muted hover:text-white transition-colors"
          aria-label="Dokumentasi API Gateway (buka tab baru)"
        >
          <BookOpen className="w-3.5 h-3.5 text-cyan-400" aria-hidden="true" />
          <span>API Docs</span>
        </a>

        {/* Principal Role Badge */}
        {principal && (
          <Badge variant="lime" size="md">
            <ShieldCheck className="w-3.5 h-3.5 shrink-0" aria-hidden="true" />
            <span className="truncate max-w-[80px] xs:max-w-[120px] sm:max-w-none">
              {principal.roles[0] || 'Admin'}
            </span>
          </Badge>
        )}
      </div>
    </header>
  );
};
