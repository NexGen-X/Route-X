import React from 'react';
import { Menu, ShieldCheck, Search } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { Badge } from '../common/Badge';

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
    <header className="h-16 border-b border-border bg-bg-base/80 backdrop-blur-md px-6 flex items-center justify-between sticky top-0 z-30">
      <div className="flex items-center gap-4">
        <button
          onClick={onOpenMobileMenu}
          className="lg:hidden p-2 text-text-secondary hover:text-white rounded-nav hover:bg-bg-surface-2"
        >
          <Menu className="w-5 h-5" />
        </button>
        <div>
          <h1 className="text-lg font-bold text-text-primary tracking-tight">{title}</h1>
          {subtitle && <p className="text-xs text-text-secondary hidden sm:block">{subtitle}</p>}
        </div>
      </div>

      <div className="flex items-center gap-3">
        {onOpenCommandPalette && (
          <button
            type="button"
            onClick={onOpenCommandPalette}
            className="hidden sm:inline-flex items-center gap-2.5 px-3 py-1.5 rounded-inner bg-bg-surface-2 hover:bg-bg-surface-3 border border-border text-xs text-text-secondary hover:text-white transition-all shadow-sm group"
            title="Buka Command Palette (Ctrl+K atau ⌘K)"
          >
            <Search className="w-3.5 h-3.5 text-text-muted group-hover:text-accent transition-colors" />
            <span className="text-text-muted group-hover:text-text-secondary text-xs">Cari cepat...</span>
            <kbd className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-bg-base border border-border text-text-muted">
              ⌘K
            </kbd>
          </button>
        )}

        {principal && (
          <Badge variant="lime" size="md">
            <ShieldCheck className="w-3.5 h-3.5" />
            {principal.roles[0] || 'User'}
          </Badge>
        )}
      </div>
    </header>
  );
};
