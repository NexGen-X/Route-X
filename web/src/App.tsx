import React, { useState, useEffect } from 'react';
import { AuthProvider, useAuth } from './context/AuthContext';
import { ToastProvider } from './context/ToastContext';
import { Sidebar } from './components/layout/Sidebar';
import { Header } from './components/layout/Header';
import { Login } from './pages/Login';
import { ChangePassword } from './pages/ChangePassword';
import { PageErrorBoundary } from './components/common/PageErrorBoundary';
import { CommandPalette } from './components/common/CommandPalette';
import { Loader2 } from 'lucide-react';

// Code-splitting dinamis via React.lazy() untuk memperkecil ukuran bundle awal dan mempercepat FCP
const Dashboard = React.lazy(() => import('./pages/Dashboard').then((m) => ({ default: m.Dashboard })));
const Observability = React.lazy(() => import('./pages/Observability').then((m) => ({ default: m.Observability })));
const Requests = React.lazy(() => import('./pages/Requests').then((m) => ({ default: m.Requests })));
const Providers = React.lazy(() => import('./pages/Providers').then((m) => ({ default: m.Providers })));
const Egress = React.lazy(() => import('./pages/Egress').then((m) => ({ default: m.Egress })));
const RoutingRules = React.lazy(() => import('./pages/RoutingRules').then((m) => ({ default: m.RoutingRules })));
const Budgets = React.lazy(() => import('./pages/Budgets').then((m) => ({ default: m.Budgets })));
const APIKeys = React.lazy(() => import('./pages/APIKeys').then((m) => ({ default: m.APIKeys })));
const Webhooks = React.lazy(() => import('./pages/Webhooks').then((m) => ({ default: m.Webhooks })));
const Settings = React.lazy(() => import('./pages/Settings').then((m) => ({ default: m.Settings })));
const Diagnostics = React.lazy(() => import('./pages/Diagnostics').then((m) => ({ default: m.Diagnostics })));
const CLIIntegrations = React.lazy(() => import('./pages/CLIIntegrations').then((m) => ({ default: m.CLIIntegrations })));
// Halaman identitas: daftar akun admin dan status sesi.
const UsersPage = React.lazy(() => import('./pages/Users').then((m) => ({ default: m.UsersPage })));

