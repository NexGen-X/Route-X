import React from 'react';
import { Menu, Search, BookOpen } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
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
    <header className="sticky top-0 z-30 h-14 border-b border-zinc-800/80 bg-zinc-950/80 backdrop-blur-md px-4 sm:px-6 flex items-center justify-between transition-colors">
      {/* Kiri: Mobile Hamburger + Breadcrumb tipis minimalis */}
      <div className="flex items-center gap-3 min-w-0">
        <button
          type="button"
          onClick={onOpenMobileMenu}
          className="lg:hidden min-w-[36px] min-h-[36px] flex items-center justify-center p-1.5 text-zinc-400 hover:text-white rounded-md hover:bg-zinc-800/60 shrink-0 cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-zinc-400"
          aria-label="Buka menu navigasi"
        >
          <Menu className="w-4 h-4" aria-hidden="true" />
        </button>

        <nav aria-label="Breadcrumb" className="min-w-0 flex items-center gap-2 text-xs">
          <span className="text-zinc-500 font-normal select-none">Route-X</span>
          <span className="text-zinc-600 select-none" aria-hidden="true">
            /
          </span>
          <h1 className="font-medium text-zinc-200 tracking-tight truncate">
            {title}
          </h1>
          {subtitle && (
            <>
              <span className="text-zinc-600 select-none" aria-hidden="true">
                /
              </span>
              <span className="hidden md:inline-block text-zinc-500 truncate font-mono">
                {subtitle}
              </span>
            </>
          )}
        </nav>
      </div>

      {/* Kanan: Minimalist Status Dot + Search Keycap Pill + Actions */}
      <div className="flex items-center gap-2 sm:gap-3 shrink-0">
        {/* Status Gateway: Clean minimal status dot + v1.4.0 */}
        <div
          role="status"
          aria-label="Status Telemetri Gateway"
          className="hidden sm:inline-flex items-center gap-2 px-2.5 py-1 rounded-full bg-zinc-900/60 border border-zinc-800/80 text-xs font-mono"
        >
          <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 shrink-0" aria-hidden="true" />
          <span className="text-emerald-400 font-medium">Gateway Online</span>
          <span className="text-zinc-600 select-none" aria-hidden="true">·</span>
          <span className="text-zinc-400">v1.4.0</span>
        </div>

        {/* Search button: clean minimal keycap pill (⌘K) */}
        {onOpenCommandPalette && (
          <>
            <Tooltip content="Cari Cepat (⌘K)" position="bottom">
              <button
                type="button"
                onClick={onOpenCommandPalette}
                className="sm:hidden min-w-[36px] min-h-[36px] flex items-center justify-center p-2 rounded-md bg-zinc-900/60 hover:bg-zinc-800/80 border border-zinc-800/80 text-zinc-400 hover:text-white transition-colors cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-zinc-400"
                aria-label="Buka Command Palette"
              >
                <Search className="w-3.5 h-3.5" aria-hidden="true" />
              </button>
            </Tooltip>
            <button
              type="button"
              onClick={onOpenCommandPalette}
              className="hidden sm:inline-flex items-center gap-2 px-2.5 py-1 rounded-md bg-zinc-900/60 hover:bg-zinc-800/80 border border-zinc-800/80 hover:border-zinc-700 text-xs text-zinc-400 hover:text-zinc-200 transition-colors group cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-zinc-400"
              aria-label="Buka Command Palette (Ctrl+K atau ⌘K)"
            >
              <Search
                className="w-3.5 h-3.5 text-zinc-500 group-hover:text-zinc-300 transition-colors"
                aria-hidden="true"
              />
              <span className="text-zinc-400 group-hover:text-zinc-200 text-xs">Cari cepat...</span>
              <kbd
                className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-400 border border-zinc-700/60 group-hover:border-zinc-600 group-hover:text-zinc-300 transition-colors"
                aria-hidden="true"
              >
                ⌘K
              </kbd>
            </button>
          </>
        )}

        {/* API Docs link */}
        <a
          href="/docs"
          target="_blank"
          rel="noopener noreferrer"
          className="hidden xl:inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md bg-zinc-900/40 hover:bg-zinc-800/60 border border-zinc-800/60 hover:border-zinc-700 text-xs font-mono text-zinc-400 hover:text-white transition-colors"
          aria-label="Dokumentasi API Gateway (buka tab baru)"
        >
          <BookOpen className="w-3.5 h-3.5 text-zinc-400" aria-hidden="true" />
          <span>API Docs</span>
        </a>

        {/* Principal Role Badge — Clean Zinc Pill */}
        {principal && (
          <div className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-zinc-800/80 text-zinc-300 border border-zinc-700/60">
            <span className="truncate max-w-[80px] xs:max-w-[120px] sm:max-w-none">
              {principal.roles[0] || 'Admin'}
            </span>
          </div>
        )}
      </div>
    </header>
  );
};
