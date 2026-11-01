import React, { useEffect, useState } from 'react';
import {
  LayoutDashboard,
  Activity,
  Terminal,
  Server,
  Network,
  GitFork,
  Coins,
  KeyRound,
  Sliders,
  ChevronLeft,
  ChevronRight,
  LogOut,
  Code2,
  Users,
  Webhook,
} from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { Tooltip } from '../common/Tooltip';
import { RouteXLogo } from '../common/RouteXLogo';
import { APP_VERSION } from '../../lib/version';
import { muatJumlahToolCli } from '../../lib/cliCount';

export interface SidebarProps {
  currentPath: string;
  onNavigate: (path: string) => void;
  isMobileOpen: boolean;
  setIsMobileOpen: (open: boolean) => void;
}

interface NavItem {
  name: string;
  path: string;
  icon: React.ComponentType<{ className?: string }>;
  badge?: string;
}

interface NavGroup {
  title: string;
  items: NavItem[];
}

export const Sidebar: React.FC<SidebarProps> = ({
  currentPath,
  onNavigate,
  isMobileOpen,
  setIsMobileOpen,
}) => {
  const [isCollapsed, setIsCollapsed] = useState(false);
  // Badge CLI dihitung dari registri backend (bukan konstanta): memuat
  // sekali per sesi lewat cache bersama di lib/cliCount.
  const [jumlahToolCli, setJumlahToolCli] = useState<number | null>(null);
  useEffect(() => {
    let hidup = true;
    muatJumlahToolCli().then((n) => {
      if (hidup) setJumlahToolCli(n);
    });
    return () => {
      hidup = false;
    };
  }, []);
  const { user, principal, logout } = useAuth();

  const navGroups: NavGroup[] = [
    {
      title: 'Ringkasan & Aktivitas',
      items: [
        { name: 'Dashboard', path: '/', icon: LayoutDashboard },
        { name: 'Permintaan', path: '/requests', icon: Terminal },
        { name: 'Observabilitas', path: '/observability', icon: Activity },
      ],
    },
    {
      title: 'Penyedia AI & Perutean',
      items: [
        { name: 'Penyedia', path: '/upstreams/providers', icon: Server },
        { name: 'Perutean & Failover', path: '/gateway/routing', icon: GitFork },
        { name: 'Integrasi CLI', path: '/cli-integrations', icon: Code2, badge: jumlahToolCli !== null ? String(jumlahToolCli) : undefined },
        { name: 'Pool Egress', path: '/upstreams/egress', icon: Network },
      ],
    },
    {
      title: 'Akses & Anggaran',
      items: [
        { name: 'Kunci API', path: '/access/api-keys', icon: KeyRound },
        { name: 'Pengguna', path: '/access/users', icon: Users },
        { name: 'Anggaran & Batas Laju', path: '/gateway/budgets', icon: Coins },
      ],
    },
    {
      title: 'Sistem & Pengaturan',
      items: [
        { name: 'Webhooks', path: '/system/webhooks', icon: Webhook },
        { name: 'Pengaturan', path: '/system/settings', icon: Sliders },
      ],
    },
  ];

  return (
    <>
      {/* Mobile backdrop */}
      {isMobileOpen && (
        <div
          className="fixed inset-0 bg-black/70 backdrop-blur-sm z-40 lg:hidden"
          onClick={() => setIsMobileOpen(false)}
          aria-hidden="true"
        />
      )}

      <aside
        className={`fixed lg:static top-0 left-0 bottom-0 z-50 flex flex-col bg-bg-sidebar border-r border-border transition-all duration-200 select-none ${
          isCollapsed ? 'w-[64px]' : 'w-[240px]'
        } ${isMobileOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'}`}
        aria-label="Sidebar Navigasi"
      >
        {/* Brand Header */}
        <div className="h-14 flex items-center px-3.5 border-b border-border justify-between">
          <button
            type="button"
            className="group flex items-center gap-2.5 overflow-hidden cursor-pointer text-left focus:outline-none focus-visible:ring-1 focus-visible:ring-accent/50 rounded-md p-1 -m-1 transition-colors"
            onClick={() => onNavigate('/')}
            aria-label="Route-X Beranda"
          >
            <RouteXLogo size={24} glow={false} className="shrink-0" />

            {!isCollapsed && (
              <div className="flex items-center gap-2 truncate">
                <span className="font-semibold tracking-tight text-white text-sm leading-none">
                  Route-X
                </span>
                <span className="text-[10px] font-mono text-accent-text px-1.5 py-0.5 rounded-full bg-accent/10 border border-accent/20">
                  {APP_VERSION}
                </span>
              </div>
            )}
          </button>
        </div>

        {/* Navigation List */}
        <nav aria-label="Navigasi Utama" className="flex-1 overflow-y-auto px-2 py-3 space-y-4 scrollbar-thin">
          {navGroups.map((group, gIdx) => (
            <div key={gIdx} className="space-y-0.5">
              {!isCollapsed && (
                <div className="px-2.5 pb-1 text-[11px] font-medium tracking-wider text-text-muted uppercase">
                  {group.title}
                </div>
              )}
              {isCollapsed && <div className="h-px bg-border my-1.5 mx-1" aria-hidden="true" />}
              {group.items.map((item) => {
                const Icon = item.icon;
                const isActive = currentPath === item.path;
                const buttonContent = (
                  <button
                    type="button"
                    onClick={() => {
                      onNavigate(item.path);
                      setIsMobileOpen(false);
                    }}
                    aria-label={item.name}
                    aria-current={isActive ? 'page' : undefined}
                    className={`w-full min-h-[44px] lg:min-h-[38px] flex items-center gap-2.5 px-2.5 py-1.5 text-xs transition-colors rounded-nav group cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-accent/50 ${
                      isActive
                        ? 'bg-bg-surface-2 text-white font-medium'
                        : 'text-text-secondary hover:text-white hover:bg-bg-surface-1'
                    } ${isCollapsed ? 'justify-center px-0' : ''}`}
                  >
                    <Icon
                      aria-hidden="true"
                      className={`w-4 h-4 flex-shrink-0 transition-colors ${
                        isActive ? 'text-white' : 'text-text-muted group-hover:text-white'
                      }`}
                    />
                    {!isCollapsed && (
                      <div className="flex-1 flex items-center justify-between min-w-0">
                        <span className="truncate">{item.name}</span>
                        {item.badge && (
                          <span className="text-[10px] font-mono px-1.5 py-0.5 rounded-full bg-accent/15 text-accent-text border border-accent/30">
                            {item.badge}
                          </span>
                        )}
                      </div>
                    )}
                  </button>
                );

                return isCollapsed ? (
                  <Tooltip key={item.path} content={item.name} position="right" className="w-full">
                    {buttonContent}
                  </Tooltip>
                ) : (
                  <React.Fragment key={item.path}>
                    {buttonContent}
                  </React.Fragment>
                );
              })}
            </div>
          ))}
        </nav>

        {/* Compact Unified Footer: user info, gateway status, logout, and collapse */}
        <div className="p-2 border-t border-border bg-bg-sidebar flex flex-col gap-1.5">
          {!isCollapsed && user ? (
            <div className="flex items-center justify-between px-2 py-1.5 rounded-md bg-bg-surface-1/60 border border-border text-xs">
              {/* User details */}
              <div className="flex items-center gap-2 min-w-0">
                <div
                  className="w-6 h-6 rounded-full bg-bg-surface-2 text-text-primary flex items-center justify-center font-semibold text-[11px] shrink-0 border border-border"
                  aria-hidden="true"
                >
                  {user.display_name.charAt(0).toUpperCase()}
                </div>
                <div className="flex flex-col min-w-0 leading-tight">
                  <span className="text-xs font-medium text-text-primary truncate">
                    {user.display_name}
                  </span>
                  <span className="text-[10px] text-text-muted truncate font-mono">
                    {principal?.roles[0] || 'User'}
                  </span>
                </div>
              </div>

              {/* Status indicator & Logout */}
              <div className="flex items-center gap-1.5 shrink-0">
                <div className="flex items-center gap-1 text-[10px] font-mono text-emerald-400 mr-1" title="Gateway Live">
                  <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 shrink-0" aria-hidden="true" />
                  <span className="hidden xl:inline">Live</span>
                  <span className="sr-only">Gateway Live</span>
                </div>
                <Tooltip content="Keluar (Logout)" position="top">
                  <button
                    type="button"
                    onClick={logout}
                    aria-label="Keluar dari akun"
                    className="p-1 text-text-secondary hover:text-rose-400 rounded-full hover:bg-bg-surface-2 transition-colors cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-accent/50"
                  >
                    <LogOut className="w-3.5 h-3.5" aria-hidden="true" />
                  </button>
                </Tooltip>
              </div>
            </div>
          ) : isCollapsed && user ? (
            <div className="flex flex-col items-center gap-2 py-1">
              <Tooltip content={`${user.display_name} (${principal?.roles[0] || 'User'})`} position="right">
                <div
                  className="w-7 h-7 rounded-full bg-bg-surface-2 text-text-primary flex items-center justify-center font-semibold text-xs shrink-0 border border-border"
                  aria-hidden="true"
                >
                  {user.display_name.charAt(0).toUpperCase()}
                </div>
              </Tooltip>
              <Tooltip content="Keluar dari akun" position="right">
                <button
                  type="button"
                  onClick={logout}
                  aria-label="Keluar dari akun"
                  className="p-1.5 text-text-secondary hover:text-rose-400 rounded-full hover:bg-bg-surface-2 transition-colors cursor-pointer"
                >
                  <LogOut className="w-3.5 h-3.5" aria-hidden="true" />
                </button>
              </Tooltip>
            </div>
          ) : null}

          {/* Collapser Toggle */}
          <div className="flex items-center justify-end px-1 pt-0.5">
            <Tooltip content={isCollapsed ? 'Perluas Sidebar' : 'Ciutkan Sidebar'} position="right">
              <button
                type="button"
                onClick={() => setIsCollapsed(!isCollapsed)}
                className={`hidden lg:flex min-w-[28px] min-h-[28px] items-center justify-center rounded text-text-muted hover:text-white hover:bg-bg-surface-2 transition-colors cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-accent/50 ${
                  isCollapsed ? 'mx-auto' : 'ml-auto'
                }`}
                aria-label={isCollapsed ? 'Perluas Sidebar' : 'Ciutkan Sidebar'}
              >
                {isCollapsed ? (
                  <ChevronRight className="w-3.5 h-3.5" aria-hidden="true" />
                ) : (
                  <ChevronLeft className="w-3.5 h-3.5" aria-hidden="true" />
                )}
              </button>
            </Tooltip>
          </div>
        </div>
      </aside>
    </>
  );
};
