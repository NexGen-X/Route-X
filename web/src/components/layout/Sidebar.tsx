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
  Cpu,
  Users,
  ShieldCheck,
  Webhook,
} from 'lucide-react';
import { useAuth } from '../../context/AuthContext';

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

  const navGroups: NavGroup[] = [
    {
      title: 'OVERVIEW & LOGS',
      items: [
        { name: 'Dashboard', path: '/', icon: LayoutDashboard },
        { name: 'Requests Inspector', path: '/requests', icon: Terminal },
        { name: 'Observability', path: '/observability', icon: Activity },
      ],
    },
    {
      title: 'UPSTREAMS & ROUTING',
      items: [
        { name: 'Providers', path: '/upstreams/providers', icon: Server },
        { name: 'Models & Pricing', path: '/upstreams/models', icon: Cpu },
        { name: 'Routing & Failover', path: '/gateway/routing', icon: GitFork },
        { name: 'CLI Integrations', path: '/cli-integrations', icon: Code2 },
        { name: 'Egress Pools', path: '/upstreams/egress', icon: Network },
      ],
    },
    {
      title: 'ACCESS & COST',
      items: [
        { name: 'API Keys', path: '/access/api-keys', icon: KeyRound },
        { name: 'Users', path: '/access/users', icon: Users },
        { name: 'Roles', path: '/access/roles', icon: ShieldCheck },
        { name: 'Budgets & Limits', path: '/gateway/budgets', icon: Coins },
      ],
    },
    {
      title: 'SYSTEM',
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
          className="fixed inset-0 bg-black/60 backdrop-blur-sm z-40 lg:hidden"
          onClick={() => setIsMobileOpen(false)}
        />
      )}

      <aside
        className={`fixed lg:static top-0 left-0 bottom-0 z-50 flex flex-col bg-bg-sidebar border-r border-[#1C1C1C] transition-all duration-300 select-none ${
          isCollapsed ? 'w-[70px]' : 'w-[260px]'
        } ${isMobileOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'}`}
      >
        {/* Brand Header */}
        <div className="h-16 flex items-center px-4 border-b border-[#1C1C1C] justify-between">
          <div className="flex items-center gap-3 overflow-hidden cursor-pointer" onClick={() => onNavigate('/')}>
            <div className="w-9 h-9 rounded-full bg-accent flex items-center justify-center flex-shrink-0 text-black font-extrabold text-base shadow-sm shadow-accent/20">
              RX
            </div>
            {!isCollapsed && (
              <div className="flex flex-col truncate">
                <span className="font-bold tracking-tight text-white text-base leading-tight">Route-X</span>
                <span className="text-[10px] text-text-secondary font-mono tracking-wider">AI GATEWAY & RUNTIME</span>
              </div>
            )}
          </div>
        </div>

        {/* Navigation List */}
        <div className="flex-1 overflow-y-auto px-3 py-4 space-y-6">
          {navGroups.map((group, gIdx) => (
            <div key={gIdx} className="space-y-1">
              {!isCollapsed && (
                <div className="px-3 pb-1.5 text-[11px] font-semibold tracking-wider text-text-muted uppercase">
                  {group.title}
                </div>
              )}
              {isCollapsed && <div className="h-px bg-border my-2 mx-1" />}
              {group.items.map((item) => {
                const Icon = item.icon;
                const isActive = currentPath === item.path;
                return (
                  <button
                    key={item.path}
                    onClick={() => {
                      onNavigate(item.path);
                      setIsMobileOpen(false);
                    }}
                    title={isCollapsed ? item.name : undefined}
                    className={`w-full flex items-center gap-3 px-3 py-2 rounded-nav text-sm transition-all relative group ${
                      isActive
                        ? 'bg-bg-surface-2 text-white font-medium border border-border shadow-inner'
                        : 'text-text-secondary hover:text-white hover:bg-bg-surface-2/60'
                    } ${isCollapsed ? 'justify-center px-0' : ''}`}
                  >
                    {isActive && (
                      <span className="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-5 bg-accent rounded-r-full" />
                    )}
                    <Icon
                      className={`w-[18px] h-[18px] flex-shrink-0 transition-colors ${
                        isActive ? 'text-accent' : 'text-text-secondary group-hover:text-text-primary'
                      }`}
                    />
                    {!isCollapsed && <span className="truncate">{item.name}</span>}
                  </button>
                );
              })}
            </div>
          ))}
        </div>

        {/* User profile & Collapser Footer */}
        <div className="p-3 border-t border-[#1C1C1C] bg-bg-surface-2/20 flex flex-col gap-2">
          {!isCollapsed && user && (
            <div className="flex items-center justify-between px-2 py-1.5 rounded-inner bg-bg-surface-2/40 border border-border/40">
              <div className="flex items-center gap-2.5 truncate">
                <div className="w-7 h-7 rounded-full bg-accent/20 border border-accent/40 text-accent flex items-center justify-center font-bold text-xs flex-shrink-0">
                  {user.display_name.charAt(0).toUpperCase()}
                </div>
                <div className="flex flex-col truncate">
                  <span className="text-xs font-semibold text-text-primary truncate">{user.display_name}</span>
                  <span className="text-[10px] text-text-muted truncate font-mono">
                    {principal?.roles[0] || 'User'}
                  </span>
                </div>
              </div>
              <button
                onClick={logout}
                title="Keluar (Logout)"
                aria-label="Keluar dari akun"
                className="p-1.5 text-text-muted hover:text-status-error hover:bg-status-error/10 rounded-inner transition-colors cursor-pointer"
              >
                <LogOut className="w-4 h-4" aria-hidden="true" />
              </button>
            </div>
          )}

          <div className="flex items-center justify-between">
            <button
              onClick={() => setIsCollapsed(!isCollapsed)}
              className="hidden lg:flex items-center justify-center w-9 h-9 rounded-nav text-text-muted hover:text-white hover:bg-bg-surface-2 transition-colors ml-auto cursor-pointer"
              title={isCollapsed ? 'Perluas Sidebar' : 'Ciutkan Sidebar'}
              aria-label={isCollapsed ? 'Perluas Sidebar' : 'Ciutkan Sidebar'}
            >
              {isCollapsed ? <ChevronRight className="w-4 h-4" aria-hidden="true" /> : <ChevronLeft className="w-4 h-4" aria-hidden="true" />}
            </button>
          </div>
        </div>
      </aside>
    </>
  );
};
