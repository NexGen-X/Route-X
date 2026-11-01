import React from 'react';
import { LayoutDashboard, Terminal, Server, KeyRound, LayoutGrid } from 'lucide-react';

export interface MobileNavProps {
  currentPath: string;
  onNavigate: (path: string) => void;
  onOpenMore: () => void;
}

interface MobileNavItem {
  name: string;
  path: string;
  icon: React.ComponentType<{ className?: string }>;
}

// Navigasi bawah ala aplikasi mobile Muse: 4 tujuan utama + tombol "Lainnya"
// yang membuka drawer berisi seluruh grup navigasi sidebar.
const ITEMS: MobileNavItem[] = [
  { name: 'Dashboard', path: '/', icon: LayoutDashboard },
  { name: 'Permintaan', path: '/requests', icon: Terminal },
  { name: 'Penyedia', path: '/upstreams/providers', icon: Server },
  { name: 'Kunci API', path: '/access/api-keys', icon: KeyRound },
];

export const MobileNav: React.FC<MobileNavProps> = ({ currentPath, onNavigate, onOpenMore }) => {
  return (
    <nav
      aria-label="Navigasi Bawah Mobile"
      className="fixed bottom-0 inset-x-0 z-40 lg:hidden bg-bg-sidebar/95 backdrop-blur-md border-t border-border pb-[env(safe-area-inset-bottom)]"
    >
      <div className="grid grid-cols-5">
        {ITEMS.map((item) => {
          const Icon = item.icon;
          const isActive = currentPath === item.path;
          return (
            <button
              key={item.path}
              type="button"
              onClick={() => onNavigate(item.path)}
              aria-label={item.name}
              aria-current={isActive ? 'page' : undefined}
              className="flex flex-col items-center justify-center gap-1 min-h-[56px] py-1.5 cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-accent/60"
            >
              <span
                className={`inline-flex items-center justify-center px-4 py-1 rounded-full transition-colors ${
                  isActive ? 'bg-bg-surface-2 text-white' : 'text-text-muted'
                }`}
              >
                <Icon className="w-5 h-5" aria-hidden="true" />
              </span>
              <span
                className={`text-[10px] leading-none ${isActive ? 'text-white font-medium' : 'text-text-muted'}`}
              >
                {item.name}
              </span>
            </button>
          );
        })}
        <button
          type="button"
          onClick={onOpenMore}
          aria-label="Lainnya, buka semua menu"
          className="flex flex-col items-center justify-center gap-1 min-h-[56px] py-1.5 cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-accent/60"
        >
          <span className="inline-flex items-center justify-center px-4 py-1 rounded-full text-text-muted">
            <LayoutGrid className="w-5 h-5" aria-hidden="true" />
          </span>
          <span className="text-[10px] leading-none text-text-muted">Lainnya</span>
        </button>
      </div>
    </nav>
  );
};
