import React, { useState } from 'react';
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
  HeartPulse,
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
  const { user, principal, logout } = useAuth();

  const navGroups: NavGroup[] = [
    {
      title: 'Ringkasan & Aktivitas',
      items: [
        { name: 'Dashboard', path: '/', icon: LayoutDashboard },
        { name: 'Requests Inspector', path: '/requests', icon: Terminal },
        { name: 'Observability', path: '/observability', icon: Activity },
      ],
    },
    {
      title: 'Penyedia AI & Perutean',
      items: [
        { name: 'Providers', path: '/upstreams/providers', icon: Server },
        { name: 'Routing & Failover', path: '/gateway/routing', icon: GitFork },
        { name: 'CLI Integrations', path: '/cli-integrations', icon: Code2, badge: '20' },
        { name: 'Egress Pools', path: '/upstreams/egress', icon: Network },
      ],
    },
    {
      title: 'Akses & Anggaran',
      items: [
        { name: 'API Keys', path: '/access/api-keys', icon: KeyRound },
        { name: 'Users', path: '/access/users', icon: Users },
        { name: 'Budgets & Limits', path: '/gateway/budgets', icon: Coins },
      ],
    },
    {
      title: 'Sistem & Pengaturan',
      items: [
        { name: 'Webhooks', path: '/system/webhooks', icon: Webhook },
        { name: 'Settings', path: '/system/settings', icon: Sliders },
        { name: 'Diagnostics', path: '/system/diagnostics', icon: HeartPulse },
      ],
    },
  ];

  return (
    <>
      {/* Mobile backdrop */}
      {isMobileOpen && (
        <div
          className="fixed inset-0 bg-black/75 backdrop-blur-sm z-40 lg:hidden"
          onClick={() => setIsMobileOpen(false)}
          aria-hidden="true"
        />
      )}

      <aside
        className={`fixed lg:static top-0 left-0 bottom-0 z-50 flex flex-col bg-bg-sidebar/95 backdrop-blur-xl border-r border-white/[0.06] transition-all duration-300 select-none ${
          isCollapsed ? 'w-[72px]' : 'w-[264px]'
        } ${isMobileOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'}`}
        aria-label="Sidebar Navigasi"
      >
        {/* Brand Header with Modern Route-X Vector Prism */}
        <div className="h-16 flex items-center px-4 border-b border-white/[0.06] justify-between">
          <button
            type="button"
            className="group flex items-center gap-3 overflow-hidden cursor-pointer text-left focus:outline-none focus-visible:ring-2 focus-visible:ring-accent rounded-xl p-1 -m-1 transition-all"
            onClick={() => onNavigate('/')}
            aria-label="Route-X Beranda"
          >
            {/* Logo Route-X Modern Vector */}
            <RouteXLogo size={32} glow={true} className="group-hover:scale-105 transition-transform" />

            {!isCollapsed && (
              <div className="flex flex-col truncate">
                <div className="flex items-center gap-1.5">
                  <span className="font-bold tracking-tight text-white text-base leading-none group-hover:text-cyan-400 transition-colors">
                    Route-X
                  </span>
                  <span className="text-[9px] font-mono font-semibold px-1.5 py-0.5 rounded-full bg-blue-500/10 text-cyan-400 border border-cyan-500/20 shadow-[0_0_8px_rgba(6,182,212,0.15)]">
                    v1.4
                  </span>
                </div>
                <span className="text-[9px] text-text-muted font-mono tracking-widest uppercase mt-0.5">
                  AI GATEWAY &amp; RUNTIME
                </span>
              </div>
            )}
          </button>
        </div>

        {/* Navigation List */}
        <nav aria-label="Navigasi Utama" className="flex-1 overflow-y-auto px-3 py-4 space-y-5 scrollbar-thin">
          {navGroups.map((group, gIdx) => (
            <div key={gIdx} className="space-y-1">
              {!isCollapsed && (
                <div className="px-3 pb-1.5 text-[10px] font-mono font-semibold tracking-wider text-text-muted/70 uppercase">
                  {group.title}
                </div>
              )}
              {isCollapsed && <div className="h-px bg-white/[0.06] my-2 mx-1" aria-hidden="true" />}
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
                    className={`w-full min-h-[44px] lg:min-h-[38px] flex items-center gap-3 px-3 py-2 rounded-xl text-sm transition-all relative group cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-accent ${
                      isActive
                        ? 'bg-gradient-to-r from-blue-500/15 via-blue-500/8 to-transparent text-white font-medium border border-blue-500/25 shadow-sm shadow-blue-500/10'
                        : 'text-text-secondary hover:text-white hover:bg-white/[0.04] border border-transparent'
                    } ${isCollapsed ? 'justify-center px-0' : ''}`}
                  >
                    {/* Floating Active Pill Indicator di sisi kiri */}
                    {isActive && (
                      <span
                        className="absolute left-1 top-1/2 -translate-y-1/2 w-1 h-5 bg-gradient-to-b from-cyan-400 to-blue-500 rounded-full shadow-[0_0_10px_rgba(59,130,246,0.85)]"
                        aria-hidden="true"
                      />
                    )}
                    <Icon
                      aria-hidden="true"
                      className={`w-[18px] h-[18px] flex-shrink-0 transition-colors ${
                        isActive ? 'text-cyan-400' : 'text-text-secondary group-hover:text-text-primary'
                      }`}
                    />
                    {!isCollapsed && (
                      <div className="flex-1 flex items-center justify-between min-w-0">
                        <span className="truncate">{item.name}</span>
                        {item.badge && (
                          <span className="text-[10px] font-mono font-medium px-1.5 py-0.2 rounded-full bg-blue-500/10 text-cyan-400 border border-cyan-500/20">
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

        {/* User profile pill di bagian bawah dengan avatar gradient & tactile logout button */}
        <div className="p-3 border-t border-white/[0.06] bg-bg-surface-1/30 flex flex-col gap-2">
          {!isCollapsed && user && (
            <div className="flex items-center justify-between p-2 rounded-xl bg-bg-surface-1/60 border border-white/[0.06] hover:border-white/[0.12] transition-colors shadow-sm">
              <div className="flex items-center gap-2.5 min-w-0">
                <div
                  className="w-8 h-8 rounded-lg bg-gradient-to-br from-blue-500 via-indigo-600 to-violet-600 text-white flex items-center justify-center font-bold text-xs flex-shrink-0 shadow-sm shadow-indigo-500/25 ring-1 ring-white/15"
                  aria-hidden="true"
                >
                  {user.display_name.charAt(0).toUpperCase()}
                </div>
                <div className="flex flex-col min-w-0">
                  <span className="text-xs font-semibold text-text-primary truncate">
                    {user.display_name}
                  </span>
                  <span className="text-[10px] text-text-muted truncate font-mono">
                    {principal?.roles[0] || 'User'}
                  </span>
                </div>
              </div>
              <Tooltip content="Keluar (Logout)" position="top">
                <button
                  type="button"
                  onClick={logout}
                  aria-label="Keluar dari akun"
                  className="min-w-[44px] min-h-[44px] lg:min-w-[34px] lg:min-h-[34px] flex items-center justify-center p-1.5 text-text-muted hover:text-status-error hover:bg-status-error/15 rounded-lg active:scale-95 transition-all cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-status-error"
                >
                  <LogOut className="w-4 h-4" aria-hidden="true" />
                </button>
              </Tooltip>
            </div>
          )}

          {/* Collapser footer & Live Status */}
          <div className="flex items-center justify-between pt-1 px-1">
            {!isCollapsed && (
              <div className="flex items-center gap-2 text-[11px] text-text-secondary">
                <span className="relative flex h-2 w-2" aria-hidden="true">
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
                  <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500 shadow-[0_0_8px_#10B981]" />
                </span>
                <span className="font-medium font-mono text-[11px] text-emerald-400">Gateway Live</span>
              </div>
            )}
            <Tooltip content={isCollapsed ? 'Perluas Sidebar' : 'Ciutkan Sidebar'} position="right">
              <button
                type="button"
                onClick={() => setIsCollapsed(!isCollapsed)}
                className={`hidden lg:flex min-w-[36px] min-h-[36px] items-center justify-center rounded-xl text-text-muted hover:text-white hover:bg-white/[0.06] active:scale-95 transition-all cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-accent ${
                  isCollapsed ? 'mx-auto' : 'ml-auto'
                }`}
                aria-label={isCollapsed ? 'Perluas Sidebar' : 'Ciutkan Sidebar'}
              >
                {isCollapsed ? (
                  <ChevronRight className="w-4 h-4" aria-hidden="true" />
                ) : (
                  <ChevronLeft className="w-4 h-4" aria-hidden="true" />
                )}
              </button>
            </Tooltip>
          </div>
        </div>
      </aside>
    </>
  );
};
