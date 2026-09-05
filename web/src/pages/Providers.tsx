import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Provider, Credential } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { Server, Plus, Key, Activity, Trash2, CheckCircle2, XCircle, DownloadCloud } from 'lucide-react';

export const Providers: React.FC = () => {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [selectedProvider, setSelectedProvider] = useState<Provider | null>(null);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [isCredModalOpen, setIsCredModalOpen] = useState(false);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [probingId, setProbingId] = useState<string | null>(null);
  const [syncingId, setSyncingId] = useState<string | null>(null);
  const [probeResult, setProbeResult] = useState<any | null>(null);

  // Form states
  const [newProv, setNewProv] = useState({
    name: '',
    display_name: '',
    kind: 'openai',
    base_url: 'https://api.openai.com/v1',
    priority: 100,
    weight: 100,
    timeout_ms: 30000,
  });

  const [newCred, setNewCred] = useState({
    label: '',
    api_key: '',
  });

  const loadProviders = async () => {
    try {
      const res = await api.providers.list();
      setProviders(res.items || []);
    } catch (err) {
      console.error(err);
    }
  };

  useEffect(() => {
    loadProviders();
  }, []);

  const handleOpenCredentials = async (prov: Provider) => {
    setSelectedProvider(prov);
    setIsCredModalOpen(true);
    try {
      const res = await api.credentials.list(prov.id);
      setCredentials(res.items || []);
    } catch (err) {
      console.error(err);
    }
  };

  const handleAddCredential = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedProvider) return;
    try {
      await api.credentials.create(selectedProvider.id, newCred);
      setNewCred({ label: '', api_key: '' });
      const res = await api.credentials.list(selectedProvider.id);
      setCredentials(res.items || []);
    } catch (err) {
      alert('Gagal menambah kredensial: ' + err);
    }
  };

  const handleDeleteCredential = async (credId: string) => {
    if (!selectedProvider || !confirm('Yakin ingin menghapus kredensial ini?')) return;
    try {
      await api.credentials.delete(selectedProvider.id, credId);
      setCredentials((prev) => prev.filter((c) => c.id !== credId));
    } catch (err) {
      alert('Gagal menghapus kredensial: ' + err);
    }
  };

  const handleProbe = async (prov: Provider) => {
    setProbingId(prov.id);
    setProbeResult(null);
    try {
      const res = await api.providers.probe(prov.id);
      setProbeResult({ providerId: prov.id, ...res });
      loadProviders();
    } catch (err: any) {
      setProbeResult({ providerId: prov.id, status: 'error', error: err.message });
    } finally {
      setProbingId(null);
    }
  };

  const handleSyncModels = async (prov: Provider) => {
    setSyncingId(prov.id);
    try {
      const res = await api.providers.syncModels(prov.id);
      alert(res.message || `Berhasil menyinkronkan ${res.count} model.`);
    } catch (err: any) {
      alert('Gagal menyinkronkan model: ' + (err.message || err));
    } finally {
      setSyncingId(null);
    }
  };

  const handleToggle = async (prov: Provider) => {
    try {
      await api.providers.toggle(prov.id, !prov.enabled);
      loadProviders();
    } catch (err) {
      alert('Gagal mengubah status: ' + err);
    }
  };

  const handleCreateProvider = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.providers.create(newProv);
      setIsCreateModalOpen(false);
      setNewProv({
        name: '',
        display_name: '',
        kind: 'openai',
        base_url: 'https://api.openai.com/v1',
        priority: 100,
        weight: 100,
        timeout_ms: 30000,
      });
      loadProviders();
    } catch (err) {
      alert('Gagal membuat provider: ' + err);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Upstream Providers</h2>
          <p className="text-xs text-text-secondary mt-1">
            Koneksi ke penyedia AI hulu (OpenAI, Anthropic, Google, vLLM, Ollama) dengan rotasi kredensial.
          </p>
        </div>
        <Button
          variant="primary"
          size="sm"
          onClick={() => setIsCreateModalOpen(true)}
          icon={<Plus className="w-4 h-4" />}
        >
          Tambah Provider
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {providers.map((p) => {
          const isHealthy = p.last_health_status === 'healthy';
          return (
            <Card key={p.id} className="flex flex-col justify-between">
              <div>
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div className="w-9 h-9 rounded-full bg-accent/10 border border-accent/20 flex items-center justify-center text-accent">
                      <Server className="w-5 h-5" />
                    </div>
                    <div>
                      <h4 className="text-sm font-bold text-white">{p.display_name || p.name}</h4>
                      <span className="text-[11px] text-text-muted font-mono">{p.name} ({p.kind})</span>
                    </div>
                  </div>
                  <Badge variant={isHealthy ? 'success' : 'error'}>
                    {p.last_health_status || 'unknown'}
                  </Badge>
                </div>

                <div className="mt-4 space-y-2 text-xs">
                  <div className="flex justify-between py-1 border-b border-border/40">
                    <span className="text-text-muted">Base URL</span>
                    <span className="font-mono text-text-secondary truncate max-w-[170px]" title={p.base_url}>
                      {p.base_url}
                    </span>
                  </div>
                  <div className="flex justify-between py-1 border-b border-border/40">
                    <span className="text-text-muted">Prioritas / Bobot</span>
                    <span className="font-mono text-white">{p.priority} / {p.weight}</span>
                  </div>
                  <div className="flex justify-between py-1 border-b border-border/40">
                    <span className="text-text-muted">Latensi Terakhir</span>
                    <span className="font-mono text-accent">{p.last_latency_ms ? `${p.last_latency_ms} ms` : '-'}</span>
                  </div>
                </div>

                {probeResult && probeResult.providerId === p.id && (
                  <div className={`mt-3 p-2.5 rounded-inner text-xs flex items-center gap-2 ${
                    probeResult.status === 'healthy'
                      ? 'bg-status-success/10 text-status-success border border-status-success/20'
                      : 'bg-status-error/10 text-status-error border border-status-error/20'
                  }`}>
                    {probeResult.status === 'healthy' ? <CheckCircle2 className="w-4 h-4" /> : <XCircle className="w-4 h-4" />}
                    <span>{probeResult.status.toUpperCase()} ({probeResult.latency_ms ?? 0} ms) {probeResult.error && `- ${probeResult.error}`}</span>
                  </div>
                )}
              </div>

              <div className="mt-5 pt-3 border-t border-border flex items-center justify-between gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => handleOpenCredentials(p)}
                  icon={<Key className="w-3.5 h-3.5" />}
                >
                  Kredensial
                </Button>

                <div className="flex items-center gap-2">
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => handleSyncModels(p)}
                    isLoading={syncingId === p.id}
                    icon={<DownloadCloud className="w-3.5 h-3.5" />}
                    title="Tarik katalog model otomatis dari endpoint upstream provider"
                  >
                    Tarik Model
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => handleProbe(p)}
                    isLoading={probingId === p.id}
                    icon={<Activity className="w-3.5 h-3.5" />}
                  >
                    Probe
                  </Button>
                  <Button
                    variant={p.enabled ? 'danger' : 'secondary'}
                    size="sm"
                    onClick={() => handleToggle(p)}
                  >
                    {p.enabled ? 'Nonaktifkan' : 'Aktifkan'}
                  </Button>
                </div>
              </div>
            </Card>
          );
        })}
      </div>

      {/* Modal Tambah Provider */}
      <Modal
        isOpen={isCreateModalOpen}
        onClose={() => setIsCreateModalOpen(false)}
        title="Daftarkan Upstream Provider Baru"
        subtitle="Konfigurasi endpoint upstream untuk inferensi LLM"
      >
        <form onSubmit={handleCreateProvider} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Identifier (Unik)</label>
            <input
              type="text"
              required
              placeholder="openai-primary"
              value={newProv.name}
              onChange={(e) => setNewProv({ ...newProv, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Tampilan</label>
            <input
              type="text"
              required
              placeholder="OpenAI Global"
              value={newProv.display_name}
              onChange={(e) => setNewProv({ ...newProv, display_name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Jenis Adaptor</label>
            <select
              value={newProv.kind}
              onChange={(e) => setNewProv({ ...newProv, kind: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            >
              <option value="openai">OpenAI</option>
              <option value="anthropic">Anthropic</option>
              <option value="google">Google Gemini</option>
              <option value="openai-compatible">OpenAI Compatible (vLLM / Ollama)</option>
              <option value="custom">Custom HTTP</option>
            </select>
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Base URL</label>
            <input
              type="url"
              required
              value={newProv.base_url}
              onChange={(e) => setNewProv({ ...newProv, base_url: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Prioritas</label>
              <input
                type="number"
                value={newProv.priority}
                onChange={(e) => setNewProv({ ...newProv, priority: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              />
            </div>
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Bobot (Weight)</label>
              <input
                type="number"
                value={newProv.weight}
                onChange={(e) => setNewProv({ ...newProv, weight: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              />
            </div>
          </div>
          <Button type="submit" variant="primary" size="md" className="w-full mt-2">
            Simpan Provider
          </Button>
        </form>
      </Modal>

      {/* Modal Kredensial Provider */}
      {selectedProvider && (
        <Modal
          isOpen={isCredModalOpen}
          onClose={() => setIsCredModalOpen(false)}
          title={`Kredensial API Key: ${selectedProvider.display_name}`}
          subtitle="Enkripsi amplop AES-256-GCM tingkat record dengan AAD"
        >
          <div className="space-y-4">
            <form onSubmit={handleAddCredential} className="p-3 bg-bg-surface-2 rounded-inner border border-border space-y-3">
              <h4 className="text-xs font-semibold text-white">Tambah Kredensial Baru</h4>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                <input
                  type="text"
                  placeholder="Label (contoh: Prod Key 1)"
                  required
                  value={newCred.label}
                  onChange={(e) => setNewCred({ ...newCred, label: e.target.value })}
                  className="px-2.5 py-1.5 bg-bg-surface border border-border rounded-nav text-xs text-white"
                />
                <input
                  type="password"
                  placeholder="API Key / Token Rahasia"
                  required
                  value={newCred.api_key}
                  onChange={(e) => setNewCred({ ...newCred, api_key: e.target.value })}
                  className="px-2.5 py-1.5 bg-bg-surface border border-border rounded-nav text-xs text-white"
                />
              </div>
              <Button type="submit" variant="primary" size="sm" className="w-full">
                Enkripsi & Simpan Kredensial
              </Button>
            </form>

            <div className="space-y-2">
              <h4 className="text-xs font-semibold text-text-muted uppercase">Daftar Kredensial Aktif</h4>
              {credentials.length === 0 ? (
                <p className="text-xs text-text-muted italic">Belum ada kredensial yang tersimpan.</p>
              ) : (
                credentials.map((c) => (
                  <div key={c.id} className="p-3 bg-bg-surface-2/60 rounded-inner border border-border flex items-center justify-between text-xs">
                    <div>
                      <div className="font-semibold text-white">{c.label}</div>
                      <div className="font-mono text-[11px] text-accent mt-0.5">{c.masked_hint}</div>
                    </div>
                    <button
                      onClick={() => handleDeleteCredential(c.id)}
                      className="p-1 text-text-muted hover:text-status-error transition-colors"
                      title="Hapus Kredensial"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                ))
              )}
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
};
