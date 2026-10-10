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
          className="fixed inset-0 bg-black/70 backdrop-blur-sm z-40 lg:hidden"
          onClick={() => setIsMobileOpen(false)}
          aria-hidden="true"
        />
      )}

      <aside
        className={`fixed lg:static top-0 left-0 bottom-0 z-50 flex flex-col bg-bg-sidebar border-r border-zinc-800/80 transition-all duration-200 select-none ${
          isCollapsed ? 'w-[64px]' : 'w-[240px]'
        } ${isMobileOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'}`}
        aria-label="Sidebar Navigasi"
      >
        {/* Brand Header */}
        <div className="h-14 flex items-center px-3.5 border-b border-zinc-800/80 justify-between">
          <button
            type="button"
            className="group flex items-center gap-2.5 overflow-hidden cursor-pointer text-left focus:outline-none focus-visible:ring-1 focus-visible:ring-indigo-500/40 rounded-md p-1 -m-1 transition-colors"
            onClick={() => onNavigate('/')}
            aria-label="Route-X Beranda"
          >
            <RouteXLogo size={24} glow={false} className="shrink-0" />

            {!isCollapsed && (
              <div className="flex items-center gap-2 truncate">
                <span className="font-semibold tracking-tight text-white text-sm leading-none">
                  Route-X
                </span>
                <span className="text-[10px] font-mono text-indigo-400 px-1.5 py-0.5 rounded-full bg-indigo-500/10 border border-indigo-500/20">
                  v1.4
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
                <div className="px-2.5 pb-1 text-[11px] font-mono font-medium tracking-wider text-zinc-500 uppercase">
                  {group.title}
                </div>
              )}
              {isCollapsed && <div className="h-px bg-zinc-800/80 my-1.5 mx-1" aria-hidden="true" />}
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
                    className={`w-full min-h-[34px] flex items-center gap-2.5 px-2.5 py-1.5 text-xs transition-colors rounded-md group cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-indigo-500/40 ${
                      isActive
                        ? 'bg-indigo-500/10 text-indigo-300 font-medium border-l-2 border-indigo-500 rounded-r-md'
                        : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800/40'
                    } ${isCollapsed ? 'justify-center px-0' : ''}`}
                  >
                    <Icon
                      aria-hidden="true"
                      className={`w-4 h-4 flex-shrink-0 transition-colors ${
                        isActive ? 'text-indigo-400' : 'text-zinc-400 group-hover:text-zinc-200'
                      }`}
                    />
                    {!isCollapsed && (
                      <div className="flex-1 flex items-center justify-between min-w-0">
                        <span className="truncate">{item.name}</span>
                        {item.badge && (
                          <span className="text-[10px] font-mono px-1.5 py-0.5 rounded-full bg-indigo-500/15 text-indigo-400 border border-indigo-500/30">
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
        <div className="p-2 border-t border-zinc-800/80 bg-zinc-950 flex flex-col gap-1.5">
          {!isCollapsed && user ? (
            <div className="flex items-center justify-between px-2 py-1.5 rounded-md bg-zinc-900/60 border border-zinc-800/80 text-xs">
              {/* User details */}
              <div className="flex items-center gap-2 min-w-0">
                <div
                  className="w-6 h-6 rounded bg-zinc-800 text-zinc-200 flex items-center justify-center font-semibold text-[11px] shrink-0 border border-zinc-700/60"
                  aria-hidden="true"
                >
                  {user.display_name.charAt(0).toUpperCase()}
                </div>
                <div className="flex flex-col min-w-0 leading-tight">
                  <span className="text-xs font-medium text-zinc-200 truncate">
                    {user.display_name}
                  </span>
                  <span className="text-[10px] text-zinc-500 truncate font-mono">
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
                    className="p-1 text-zinc-400 hover:text-rose-400 rounded hover:bg-zinc-800/80 transition-colors cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-indigo-500/40"
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
                  className="w-7 h-7 rounded bg-zinc-800 text-zinc-200 flex items-center justify-center font-semibold text-xs shrink-0 border border-zinc-700/60"
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
                  className="p-1.5 text-zinc-400 hover:text-rose-400 rounded hover:bg-zinc-800/80 transition-colors cursor-pointer"
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
                className={`hidden lg:flex min-w-[28px] min-h-[28px] items-center justify-center rounded text-zinc-500 hover:text-zinc-200 hover:bg-zinc-800/60 transition-colors cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-indigo-500/40 ${
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