const Shell: React.FC = () => {
  const { principal, user, isLoading } = useAuth();
  const [currentPath, setCurrentPath] = useState('/');
  const [isMobileOpen, setIsMobileOpen] = useState(false);
  const [isCommandPaletteOpen, setIsCommandPaletteOpen] = useState(false);

  useEffect(() => {
    const handleHash = () => {
      const hash = window.location.hash.replace(/^#/, '') || '/';
      setCurrentPath(hash);
    };
    window.addEventListener('hashchange', handleHash);
    handleHash();
    return () => window.removeEventListener('hashchange', handleHash);
  }, []);

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

  const navigate = (path: string) => {
    window.location.hash = path;
    setCurrentPath(path);
  };

  if (isLoading) {
    return (
      <div className="min-h-screen bg-bg-base flex flex-col items-center justify-center">
        <div className="w-12 h-12 rounded-full bg-accent flex items-center justify-center font-extrabold text-black text-xl mb-4 shadow-lg shadow-accent/20 animate-pulse">
          RX
        </div>
        <p className="text-xs text-text-muted font-mono flex items-center gap-2">
          <Loader2 className="w-3.5 h-3.5 animate-spin text-accent" />
          Memuat Konsol Route-X...
        </p>
      </div>
    );
  }

  if (!principal || !user) {
    return <Login />;
  }

  if (user.must_change_password) {
    return <ChangePassword />;
  }

  const getPageInfo = (): { title: string; content: React.ReactNode } => {
    const [pathname, searchStr] = currentPath.split('?');
    const searchParams = new URLSearchParams(searchStr || '');

    switch (pathname) {
      case '/':
        return {
          title: 'Dashboard',
          content: <Dashboard onNavigate={navigate} />,
        };
      case '/observability':
        return {
          title: 'Observabilitas & Telemetri',
          content: <Observability />,
        };
      case '/requests':
        return {
          title: 'Requests Inspector',
          content: <Requests />,
        };
      case '/upstreams/providers':
      case '/providers':
        return {
          title: 'Upstream Providers',
          content: <Providers />,
        };
      case '/upstreams/models':
      case '/models':
        // Redirect otomatis ke Upstream Providers agar lebih simpel
        navigate('/upstreams/providers');
        return {
          title: 'Upstream Providers',
          content: <Providers />,
        };
      case '/upstreams/egress':
      case '/egress':
        return {
          title: 'Egress Proxy Pools',
          content: <Egress />,
        };
      case '/upstreams/routing':
      case '/gateway/routing':
      case '/routing':
        return {
          title: 'Routing Rules',
          content: <RoutingRules />,
        };
      case '/cli-integrations':
      case '/integrations':
        return {
          title: 'CLI Integrations & 3-Mode Configurator',
          content: <CLIIntegrations />,
        };
      case '/gateway/rate-limits':
      case '/rate-limits':
        navigate('/gateway/budgets?tab=limits');
        return {
          title: 'Budgets & Rate Limits',
          content: <Budgets initialTab="limits" />,
        };
      case '/gateway/budgets':
      case '/budgets':
        return {
          title: 'Budgets & Rate Limits',
          content: <Budgets initialTab={searchParams.get('tab') === 'limits' ? 'limits' : 'budgets'} />,
        };
      case '/gateway/breakers':
      case '/breakers':
        return {
          title: 'Routing & Failover',
          content: <RoutingRules />,
        };
      case '/access/api-keys':
      case '/api-keys':
        return {
          title: 'Client API Keys',
          content: <APIKeys />,
        };
      case '/access/users':
      case '/users':
        return {
          title: 'Pengguna Admin',
          content: <UsersPage />,
        };
      case '/webhooks':
      case '/system/webhooks':
        return {
          title: 'Webhooks',
          content: <Webhooks />,
        };
      case '/system/settings':
      case '/settings':
        return {
          title: 'Runtime Settings',
          content: <Settings />,
        };
      case '/system/jobs':
      case '/system/diagnostics':
      case '/diagnostics':
        return {
          title: 'System Diagnostics & Workers',
          content: <Diagnostics />,
        };
      default:
        return {
          title: 'Dashboard',
          content: <Dashboard onNavigate={navigate} />,
        };
    }
  };

  const { title, content } = getPageInfo();

  return (
    <div className="min-h-screen bg-bg-base flex">
      <Sidebar
        currentPath={currentPath}
        onNavigate={navigate}
        isMobileOpen={isMobileOpen}
        setIsMobileOpen={setIsMobileOpen}
      />
      <div className="flex-1 flex flex-col min-w-0">
        <Header
          title={title}
          onOpenMobileMenu={() => setIsMobileOpen(true)}
          onOpenCommandPalette={() => setIsCommandPaletteOpen(true)}
        />
        <main className="flex-1 p-3.5 sm:p-6 max-w-7xl w-full mx-auto overflow-x-hidden">
          <PageErrorBoundary key={currentPath} pageName={title}>
            <React.Suspense
              fallback={
                <div className="flex flex-col items-center justify-center py-24 text-text-muted">
                  <Loader2 className="w-7 h-7 animate-spin text-accent mb-2" />
                  <span className="text-xs font-mono">Memuat modul {title}...</span>
                </div>
              }
            >
              {content}
            </React.Suspense>
          </PageErrorBoundary>
        </main>
      </div>
      <CommandPalette
        isOpen={isCommandPaletteOpen}
        onClose={() => setIsCommandPaletteOpen(false)}
        onNavigate={navigate}
      />
    </div>
  );
};

export const App: React.FC = () => {
  return (
    <ToastProvider>
      <AuthProvider>
        <Shell />
      </AuthProvider>
    </ToastProvider>
  );
};
