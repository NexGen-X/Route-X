import React, { useState, useEffect } from 'react';
import { AuthProvider, useAuth } from './context/AuthContext';
import { ToastProvider } from './context/ToastContext';
import { Sidebar } from './components/layout/Sidebar';
import { Header } from './components/layout/Header';
import { MobileNav } from './components/layout/MobileNav';
import { Login } from './pages/Login';
import { ChangePassword } from './pages/ChangePassword';
import { PageErrorBoundary } from './components/common/PageErrorBoundary';
import { WidgetErrorBoundary } from './components/common/WidgetErrorBoundary';
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
      <div className="min-h-screen bg-bg-base flex flex-col items-center justify-center gap-3">
        <div className="w-10 h-10 rounded-xl bg-gradient-to-tr from-accent to-accent-hover flex items-center justify-center font-extrabold text-white text-lg shadow-lg shadow-black/40 ring-1 ring-white/10">
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
    return (
      <PageErrorBoundary pageName="Masuk (Login)">
        <Login />
      </PageErrorBoundary>
    );
  }

  if (user.must_change_password) {
    return (
      <PageErrorBoundary pageName="Ganti Kata Sandi">
        <ChangePassword />
      </PageErrorBoundary>
    );
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
          title: 'Permintaan',
          content: <Requests />,
        };
      case '/upstreams/providers':
      case '/providers':
        return {
          title: 'Penyedia Upstream',
          content: <Providers />,
        };
      case '/upstreams/models':
      case '/models':
        // Redirect otomatis ke Penyedia Upstream agar lebih simpel
        navigate('/upstreams/providers');
        return {
          title: 'Penyedia Upstream',
          content: <Providers />,
        };
      case '/upstreams/egress':
      case '/egress':
        return {
          title: 'Pool Egress',
          content: <Egress />,
        };
      case '/upstreams/routing':
      case '/gateway/routing':
      case '/routing':
        return {
          title: 'Aturan Perutean',
          content: <RoutingRules />,
        };
      case '/cli-integrations':
      case '/integrations':
        return {
          title: 'Integrasi CLI & Konfigurator 3 Mode',
          content: <CLIIntegrations />,
        };
      case '/gateway/rate-limits':
      case '/rate-limits':
        navigate('/gateway/budgets?tab=limits');
        return {
          title: 'Anggaran & Batas Laju',
          content: <Budgets initialTab="limits" />,
        };
      case '/gateway/budgets':
      case '/budgets':
        return {
          title: 'Anggaran & Batas Laju',
          content: <Budgets initialTab={searchParams.get('tab') === 'limits' ? 'limits' : 'budgets'} />,
        };
      case '/gateway/breakers':
      case '/breakers':
        return {
          title: 'Perutean & Failover',
          content: <RoutingRules />,
        };
      case '/access/api-keys':
      case '/api-keys':
        return {
          title: 'Kunci API Klien',
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
          title: 'Pengaturan Runtime',
          content: <Settings />,
        };
      case '/system/jobs':
      case '/system/diagnostics':
      case '/diagnostics':
        // Halaman Diagnostics dipensiunkan (Fase 2 R6): isinya pindah ke
        // section Kesehatan Sistem di Observability. Hash lama tetap
        // diterima agar bookmark tidak mati.
        return {
          title: 'Observabilitas & Telemetri',
          content: <Observability />,
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
      <WidgetErrorBoundary title="Navigasi Sidebar" className="m-2 rounded-xl">
        <Sidebar
          currentPath={currentPath}
          onNavigate={navigate}
          isMobileOpen={isMobileOpen}
          setIsMobileOpen={setIsMobileOpen}
        />
      </WidgetErrorBoundary>
      <div className="flex-1 flex flex-col min-w-0">
        <WidgetErrorBoundary title="Header Panel" className="m-2 rounded-xl">
          <Header
            title={title}
            onOpenMobileMenu={() => setIsMobileOpen(true)}
            onOpenCommandPalette={() => setIsCommandPaletteOpen(true)}
          />
        </WidgetErrorBoundary>
        <main className="flex-1 p-4 sm:p-6 pb-24 lg:pb-6 max-w-7xl w-full mx-auto overflow-x-hidden">
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
      <MobileNav
        currentPath={currentPath}
        onNavigate={navigate}
        onOpenMore={() => setIsMobileOpen(true)}
      />
      <WidgetErrorBoundary title="Command Palette">
        <CommandPalette
          isOpen={isCommandPaletteOpen}
          onClose={() => setIsCommandPaletteOpen(false)}
          onNavigate={navigate}
        />
      </WidgetErrorBoundary>
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
