import React from 'react';
import { Menu, ShieldCheck, Search } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { Tooltip } from '../common/Tooltip';

interface HeaderProps {
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
    <header className="h-14 bg-bg-base/75 backdrop-blur-md border-b border-white/[0.06] px-4 sm:px-6 flex items-center justify-between sticky top-0 z-30">
      <div className="flex items-center gap-2.5 sm:gap-3.5 min-w-0">
        <button
          type="button"
          onClick={onOpenMobileMenu}
          className="lg:hidden p-1.5 text-zinc-400 hover:text-white rounded-lg hover:bg-white/[0.04] shrink-0 cursor-pointer transition-colors"
          aria-label="Buka menu navigasi"
        >
          <Menu className="w-5 h-5" aria-hidden="true" />
        </button>
        <div className="min-w-0">
          <h1 className="text-sm font-medium text-white tracking-tight truncate">
            {title}
          </h1>
        </div>
      </div>

      <div className="flex items-center gap-2 sm:gap-2.5 shrink-0">
        {onOpenCommandPalette && (
          <>
            <Tooltip content="Cari Cepat (⌘K)" position="bottom">
              <button
                type="button"
                onClick={onOpenCommandPalette}
                className="sm:hidden p-1.5 rounded-lg bg-white/[0.03] hover:bg-white/[0.06] border border-white/[0.08] text-text-muted hover:text-white transition-all shadow-sm cursor-pointer"
                aria-label="Buka Command Palette"
              >
                <Search className="w-4 h-4 text-text-muted" aria-hidden="true" />
              </button>
            </Tooltip>
            <button
              type="button"
              onClick={onOpenCommandPalette}
              className="hidden sm:inline-flex items-center gap-2 px-2.5 py-1.5 rounded-lg bg-white/[0.03] hover:bg-white/[0.06] border border-white/[0.08] text-xs text-text-muted hover:text-white transition-all shadow-sm group cursor-pointer"
              aria-label="Buka Command Palette (Ctrl+K atau ⌘K)"
            >
              <Search className="w-3.5 h-3.5 text-zinc-500 group-hover:text-accent transition-colors" aria-hidden="true" />
              <span className="text-zinc-400 group-hover:text-zinc-200 text-xs">Cari cepat...</span>
              <kbd className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-white/[0.04] border border-white/[0.08] text-zinc-400" aria-hidden="true">
                ⌘K
              </kbd>
            </button>
          </>
        )}

        {principal && (
          <div className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[11px] font-medium bg-white/[0.03] border border-white/[0.08] text-zinc-300">
            <ShieldCheck className="w-3.5 h-3.5 text-accent shrink-0" aria-hidden="true" />
            <span className="truncate max-w-[80px] xs:max-w-[120px] sm:max-w-none font-mono text-[10px] tracking-wide">{principal.roles[0] || 'Admin'}</span>
          </div>
        )}
      </div>
    </header>
  );
};
