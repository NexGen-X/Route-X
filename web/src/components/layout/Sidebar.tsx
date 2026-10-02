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

interface SidebarProps {
  currentPath: string;
  onNavigate: (path: string) => void;
  isMobileOpen: boolean;
  setIsMobileOpen: (open: boolean) => void;
}

interface NavItem {
  name: string;
  path: string;
  icon: React.ComponentType<{ className?: string }>;
  perm?: string;
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

  React.useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isMobileOpen) {
        setIsMobileOpen(false);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isMobileOpen, setIsMobileOpen]);

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
        { name: 'CLI Integrations', path: '/cli-integrations', icon: Code2 },
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
        className={`fixed lg:sticky top-0 left-0 bottom-0 h-screen z-50 flex flex-col bg-bg-surface/40 backdrop-blur-md border-r border-white/[0.06] transition-all duration-300 select-none ${
          isCollapsed ? 'w-[68px]' : 'w-64'
        } ${isMobileOpen ? 'translate-x-0 shadow-2xl' : '-translate-x-full lg:translate-x-0'}`}
      >
        {/* Brand Header */}
        <div className={`h-14 flex items-center px-4 border-b border-white/[0.06] ${isCollapsed ? 'justify-center px-0' : 'justify-between'}`}>
          <button
            type="button"
            className="flex items-center gap-3 overflow-hidden cursor-pointer text-left focus:outline-none focus-visible:ring-1 focus-visible:ring-accent rounded-lg"
            onClick={() => {
              onNavigate('/');
              setIsMobileOpen(false);
            }}
            aria-label="Route-X Beranda"
          >
            <div className="w-8 h-8 rounded-lg bg-accent flex items-center justify-center flex-shrink-0 text-black font-extrabold text-sm shadow-[0_0_12px_rgba(190,242,100,0.2)]">
              RX
            </div>
            {!isCollapsed && (
              <div className="flex flex-col truncate">
                <span className="font-bold tracking-tight text-white text-sm leading-tight">Route-X</span>
                <span className="text-[10px] text-zinc-500 font-mono tracking-wider">AI GATEWAY & RUNTIME</span>
              </div>
            )}
          </button>
        </div>

        {/* Navigation List */}
        <nav aria-label="Navigasi Utama" className="flex-1 overflow-y-auto px-3 py-4 space-y-6">
          {navGroups.map((group, gIdx) => (
            <div key={gIdx} className="space-y-1">
              {!isCollapsed && (
                <div className="text-[10px] font-medium tracking-wider text-zinc-500 uppercase px-3 py-1">
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
                    className={`w-full flex items-center gap-2.5 px-3 py-2 rounded-lg text-xs transition-all duration-150 relative group cursor-pointer ${
                      isActive
                        ? 'bg-white/[0.05] text-white font-medium border border-white/[0.06]'
                        : 'text-zinc-400 hover:text-zinc-200 hover:bg-white/[0.03] border border-transparent'
                    } ${isCollapsed ? 'justify-center px-0' : ''}`}
                  >
                    {isActive && (
                      <span className="absolute left-1.5 top-1/2 -translate-y-1/2 w-0.5 h-3.5 bg-accent rounded-full shadow-[0_0_8px_rgba(190,242,100,0.6)]" aria-hidden="true" />
                    )}
                    <Icon
                      aria-hidden="true"
                      className={`w-[18px] h-[18px] flex-shrink-0 transition-colors ${
                        isActive ? 'text-accent' : 'text-zinc-500 group-hover:text-zinc-300'
                      }`}
                    />
                    {!isCollapsed && <span className="truncate">{item.name}</span>}
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

        {/* User profile & Collapser Footer */}
        <div className="p-3 border-t border-white/[0.06] flex flex-col gap-2 mt-auto">
          {!isCollapsed && user && (
            <div className="flex items-center justify-between px-2.5 py-2 rounded-lg bg-white/[0.02] border border-white/[0.06]">
              <div className="flex items-center gap-2 min-w-0">
                <div className="w-6 h-6 rounded-full bg-accent/20 border border-accent/40 text-accent flex items-center justify-center font-bold text-[10px] flex-shrink-0">
                  {user.display_name.charAt(0).toUpperCase()}
                </div>
                <div className="flex flex-col min-w-0 truncate">
                  <span className="text-xs font-medium text-white truncate">{user.display_name}</span>
                  <span className="text-[10px] text-zinc-500 truncate font-mono">
                    {principal?.roles[0] || 'User'}
                  </span>
                </div>
              </div>
              <Tooltip content="Keluar (Logout)" position="top">
                <button
                  type="button"
                  onClick={logout}
                  aria-label="Keluar dari akun"
                  className="p-1 text-zinc-400 hover:text-rose-400 hover:bg-rose-500/10 rounded transition-colors cursor-pointer"
                >
                  <LogOut className="w-3.5 h-3.5" aria-hidden="true" />
                </button>
              </Tooltip>
            </div>
          )}

          <div className="flex items-center justify-between pt-1 px-1">
            {!isCollapsed && (
              <div className="flex items-center gap-2 text-[11px] text-zinc-400">
                <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 flex-shrink-0 animate-pulse" />
                <span className="font-mono text-[10px] text-zinc-500">Route-X Ready</span>
              </div>
            )}
            <Tooltip content={isCollapsed ? 'Perluas Sidebar' : 'Ciutkan Sidebar'} position="right">
              <button
                type="button"
                onClick={() => setIsCollapsed(!isCollapsed)}
                className={`hidden lg:flex items-center justify-center w-7 h-7 rounded-lg text-zinc-500 hover:text-white hover:bg-white/[0.04] transition-colors cursor-pointer ${isCollapsed ? 'mx-auto' : 'ml-auto'}`}
                aria-label={isCollapsed ? 'Perluas Sidebar' : 'Ciutkan Sidebar'}
              >
                {isCollapsed ? <ChevronRight className="w-3.5 h-3.5" aria-hidden="true" /> : <ChevronLeft className="w-3.5 h-3.5" aria-hidden="true" />}
              </button>
            </Tooltip>
          </div>
        </div>
      </aside>
    </>
  );
};
