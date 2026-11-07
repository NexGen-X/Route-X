import React, { useEffect, useState } from 'react';
import type { RoutingRule, Model, Provider } from '../../types';
import { Drawer } from '../common/Drawer';
import { Button } from '../common/Button';
import { InputBilangan } from '../common/InputBilangan';
import { Checkbox } from '../common/Checkbox';
import { useToast } from '../../context/ToastContext';
import { api } from '../../api/client';
import { getErrorMessage } from '../../utils/error';

export interface RuleProvidersDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  rule: RoutingRule | null;
  models: Model[];
  providers: Provider[];
  onSuccess: () => void;
  // B-449: nama sumber (model/provider) yang gagal dimuat di halaman
  // induk — daftar kosong di drawer berarti belum terukur.
  sumberGagal?: string | null;
}

export const RuleProvidersDrawer: React.FC<RuleProvidersDrawerProps> = ({
  isOpen,
  onClose,
  rule,
  models,
  providers,
  onSuccess,
  sumberGagal,
}) => {
  const { toast } = useToast();
  const [ruleProviders, setRuleProviders] = useState<string[]>([]);
  const [ruleWeights, setRuleWeights] = useState<Record<string, number>>({});
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    if (!rule || !isOpen) return;
    const pIds = rule.providers ? rule.providers.map((p) => p.provider_id) : [];
    const pWeights: Record<string, number> = {};
    if (rule.providers) {
      rule.providers.forEach((p) => {
        if (p.weight) pWeights[p.provider_id] = p.weight;
      });
    }
    setRuleProviders(pIds);
    setRuleWeights(pWeights);
  }, [rule, isOpen]);

  const toggleProvider = (pid: string) => {
    setRuleProviders((prev) =>
      prev.includes(pid) ? prev.filter((p) => p !== pid) : [...prev, pid],
    );
  };

  const updateWeight = (pid: string, w: number) => {
    setRuleWeights((prev) => ({ ...prev, [pid]: w }));
  };

  const handleSaveProviders = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!rule) return;
    // Cermin penolakan server (V-FF-11): provider terpilih yang tidak
    // memetakan model target aturan akan ditolak — sampaikan di sini dengan
    // nama providernya, bukan menunggu error umum dari server. Lencana
    // "Model tidak tersedia" di daftar sudah menunjukkan hal yang sama.
    const targetM = models.find(
      (m) => m.id === rule.match_model_id || m.model_id === rule.match_model_id,
    );
    if (targetM?.providers) {
      const tanpaPemetaan = ruleProviders
        .map((pid) => providers.find((p) => p.id === pid))
        .filter(
          (p): p is Provider =>
            !!p && !targetM.providers!.some((mp) => mp.provider_id === p.id),
        );
      if (tanpaPemetaan.length > 0) {
        toast.error(
          `Provider berikut tidak memetakan model ${targetM.model_id}: ${tanpaPemetaan
            .map((p) => p.display_name || p.name)
            .join(', ')}. Lepas centangnya atau petakan dulu modelnya di halaman Penyedia.`
        );
        return;
      }
    }
    setIsSubmitting(true);
    try {
      await api.routing.setProviders(rule.id, {
        provider_ids: ruleProviders,
        weights: ruleWeights,
      });
      toast.success('Penyedia & bobot aturan berhasil diperbarui.');
      onSuccess();
      onClose();
    } catch (err: unknown) {
      toast.error('Gagal menyimpan providers: ' + getErrorMessage(err));
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Drawer
      isOpen={isOpen}
      onClose={onClose}
      title="Edit Penyedia & Bobot"
      footer={
        <div className="flex items-center justify-between gap-3 w-full">
          <Button type="button" variant="ghost" size="md" onClick={onClose}>
            Batal
          </Button>
          <Button
            type="submit"
            form="providers-routing-rule-form"
            variant="primary"
            size="md"
            isLoading={isSubmitting}
            className="flex-1 sm:flex-initial"
          >
            Simpan Providers
          </Button>
        </div>
      }
    >
      <form
        id="providers-routing-rule-form"
        noValidate
        onSubmit={handleSaveProviders}
        className="space-y-4 text-xs"
      >
{sumberGagal && (
          <p role="alert" className="text-xs text-amber-300 border border-amber-500/30 bg-amber-500/10 rounded-lg p-3">
            Daftar gagal dimuat: {sumberGagal} — pilihan di bawah belum terukur, bukan daftar kosong.
          </p>
        )}
                <div className="space-y-2">
          {providers.map((p) => {
            const targetM = models.find(
              (m) =>
                m.id === rule?.match_model_id ||
                m.model_id === rule?.match_model_id,
            );
            const matchedUpstream = targetM?.providers?.find((mp) => mp.provider_id === p.id);
            const isChecked = ruleProviders.includes(p.id);
            return (
              <div
                key={p.id}
                className={`p-2.5 rounded-lg border transition-all ${
                  isChecked
                    ? 'bg-purple-500/10 border-purple-400/40 shadow-sm shadow-purple-500/5'
                    : 'bg-bg-surface-1 border-border/70 hover:border-border'
                }`}
              >
                <div className="flex items-center justify-between gap-2">
                  <div className="flex items-center gap-2 font-semibold text-white cursor-pointer min-w-0">
                    <Checkbox checked={isChecked} onChange={() => toggleProvider(p.id)} />
                    <span className="truncate">{p.display_name || p.name}</span>
                    <span className="text-[10px] text-text-muted font-mono">({p.kind})</span>
                  </div>
                  {targetM ? (
                    matchedUpstream ? (
                      <span className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 shrink-0">
                        ✓ {matchedUpstream.upstream_model_name}
                      </span>
                    ) : (
                      <span className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-bg-surface-2 text-text-muted border border-border/60 shrink-0">
                        Model tidak tersedia
                      </span>
                    )
                  ) : null}
                </div>
                {isChecked && (
                  <div className="pl-6 flex items-center gap-2 mt-2 pt-2 border-t border-border/40">
                    <label className="text-[11px] text-text-secondary">Bobot (Weight):</label>
                    <div className="w-24">
                      <InputBilangan
                        ariaLabel={`Bobot ${p.display_name || p.name}`}
                        value={ruleWeights[p.id] ?? 1}
                        onChange={(n) => updateWeight(p.id, n)}
                        min={1}
                        className="w-full px-2 py-0.5 bg-bg-surface-2 border border-border rounded text-white text-xs font-mono focus:outline-none focus:border-purple-400 focus:ring-1 focus:ring-purple-400"
                      />
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </form>
    </Drawer>
  );
};
