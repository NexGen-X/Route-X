import React, { useState, useEffect } from 'react';
import { AuthProvider, useAuth } from './context/AuthContext';
import { Sidebar } from './components/layout/Sidebar';
import { Header } from './components/layout/Header';
import { Login } from './pages/Login';
import { ChangePassword } from './pages/ChangePassword';
import { Dashboard } from './pages/Dashboard';
import { Observability } from './pages/Observability';
import { Requests } from './pages/Requests';
import { Providers } from './pages/Providers';
import { Models } from './pages/Models';
import { Egress } from './pages/Egress';
import { RoutingRules } from './pages/RoutingRules';
import { RateLimits } from './pages/RateLimits';
import { Budgets } from './pages/Budgets';
import { ContentFilters } from './pages/ContentFilters';
import { Bans } from './pages/Bans';
import { Breakers } from './pages/Breakers';
import { APIKeys } from './pages/APIKeys';
import { Users } from './pages/Users';
import { Sessions } from './pages/Sessions';
import { Webhooks } from './pages/Webhooks';
import { Settings } from './pages/Settings';
import { Jobs } from './pages/Jobs';
import { AuditLogs } from './pages/AuditLogs';
import { Diagnostics } from './pages/Diagnostics';
import { Loader2 } from 'lucide-react';

const Shell: React.FC = () => {
  const { principal, user, isLoading } = useAuth();
  const [currentPath, setCurrentPath] = useState('/');
  const [isMobileOpen, setIsMobileOpen] = useState(false);

  useEffect(() => {
    const handleHash = () => {
      const hash = window.location.hash.replace(/^#/, '') || '/';
      setCurrentPath(hash);
    };
    window.addEventListener('hashchange', handleHash);
    handleHash();
    return () => window.removeEventListener('hashchange', handleHash);
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
          title: 'Dashboard Overview',
          subtitle: 'Ringkasan operasional lalu lintas inferensi dan performa model',
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
        return {
          title: 'Upstream Providers',
          subtitle: 'Koneksi provider AI, pemeriksaan kesehatan, dan kredensial',
          content: <Providers />,
        };
      case '/upstreams/models':
        return {
          title: 'Models & Pricing',
          subtitle: 'Katalog model kanonik dan struktur penetapan harga moneter',
          content: <Models />,
        };
      case '/upstreams/egress':
        return {
          title: 'Egress Proxy Pools',
          subtitle: 'Manajemen pool proxy keluar HTTP/HTTPS/SOCKS5',
          content: <Egress />,
        };
      case '/gateway/routing':
        return {
          title: 'Routing Rules',
          subtitle: 'Mesin aturan pemilihan provider dan kebijakan failover',
          content: <RoutingRules />,
        };
      case '/gateway/rate-limits':
        return {
          title: 'Rate Limits',
          subtitle: 'Pembatasan laju kuota terdistribusi per-key, IP, atau model',
          content: <RateLimits />,
        };
      case '/gateway/budgets':
        return {
          title: 'Budgets & Cost Control',
          subtitle: 'Alokasi anggaran USD skala 8 desimal dan pencegahan pembengkakan biaya',
          content: <Budgets />,
        };
      case '/gateway/filters':
        return {
          title: 'Content Filters',
          subtitle: 'Penyaringan konten masukan/keluaran dan deteksi pola sensitif',
          content: <ContentFilters />,
        };
      case '/gateway/bans':
        return {
          title: 'Active Bans',
          subtitle: 'Daftar pemblokiran IP dan kunci API yang mencurigakan',
          content: <Bans />,
        };
      case '/gateway/breakers':
        return {
          title: 'Circuit Breakers',
          subtitle: 'Status pemutus arus terdistribusi dan pemulihan darurat',
          content: <Breakers />,
        };
      case '/access/api-keys':
        return {
          title: 'Client API Keys',
          subtitle: 'Kunci otentikasi klien dengan hashing HMAC-SHA256 ber-pepper',
          content: <APIKeys />,
        };
      case '/access/users':
        return {
          title: 'Users & RBAC',
          subtitle: 'Manajemen akun konsol dan hak akses berbutir halus',
          content: <Users />,
        };
      case '/access/sessions':
        return {
          title: 'Active Sessions',
          subtitle: 'Pengawasan sesi login aktif dan pemutusan instan',
          content: <Sessions />,
        };
      case '/automation/webhooks':
        return {
          title: 'Webhooks & Deliveries',
          subtitle: 'Pengiriman notifikasi otomatis ke endpoint eksternal',
          content: <Webhooks />,
        };
      case '/system/settings':
        return {
          title: 'Runtime Settings',
          subtitle: 'Konfigurasi parameter gateway dinamis',
          content: <Settings />,
        };
      case '/system/jobs':
        return {
          title: 'Background Jobs',
          subtitle: 'Supervisor tugas latar belakang Route-X',
          content: <Jobs />,
        };
      case '/system/audit':
        return {
          title: 'Audit Logs',
          subtitle: 'Jejak audit perubahan konfigurasi dan aksi administratif',
          content: <AuditLogs />,
        };
      case '/system/diagnostics':
        return {
          title: 'System Diagnostics',
          subtitle: 'Status memori Go runtime dan statistik koneksi PostgreSQL',
          content: <Diagnostics />,
        };
      default:
        return {
          title: 'Dashboard Overview',
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
        />
        <main className="flex-1 p-6 max-w-7xl w-full mx-auto overflow-x-hidden">
          {content}
        </main>
      </div>
    </div>
  );
};

export const App: React.FC = () => {
  return (
    <AuthProvider>
      <Shell />
    </AuthProvider>
  );
};
