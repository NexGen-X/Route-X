import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import { Webhook, WebhookDelivery } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Drawer } from '../components/common/Drawer';
import { Modal } from '../components/common/Modal';
import { Checkbox } from '../components/common/Checkbox';
import { Tooltip } from '../components/common/Tooltip';
import { PageHeader } from '../components/common/PageHeader';
import { Webhook as WebhookIcon, Plus, Trash2, Zap, Play, ChevronRight, Eye, EyeOff } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';
import { getErrorMessage } from '../utils/error';

export const Webhooks: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [webhooks, setWebhooks] = useState<Webhook[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [showSecret, setShowSecret] = useState(false);
  const [newWebhook, setNewWebhook] = useState({ name: '', url: '', events: [] as string[], secret: '', enabled: true });
  
  const [selectedWebhook, setSelectedWebhook] = useState<Webhook | null>(null);
  const [deliveries, setDeliveries] = useState<WebhookDelivery[]>([]);
  const [deliveriesLoading, setDeliveriesLoading] = useState(false);

  const [selectedDeliveryId, setSelectedDeliveryId] = useState<number | null>(null);
  const [deliveryDetails, setDeliveryDetails] = useState<WebhookDelivery | null>(null);
  const [detailsLoading, setDetailsLoading] = useState(false);

  const availableEvents = ["*","request.completed","request.failed","provider.unhealthy","provider.recovered","circuit.opened","circuit.closed","budget.threshold","budget.exceeded","ratelimit.exceeded","content.blocked","api_key.created","api_key.revoked","ban.created"];

  const loadWebhooks = async () => {
    try {
      setLoading(true);
      const res = await api.webhooks.list();
      setWebhooks(res.items || []);
      setError(null);
    } catch (err: unknown) {
      setError(getErrorMessage(err, 'Gagal memuat daftar webhook.'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadWebhooks();
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      if (!newWebhook.name || !newWebhook.url) {
        toast.error('Nama dan URL webhook wajib diisi.');
        return;
      }
      await api.webhooks.create(newWebhook);
      toast.success('Webhook baru berhasil didaftarkan.');
      setIsCreateOpen(false);
      setNewWebhook({ name: '', url: '', events: [], secret: '', enabled: true });
      void loadWebhooks();
    } catch (err: unknown) {
      toast.error(getErrorMessage(err, 'Gagal mendaftarkan webhook.'));
    }
  };

  const handleToggle = async (wh: Webhook) => {
    try {
      await api.webhooks.toggle(wh.id, !wh.enabled);
      toast.success(!wh.enabled ? 'Webhook diaktifkan.' : 'Webhook dinonaktifkan.');
      void loadWebhooks();
    } catch (err: unknown) {
      toast.error(getErrorMessage(err, 'Gagal mengubah status webhook.'));
    }
  };

  const handleDelete = async (wh: Webhook) => {
    const ok = await confirmModal({
      title: 'Hapus Webhook?',
      message: `Anda yakin ingin menghapus webhook "${wh.name}"?`,
      confirmText: 'Hapus',
      danger: true,
    });
    if (!ok) return;
    try {
      await api.webhooks.delete(wh.id);
      toast.success('Webhook berhasil dihapus.');
      void loadWebhooks();
      if (selectedWebhook?.id === wh.id) {
        setSelectedWebhook(null);
      }
    } catch (err: unknown) {
      toast.error(getErrorMessage(err, 'Gagal menghapus webhook.'));
    }
  };

  const handleTest = async (wh: Webhook) => {
    try {
      const res = await api.webhooks.test(wh.id);
      toast.success(`Uji coba webhook terkirim. Status: ${res.status}, Durasi: ${res.duration_ms}ms`);
      // Reload deliveries if currently viewing this webhook
      if (selectedWebhook?.id === wh.id) {
        loadDeliveries(wh.id);
      }
    } catch (err: unknown) {
      toast.error(getErrorMessage(err, 'Gagal menguji webhook.'));
    }
  };

  const loadDeliveries = async (id: string) => {
    setDeliveriesLoading(true);
    try {
      const res = await api.webhooks.getDeliveries(id);
      setDeliveries(res.items || []);
    } catch (err: unknown) {
      toast.error(getErrorMessage(err, 'Gagal memuat riwayat pengiriman.'));
    } finally {
      setDeliveriesLoading(false);
    }
  };

  const openWebhookDeliveries = (wh: Webhook) => {
    setSelectedWebhook(wh);
    void loadDeliveries(wh.id);
  };

  const loadDeliveryDetails = async (deliveryId: number) => {
    setDetailsLoading(true);
    setSelectedDeliveryId(deliveryId);
    try {
      const res = await api.webhooks.getDelivery(deliveryId.toString());
      setDeliveryDetails(res);
    } catch (err: unknown) {
      toast.error(getErrorMessage(err, 'Gagal memuat detail pengiriman.'));
    } finally {
      setDetailsLoading(false);
    }
  };

  const toggleEvent = (ev: string) => {
    if (newWebhook.events.includes(ev)) {
      setNewWebhook({ ...newWebhook, events: newWebhook.events.filter(e => e !== ev) });
    } else {
      setNewWebhook({ ...newWebhook, events: [...newWebhook.events, ev] });
    }
  };

  if (error) {
    return (
      <div className="p-4 sm:p-8">
        <QueryError message={error} onRetry={loadWebhooks} />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Webhooks"
        actions={
          <Button variant="primary" size="sm" onClick={() => setIsCreateOpen(true)} icon={<Plus className="w-4 h-4" />}>
            Buat Webhook
          </Button>
        }
      />

      {loading && webhooks.length === 0 ? (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {[1, 2].map((i) => (
            <Card key={i} className="p-5 space-y-4 animate-pulse">
              <div className="space-y-2">
                <div className="h-4 bg-bg-surface-2 rounded w-1/3" />
                <div className="h-3 bg-bg-surface-2 rounded w-2/3" />
              </div>
              <div className="flex gap-2">
                <div className="h-5 w-16 bg-bg-surface-2 rounded" />
                <div className="h-5 w-20 bg-bg-surface-2 rounded" />
              </div>
            </Card>
          ))}
        </div>
      ) : webhooks.length === 0 ? (
        <Card className="py-12 px-6 text-center">
          <div className="max-w-md mx-auto space-y-4">
            <div className="w-12 h-12 rounded-2xl bg-accent/10 border border-accent/20 text-accent flex items-center justify-center mx-auto shadow-inner">
              <WebhookIcon className="w-6 h-6" />
            </div>
            <div>
              <h3 className="text-base font-bold text-white">Belum Ada Webhook Terkonfigurasi</h3>
              <p className="text-xs text-text-muted mt-1 leading-relaxed">
                Kirim event real-time (failover provider, pelanggaran kuota, perubahan kredensial) ke endpoint HTTP/HTTPS eksternal secara otomatis.
              </p>
            </div>
            <Button
              variant="primary"
              size="sm"
              onClick={() => setIsCreateOpen(true)}
              icon={<Plus className="w-4 h-4 text-black" />}
            >
              Buat Webhook Pertama
            </Button>
          </div>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {webhooks.map((wh) => (
            <div key={wh.id} onClick={() => openWebhookDeliveries(wh)} className="cursor-pointer group"><Card className="flex flex-col relative hover:border-accent/30 transition-colors h-full">
              <div className="flex justify-between items-start mb-2">
                <div>
                  <h3 className="font-semibold text-white flex items-center gap-2">
                    {wh.name}
                    {!wh.enabled && <Badge variant="warn">Disabled</Badge>}
                  </h3>
                  <p className="text-[11px] text-text-muted font-mono mt-1 break-all">{wh.url}</p>
                </div>
                <div onClick={(e) => e.stopPropagation()}>
                  <button
                    type="button"
                    onClick={() => handleToggle(wh)}
                    className="p-2 rounded-md text-text-muted hover:text-white hover:bg-bg-surface-2 transition-colors cursor-pointer min-w-[36px] min-h-[36px] flex items-center justify-center"
                    title={wh.enabled ? 'Nonaktifkan webhook' : 'Aktifkan webhook'}
                    aria-label={wh.enabled ? `Nonaktifkan webhook ${wh.name}` : `Aktifkan webhook ${wh.name}`}
                  >
                    <Zap className={`w-4 h-4 ${wh.enabled ? 'text-green-400' : ''}`} aria-hidden="true" />
                  </button>
                </div>
              </div>
              
              <div className="flex-1">
                <div className="flex flex-wrap gap-1 mt-2">
                  {wh.events.map(ev => (
                    <Badge key={ev} variant="neutral">{ev}</Badge>
                  ))}
                  {wh.events.length === 0 && <span className="text-[10px] text-text-muted">Semua Event</span>}
                </div>
              </div>

              <div className="mt-4 pt-3 border-t border-border flex justify-between items-center" onClick={(e) => e.stopPropagation()}>
                <div className="text-[11px] text-text-muted flex items-center gap-2">
                  {wh.last_delivery_at ? (
                    <Tooltip content={`Waktu: ${new Date(wh.last_delivery_at).toLocaleString()}`} position="top">
                      <span className="cursor-default">
                        Terakhir: {wh.last_delivery_status || 'Belum pernah'}
                      </span>
                    </Tooltip>
                  ) : (
                    <span>
                      Terakhir: {wh.last_delivery_status || 'Belum pernah'}
                    </span>
                  )}
                </div>
                <div className="flex gap-2">
                  <Button variant="secondary" size="sm" onClick={() => handleTest(wh)} icon={<Play className="w-3.5 h-3.5" />}>
                    Test
                  </Button>
                  <Button variant="secondary" size="sm" onClick={() => handleDelete(wh)} icon={<Trash2 className="w-3.5 h-3.5" />}>
                    Hapus
                  </Button>
                </div>
              </div>
            </Card></div>
          ))}
        </div>
      )}

      {/* Create Drawer */}
      <Drawer
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Tambah Webhook"
        footer={
          <>
            <Button variant="ghost" onClick={() => setIsCreateOpen(false)}>
              Batal
            </Button>
            <Button variant="primary" type="submit" form="create-webhook-form">
              Simpan Webhook
            </Button>
          </>
        }
      >
        <form id="create-webhook-form" noValidate onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label htmlFor="wh-name" className="block text-xs font-medium text-text-secondary mb-1.5">Nama *</label>
            <input
              id="wh-name"
              type="text"
              required
              value={newWebhook.name}
              onChange={(e) => setNewWebhook({ ...newWebhook, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
              placeholder="Contoh: Slack Alerts"
            />
          </div>
          <div>
            <label htmlFor="wh-url" className="block text-xs font-medium text-text-secondary mb-1.5">URL Endpoint *</label>
            <input
              id="wh-url"
              type="url"
              required
              value={newWebhook.url}
              onChange={(e) => setNewWebhook({ ...newWebhook, url: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
              placeholder="https://..."
            />
          </div>
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <label htmlFor="wh-secret" className="text-xs font-medium text-text-secondary">Secret (Opsional)</label>
              <button
                type="button"
                onClick={() => setShowSecret(!showSecret)}
                aria-label={showSecret ? "Sembunyikan secret webhook" : "Tampilkan secret webhook"}
                className="text-[11px] text-text-muted hover:text-white flex items-center gap-1 focus:outline-none cursor-pointer"
              >
                {showSecret ? (
                  <>
                    <EyeOff className="w-3 h-3" aria-hidden="true" />
                    <span>Sembunyikan</span>
                  </>
                ) : (
                  <>
                    <Eye className="w-3 h-3" aria-hidden="true" />
                    <span>Tampilkan</span>
                  </>
                )}
              </button>
            </div>
            <input
              id="wh-secret"
              type={showSecret ? 'text' : 'password'}
              value={newWebhook.secret}
              onChange={(e) => setNewWebhook({ ...newWebhook, secret: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
              placeholder="Untuk validasi signature (X-Webhook-Signature)"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Event Subscriptions</label>
            <div className="space-y-2 mt-2">
              {availableEvents.map(ev => (
                <div key={ev}>
                  <Checkbox
                    checked={newWebhook.events.includes(ev)}
                    onChange={() => toggleEvent(ev)}
                    label={ev}
                  />
                </div>
              ))}
            </div>
            <p className="mt-1.5 text-text-muted">Biarkan kosong untuk berlangganan semua event.</p>
          </div>
        </form>
      </Drawer>

      {/* Deliveries Drawer */}
      <Drawer
        isOpen={selectedWebhook !== null}
        onClose={() => setSelectedWebhook(null)}
        title="Webhook Deliveries"
        maxWidth="lg"
        footer={
          <Button variant="ghost" onClick={() => setSelectedWebhook(null)}>
            Tutup
          </Button>
        }
      >
        {deliveriesLoading ? (
          <p className="text-xs text-text-muted py-6 text-center">Memuat riwayat pengiriman...</p>
        ) : (
          <div className="space-y-4">
            {deliveries.length === 0 ? (
              <p className="text-xs text-text-muted py-6 text-center">Belum ada riwayat pengiriman.</p>
            ) : (
              <div className="space-y-2">
                {deliveries.map(d => (
                  <div key={d.id} className="flex items-center justify-between p-3 bg-bg-surface-2 border border-border rounded-nav cursor-pointer hover:border-accent/30 transition-colors" onClick={() => loadDeliveryDetails(d.id)}>
                    <div>
                      <div className="flex items-center gap-2">
                        <Badge variant={d.status === 'success' ? 'success' : d.status === 'failed' ? 'error' : 'warn'}>{d.status}</Badge>
                        <span className="text-xs text-white font-mono">{d.event}</span>
                      </div>
                      <div className="text-[10px] text-text-muted mt-1">
                        {new Date(d.created_at).toLocaleString()} • Att: {d.attempt_count} • HTTP {d.response_status_code || '-'}
                      </div>
                    </div>
                    <ChevronRight className="w-4 h-4 text-text-muted" />
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </Drawer>

      {/* Delivery Details Modal */}
      <Modal
        isOpen={selectedDeliveryId !== null}
        onClose={() => setSelectedDeliveryId(null)}
        title="Delivery Details"
        maxWidth="lg"
        footer={
          <Button variant="ghost" onClick={() => setSelectedDeliveryId(null)}>
            Tutup
          </Button>
        }
      >
        <div className="space-y-4 text-xs">
          {detailsLoading ? (
            <p className="text-center text-text-muted py-4">Memuat detail...</p>
          ) : deliveryDetails ? (
            <>
              <div className="grid grid-cols-2 gap-4">
                <div className="bg-bg-surface-2 p-3 rounded-nav border border-border">
                  <span className="block text-xs font-medium text-text-secondary mb-1">Status</span>
                  <Badge variant={deliveryDetails.status === 'success' ? 'success' : deliveryDetails.status === 'failed' ? 'error' : 'warn'}>{deliveryDetails.status}</Badge>
                </div>
                <div className="bg-bg-surface-2 p-3 rounded-nav border border-border">
                  <span className="block text-xs font-medium text-text-secondary mb-1">Response Code</span>
                  <span className="text-white font-mono">{deliveryDetails.response_status_code || 'N/A'}</span>
                </div>
              </div>
              
              <div>
                <h3 className="font-semibold text-white mb-2">Request Payload</h3>
                <pre className="bg-bg-surface-2 border border-border p-3 rounded-nav overflow-x-auto text-[11px] text-blue-300 font-mono whitespace-pre-wrap break-all">
                  {deliveryDetails.payload ? JSON.stringify(deliveryDetails.payload, null, 2) : 'No payload available'}
                </pre>
              </div>
              
              <div>
                <h3 className="font-semibold text-white mb-2">Response / Error</h3>
                <pre className="bg-bg-surface-2 border border-border p-3 rounded-nav overflow-x-auto text-[11px] text-red-300 font-mono whitespace-pre-wrap break-all">
                  {deliveryDetails.error_message || 'Success'}
                </pre>
              </div>
            </>
          ) : (
            <p className="text-center text-text-muted py-4">Gagal memuat detail</p>
          )}
        </div>
      </Modal>
    </div>
  );
};
