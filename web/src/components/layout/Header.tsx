import React from 'react';
import { Menu, ShieldCheck, Search } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { Badge } from '../common/Badge';
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
    <header className="h-24 sm:h-16 pt-[env(safe-area-inset-top)] sm:pt-0 bg-bg-base/60 backdrop-blur-xl px-4 sm:px-6 flex items-center justify-between sticky top-0 z-30 shadow-[0_4px_30px_rgba(0,0,0,0.1)]">
      <div className="flex items-center gap-2.5 sm:gap-4 min-w-0">
        <button
          type="button"
          onClick={onOpenMobileMenu}
          className="lg:hidden p-4 sm:p-3 text-text-secondary hover:text-white rounded-nav hover:bg-bg-surface-2 shrink-0 cursor-pointer"
          aria-label="Buka menu navigasi"
        >
          <Menu className="w-7 h-7" aria-hidden="true" />
        </button>
        <div className="min-w-0">
          <h1 className="text-lg sm:text-base font-bold text-text-primary tracking-tight truncate">
            {title}
          </h1>
        </div>
      </div>

      <div className="flex items-center gap-2 sm:gap-3 shrink-0">
        {onOpenCommandPalette && (
          <>
            <Tooltip content="Cari Cepat (⌘K)" position="bottom">
              <button
                type="button"
                onClick={onOpenCommandPalette}
                className="sm:hidden p-3 rounded-inner bg-bg-surface-2 hover:bg-bg-surface-3 border border-border text-text-secondary hover:text-white transition-all shadow-sm cursor-pointer"
                aria-label="Buka Command Palette"
              >
                <Search className="w-5 h-5 text-text-muted" aria-hidden="true" />
              </button>
            </Tooltip>
            <button
              type="button"
              onClick={onOpenCommandPalette}
              className="hidden sm:inline-flex items-center gap-2.5 px-3 py-1.5 rounded-inner bg-bg-surface-2 hover:bg-bg-surface-3 border border-border text-xs text-text-secondary hover:text-white transition-all shadow-sm group cursor-pointer"
              aria-label="Buka Command Palette (Ctrl+K atau ⌘K)"
            >
              <Search className="w-3.5 h-3.5 text-text-muted group-hover:text-accent transition-colors" aria-hidden="true" />
              <span className="text-text-muted group-hover:text-text-secondary text-xs">Cari cepat...</span>
              <kbd className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-bg-base border border-border text-text-muted" aria-hidden="true">
                ⌘K
              </kbd>
            </button>
          </>
        )}

        {principal && (
          <Badge variant="lime" size="md">
            <ShieldCheck className="w-3.5 h-3.5 shrink-0" aria-hidden="true" />
            <span className="truncate max-w-[80px] xs:max-w-[120px] sm:max-w-none">{principal.roles[0] || 'Admin'}</span>
          </Badge>
        )}
      </div>
    </header>
  );
};
