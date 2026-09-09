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
const RateLimits = React.lazy(() => import('./pages/RateLimits').then((m) => ({ default: m.RateLimits })));
const Budgets = React.lazy(() => import('./pages/Budgets').then((m) => ({ default: m.Budgets })));
const APIKeys = React.lazy(() => import('./pages/APIKeys').then((m) => ({ default: m.APIKeys })));
const Settings = React.lazy(() => import('./pages/Settings').then((m) => ({ default: m.Settings })));
const Diagnostics = React.lazy(() => import('./pages/Diagnostics').then((m) => ({ default: m.Diagnostics })));
const CLIIntegrations = React.lazy(() => import('./pages/CLIIntegrations').then((m) => ({ default: m.CLIIntegrations })));
// Halaman katalog model kanonis (dipakai rute /models dan /upstreams/models)
const Models = React.lazy(() => import('./pages/Models').then((m) => ({ default: m.Models })));

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

  const getPageInfo = (): { title: string; subtitle: string; content: React.ReactNode } => {
    switch (currentPath) {
      case '/':
        return {
          title: 'Dashboard',
          subtitle: 'Live health of the inference proxy — traffic, credential pools, and upstreams.',
          content: <Dashboard onNavigate={navigate} />,
        };
      case '/observability':
        return {
          title: 'Observabilitas & Telemetri',
          subtitle: 'Deret waktu metrik, breakdown penggunaan, dan analitik latensi',
          content: <Observability />,
        };
      case '/requests':
        return {
          title: 'Requests Inspector',
          subtitle: 'Penelusuran detail request, timeline event, dan raw payload',
          content: <Requests />,
        };
      case '/upstreams/providers':
      case '/providers':
        return {
          title: 'Upstream Providers',
          subtitle: 'Koneksi provider AI, pemeriksaan kesehatan, dan kredensial',
          content: <Providers />,
        };
      case '/upstreams/models':
      case '/models':
        return {
          title: 'Katalog Model',
          subtitle: 'Katalog model kanonis, pemetaan upstream, dan harga per model',
          content: <Models />,
        };
      case '/upstreams/egress':
      case '/egress':
        return {
          title: 'Egress Proxy Pools',
          subtitle: 'Manajemen pool proxy keluar HTTP/HTTPS/SOCKS5',
          content: <Egress />,
        };
      case '/gateway/routing':
      case '/routing':
        return {
          title: 'Routing Rules',
          subtitle: 'Mesin aturan pemilihan provider dan kebijakan failover',
          content: <RoutingRules />,
        };
      case '/cli-integrations':
      case '/integrations':
        return {
          title: 'CLI Integrations & 3-Mode Configurator',
          subtitle: 'Pemindai otomatis perkakas AI CLI di sistem host dan konfigurasi multi-mode terpadu',
          content: <CLIIntegrations />,
        };
      case '/gateway/rate-limits':
      case '/rate-limits':
        return {
          title: 'Rate Limits',
          subtitle: 'Pembatasan laju kuota terdistribusi per-key, IP, atau model',
          content: <RateLimits />,
        };
      case '/gateway/budgets':
      case '/budgets':
        return {
          title: 'Budgets & Cost Limits',
          subtitle: 'Alokasi pagu pengeluaran USD skala 8 desimal dan pencegahan pembengkakan biaya',
          content: <Budgets />,
        };
      case '/gateway/breakers':
      case '/breakers':
        return {
          title: 'Routing & Failover',
          subtitle: 'Mesin aturan pemilihan provider, kebijakan failover, dan pemutus sirkuit terdistribusi',
          content: <RoutingRules />,
        };
      case '/access/api-keys':
      case '/api-keys':
        return {
          title: 'Client API Keys',
          subtitle: 'Kunci otentikasi klien untuk Cursor, Cline, Open WebUI, dan skrip personal',
          content: <APIKeys />,
        };
      case '/system/settings':
      case '/settings':
        return {
          title: 'Runtime Settings',
          subtitle: 'Konfigurasi parameter gateway dinamis dan profil pemilik',
          content: <Settings />,
        };
      case '/system/jobs':
      case '/system/diagnostics':
      case '/diagnostics':
        return {
          title: 'System Diagnostics & Workers',
          subtitle: 'Status memori Go runtime, koneksi PostgreSQL, dan supervisor worker latar belakang',
          content: <Diagnostics />,
        };
      default:
        return {
          title: 'Dashboard',
          subtitle: 'Ringkasan operasional lalu lintas inferensi',
          content: <Dashboard onNavigate={navigate} />,
        };
    }
  };

  const { title, subtitle, content } = getPageInfo();

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
          subtitle={subtitle}
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
