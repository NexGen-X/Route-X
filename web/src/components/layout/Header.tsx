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
    <header className="sticky top-0 z-30 h-16 border-b border-border/60 bg-bg-base/80 backdrop-blur-md px-4 sm:px-6 flex items-center justify-between transition-all">
      {/* Kiri: Mobile Hamburger + Breadcrumb/Title */}
      <div className="flex items-center gap-3 sm:gap-4 min-w-0">
        <button
          type="button"
          onClick={onOpenMobileMenu}
          className="lg:hidden min-w-[44px] min-h-[44px] flex items-center justify-center p-2 text-text-secondary hover:text-white rounded-xl hover:bg-bg-surface-2 shrink-0 cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
          aria-label="Buka menu navigasi"
        >
          <Menu className="w-5 h-5" aria-hidden="true" />
        </button>

        <div className="min-w-0 flex flex-col justify-center">
          <div className="flex items-center gap-2">
            <h1 className="text-sm sm:text-base font-semibold text-text-primary tracking-tight truncate">
              {title}
            </h1>
            {subtitle && (
              <span className="hidden md:inline-block text-xs text-text-muted truncate">
                · {subtitle}
              </span>
            )}
          </div>
        </div>
      </div>

      {/* Tengah / Kanan: Telemetry Pill & Quick Actions */}
      <div className="flex items-center gap-2.5 sm:gap-3 shrink-0">
        {/* Live gateway telemetry status pill */}
        <div
          role="status"
          aria-label="Status Telemetri Gateway"
          className="hidden md:inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-bg-surface-2/70 border border-border/60 text-xs font-mono shadow-sm"
        >
          <span className="relative flex h-2 w-2">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
            <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500" />
          </span>
          <span className="text-emerald-400 font-medium">Gateway Online</span>
          <span className="text-border" aria-hidden="true">•</span>
          <span className="text-text-muted">v1.4.0</span>
          <span className="text-border" aria-hidden="true">•</span>
          <span className="text-text-secondary font-medium">Active</span>
        </div>

        {/* Search button bergaya Spotlight dengan keycap pill (⌘K) */}
        {onOpenCommandPalette && (
          <>
            <Tooltip content="Cari Cepat (⌘K)" position="bottom">
              <button
                type="button"
                onClick={onOpenCommandPalette}
                className="sm:hidden min-w-[44px] min-h-[44px] flex items-center justify-center p-2.5 rounded-xl bg-bg-surface-2/70 hover:bg-bg-surface-3 border border-border/70 text-text-secondary hover:text-white transition-all shadow-sm cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
                aria-label="Buka Command Palette"
              >
                <Search className="w-4 h-4 text-text-muted" aria-hidden="true" />
              </button>
            </Tooltip>
            <button
              type="button"
              onClick={onOpenCommandPalette}
              className="hidden sm:inline-flex items-center gap-2.5 px-3 py-1.5 rounded-xl bg-bg-surface-2/70 hover:bg-bg-surface-3 hover:border-accent/40 border border-border/70 text-xs text-text-secondary hover:text-white transition-all shadow-sm group cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
              aria-label="Buka Command Palette (Ctrl+K atau ⌘K)"
            >
              <Search
                className="w-3.5 h-3.5 text-text-muted group-hover:text-accent transition-colors"
                aria-hidden="true"
              />
              <span className="text-text-muted group-hover:text-text-secondary text-xs">Cari cepat...</span>
              <kbd
                className="text-[10px] font-mono px-1.5 py-0.5 rounded-md bg-bg-base border border-border text-text-muted group-hover:border-accent/30 group-hover:text-text-secondary transition-colors"
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
          className="hidden xl:inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-bg-surface-2/40 hover:bg-bg-surface-2 border border-border/60 hover:border-border text-xs font-mono text-text-muted hover:text-white transition-colors"
          aria-label="Dokumentasi API Gateway (buka tab baru)"
        >
          <BookOpen className="w-3.5 h-3.5" aria-hidden="true" />
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
