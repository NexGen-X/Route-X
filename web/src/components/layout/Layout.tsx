import React, { useState, useEffect } from 'react';
import { Sidebar } from './Sidebar';
import { Header } from './Header';
import { CommandPalette } from '../common/CommandPalette';
import { LayoutDashboard, Server, KeyRound, Sliders } from 'lucide-react';

interface LayoutProps {
  title: string;
  currentPath: string;
  onNavigate: (path: string) => void;
  children: React.ReactNode;
}

export const Layout: React.FC<LayoutProps> = ({ title, currentPath, onNavigate, children }) => {
  const [isCommandPaletteOpen, setIsCommandPaletteOpen] = useState(false);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setIsCommandPaletteOpen((prev) => !prev);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  const bottomNavItems = [
    { name: 'Dashboard', path: '/', icon: LayoutDashboard },
    { name: 'Providers', path: '/upstreams/providers', icon: Server },
    { name: 'API Keys', path: '/access/api-keys', icon: KeyRound },
    { name: 'Settings', path: '/system/settings', icon: Sliders },
  ];

  return (
    <div className="min-h-screen bg-bg-base flex pb-16 md:pb-0">
      {/* Sidebar is hidden on < md screens */}
      <div className="hidden md:flex">
        <Sidebar currentPath={currentPath} onNavigate={onNavigate} isMobileOpen={false} setIsMobileOpen={() => {}} />
      </div>

      <div className="flex-1 flex flex-col min-w-0">
        <Header 
          title={title} 
          onOpenMobileMenu={() => {}}
          onOpenCommandPalette={() => setIsCommandPaletteOpen(true)}
        />
        
        <main className="flex-1 p-4 sm:p-6 max-w-7xl w-full mx-auto overflow-x-hidden">
          {children}
        </main>
      </div>

      {/* Mobile Bottom Navigation (Visible only on < md) */}
      <nav className="md:hidden fixed bottom-0 left-0 right-0 h-16 bg-bg-base/80 backdrop-blur-xl border-t border-border z-40 flex items-center justify-around px-2 pb-safe">
        {bottomNavItems.map((item) => {
          const Icon = item.icon;
          const isActive = currentPath === item.path || (item.path !== '/' && currentPath.startsWith(item.path));
          return (
            <button
              key={item.path}
              onClick={() => onNavigate(item.path)}
              className={`flex flex-col items-center justify-center w-full h-full p-2 space-y-1 ${isActive ? 'text-accent' : 'text-text-secondary hover:text-white'}`}
            >
              <Icon className="w-6 h-6" />
              <span className="text-[10px] font-medium">{item.name}</span>
            </button>
          );
        })}
      </nav>

      <CommandPalette 
        isOpen={isCommandPaletteOpen}
        onClose={() => setIsCommandPaletteOpen(false)}
        onNavigate={onNavigate}
      />
    </div>
  );
};
