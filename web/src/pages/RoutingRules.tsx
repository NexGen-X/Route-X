import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { RoutingRule, CircuitBreakerStatus, Model, Provider } from '../types';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { PageHeader } from '../components/common/PageHeader';
import { Plus, Layers } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';
import {
  CircuitBreakersPanel,
  RuleCard,
  CreateRuleDrawer,
  EditRuleDrawer,
  RuleProvidersDrawer,
} from '../components/routing';
import { EmptyState } from '../components/common/EmptyState';
import { Skeleton } from '../components/common/Skeleton';

export const RoutingRules: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [rules, setRules] = useState<RoutingRule[]>([]);
  const [breakers, setBreakers] = useState<CircuitBreakerStatus[]>([]);
  const [models, setModels] = useState<Model[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [isRulesLoading, setIsRulesLoading] = useState(true);
  const [isBreakersLoading, setIsBreakersLoading] = useState(false);

  // Drawer Modals
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [isEditOpen, setIsEditOpen] = useState(false);
  const [editingRule, setEditingRule] = useState<RoutingRule | null>(null);
  const [isProvidersDrawerOpen, setIsProvidersDrawerOpen] = useState(false);
  const [selectedRuleForProviders, setSelectedRuleForProviders] = useState<RoutingRule | null>(null);

  const [rulesError, setRulesError] = useState<string | null>(null);
  const [breakersError, setBreakersError] = useState<string | null>(null);

  const loadRules = async () => {
    setIsRulesLoading(true);
    try {
      const [resRules, resModels, resProv] = await Promise.allSettled([
        api.routing.listAll(),
        api.models.listAll(),
        api.providers.listAll(),
      ]);

      if (resRules.status === 'fulfilled') {
        setRules(resRules.value?.items || []);
        setRulesError(null);
      } else {
        const err = resRules.reason;
        setRulesError(err instanceof Error ? err.message : String(err));
      }

      if (resModels.status === 'fulfilled') {
        setModels(resModels.value?.items || []);
      }

      if (resProv.status === 'fulfilled') {
        const pItems = Array.isArray(resProv.value) ? resProv.value : resProv.value?.items;
        setProviders(pItems || []);
      }
    } catch (err: unknown) {
      setRulesError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsRulesLoading(false);
    }
  };

  const loadBreakers = async () => {
    setIsBreakersLoading(true);
    try {
      const res = await api.breakers.list();
      setBreakers(res.items || []);
      setBreakersError(null);
    } catch (err: unknown) {
      setBreakersError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsBreakersLoading(false);
    }
  };

  useEffect(() => {
    void loadRules();
    void loadBreakers();
  }, []);

  const handleResetBreaker = async (providerId: string, model: string) => {
    try {
      await api.breakers.reset(providerId, model);
      toast.success('Circuit breaker berhasil direset');
      void loadBreakers();
    } catch (err: unknown) {
      toast.error(
        'Gagal mereset circuit breaker: ' + (err instanceof Error ? err.message : String(err)),
      );
    }
  };

  const handleToggle = async (r: RoutingRule) => {
    try {
      await api.routing.toggle(r.id, !r.enabled);
      toast.success(`Aturan ${!r.enabled ? 'diaktifkan' : 'dinonaktifkan'}`);
      void loadRules();
    } catch (err: unknown) {
      toast.error('Gagal toggle aturan: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleDelete = async (id: string) => {
    const confirmed = await confirmModal({
      title: 'Hapus Aturan Routing',
      message:
        'Apakah Anda yakin ingin menghapus aturan perutean ini? Jalur fallback akan disesuaikan secara dinamis.',
      danger: true,
      confirmText: 'Ya, Hapus Aturan',
    });
    if (!confirmed) return;

    try {
      await api.routing.delete(id);
      toast.success('Aturan perutean berhasil dihapus');
      void loadRules();
    } catch (err: unknown) {
      toast.error('Gagal menghapus: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleOpenEdit = (r: RoutingRule) => {
    setEditingRule(r);
    setIsEditOpen(true);
  };

  const handleOpenProviders = (r: RoutingRule) => {
    setSelectedRuleForProviders(r);
    setIsProvidersDrawerOpen(true);
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Mesin Aturan Perutean"
        actions={
          <Button
            variant="primary"
            size="sm"
            onClick={() => setIsCreateOpen(true)}
            icon={<Plus className="w-4 h-4" />}
            className="w-full sm:w-auto justify-center"
          >
            Tambah Aturan
          </Button>
        }
      />

      {rulesError && (
        <div className="mb-4">
          <QueryError message={rulesError} onRetry={() => void loadRules()} />
        </div>
      )}

      {isRulesLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {[...Array(3)].map((_, idx) => (
            <Card key={idx} className="p-5 animate-pulse space-y-4">
              <div className="flex items-center gap-3">
                <Skeleton className="w-9 h-9 rounded-full" />
                <div className="space-y-1.5 flex-1">
                  <Skeleton className="h-4 w-2/3" />
                  <Skeleton className="h-3 w-1/3" />
                </div>
              </div>
              <Skeleton className="h-12" />
            </Card>
          ))}
        </div>
      ) : rules.length === 0 ? (
        <EmptyState
          icon={<Layers className="w-5 h-5" />}
          title="Belum Ada Aturan Perutean Aktif"
          description="Konfigurasikan aturan perutean pertama: Passthrough langsung 1:1 ke upstream, Failover multi-provider otomatis, atau Cascade hemat biaya."
          action={
            <Button
              variant="primary"
              size="sm"
              onClick={() => setIsCreateOpen(true)}
              icon={<Plus className="w-4 h-4" />}
              className="h-9 px-4 text-xs font-semibold"
            >
              Tambah Aturan Pertama
            </Button>
          }
        />
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {rules.map((r) => (
            <RuleCard
              key={r.id}
              rule={r}
              models={models}
              providers={providers}
              onEdit={handleOpenEdit}
              onConfigureProviders={handleOpenProviders}
              onToggle={handleToggle}
              onDelete={handleDelete}
            />
          ))}
        </div>
      )}

      {/* Integrated Circuit Breakers Panel */}
      <CircuitBreakersPanel
        breakers={breakers}
        providers={providers}
        rules={rules}
        models={models}
        isBreakersLoading={isBreakersLoading}
        breakersError={breakersError}
        loadBreakers={loadBreakers}
        handleResetBreaker={handleResetBreaker}
      />

      {/* Drawer Buat Routing Rule */}
      <CreateRuleDrawer
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        models={models}
        providers={providers}
        onSuccess={loadRules}
      />

      {/* Drawer Edit Routing Rule */}
      <EditRuleDrawer
        isOpen={isEditOpen}
        onClose={() => {
          setIsEditOpen(false);
          setEditingRule(null);
        }}
        rule={editingRule}
        models={models}
        providers={providers}
        onSuccess={loadRules}
      />

      {/* Drawer Edit Penyedia & Bobot */}
      <RuleProvidersDrawer
        isOpen={isProvidersDrawerOpen}
        onClose={() => {
          setIsProvidersDrawerOpen(false);
          setSelectedRuleForProviders(null);
        }}
        rule={selectedRuleForProviders}
        models={models}
        providers={providers}
        onSuccess={loadRules}
      />
    </div>
  );
};
