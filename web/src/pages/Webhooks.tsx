import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import { Webhook, WebhookDelivery } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Drawer } from '../components/common/Drawer';
import { PageHeader } from '../components/common/PageHeader';
import { Webhook as WebhookIcon, Plus, Trash2, Activity, Zap, Play, ChevronRight, X } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';

export const Webhooks: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [webhooks, setWebhooks] = useState<Webhook[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [newWebhook, setNewWebhook] = useState({ name: '', url: '', events: [] as string[], secret: '', enabled: true });
  
  const [selectedWebhook, setSelectedWebhook] = useState<Webhook | null>(null);
  const [deliveries, setDeliveries] = useState<WebhookDelivery[]>([]);
  const [deliveriesLoading, setDeliveriesLoading] = useState(false);

  const [selectedDeliveryId, setSelectedDeliveryId] = useState<number | null>(null);
  const [deliveryDetails, setDeliveryDetails] = useState<any | null>(null);
  const [detailsLoading, setDetailsLoading] = useState(false);

  const availableEvents = ["*","request.completed","request.failed","provider.unhealthy","provider.recovered","circuit.opened","circuit.closed","budget.threshold","budget.exceeded","ratelimit.exceeded","content.blocked","api_key.created","api_key.revoked","ban.created"];

  const loadWebhooks = async () => {
    try {
      setLoading(true);
      const res = await api.webhooks.list();
      setWebhooks(res.items || []);
      setError(null);
    } catch (err: any) {
      setError(err.message || 'Failed to load webhooks');
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
        toast.error('Name and URL are required.');
        return;
      }
      await api.webhooks.create(newWebhook);
      toast.success('Webhook created successfully.');
      setIsCreateOpen(false);
      setNewWebhook({ name: '', url: '', events: [], secret: '', enabled: true });
      void loadWebhooks();
    } catch (err: any) {
      toast.error(err.message || 'Failed to create webhook');
    }
  };

  const handleToggle = async (wh: Webhook) => {
    try {
      await api.webhooks.toggle(wh.id, !wh.enabled);
      toast.success(`Webhook ${!wh.enabled ? 'enabled' : 'disabled'}.`);
      void loadWebhooks();
    } catch (err: any) {
      toast.error(err.message || 'Failed to toggle webhook');
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
      toast.success('Webhook deleted.');
      void loadWebhooks();
      if (selectedWebhook?.id === wh.id) {
        setSelectedWebhook(null);
      }
    } catch (err: any) {
      toast.error(err.message || 'Failed to delete webhook');
    }
  };

  const handleTest = async (wh: Webhook) => {
    try {
      const res = await api.webhooks.test(wh.id);
      toast.success(`Test event sent. Status: ${res.status}, Duration: ${res.duration_ms}ms`);
      // Reload deliveries if currently viewing this webhook
      if (selectedWebhook?.id === wh.id) {
        loadDeliveries(wh.id);
      }
    } catch (err: any) {
      toast.error(err.message || 'Failed to test webhook');
    }
  };

  const loadDeliveries = async (id: string) => {
    setDeliveriesLoading(true);
    try {
      const res = await api.webhooks.getDeliveries(id);
      setDeliveries(res.items || []);
    } catch (err: any) {
      toast.error(err.message || 'Failed to load deliveries');
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
    } catch (err: any) {
      toast.error(err.message || 'Failed to load delivery details');
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
      <div className="p-8">
        <QueryError message={error} onRetry={loadWebhooks} />
      </div>
    );
  }

  return (
    <div className="p-8 pb-20 max-w-7xl mx-auto space-y-6 animate-in fade-in duration-300">
      <PageHeader
        title="Webhooks"
        
        
        actions={
          <Button variant="primary" onClick={() => setIsCreateOpen(true)} icon={<Plus className="w-4 h-4" />}>
            Create Webhook
          </Button>
        }
      />

      {loading && webhooks.length === 0 ? (
        <div className="flex items-center justify-center h-40">
          <Activity className="w-6 h-6 animate-pulse text-text-muted" />
        </div>
      ) : webhooks.length === 0 ? (
        <div className="text-center py-12 bg-bg-surface border border-border rounded-lg text-text-muted">
          <WebhookIcon className="w-12 h-12 mx-auto mb-3 opacity-20" />
          <p>Belum ada webhook yang dikonfigurasi.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {webhooks.map((wh) => (
            <div key={wh.id} onClick={() => openWebhookDeliveries(wh)} className="cursor-pointer group"><Card className="flex flex-col relative hover:border-brand/30 transition-colors h-full">
              <div className="flex justify-between items-start mb-2">
                <div>
                  <h3 className="font-semibold text-white flex items-center gap-2">
                    {wh.name}
                    {!wh.enabled && <Badge variant="warn">Disabled</Badge>}
                  </h3>
                  <p className="text-[11px] text-text-muted font-mono mt-1 break-all">{wh.url}</p>
                </div>
                <div onClick={(e) => e.stopPropagation()}>
                  <button type="button" onClick={() => handleToggle(wh)} className="p-1.5 rounded-md text-text-muted hover:text-white hover:bg-bg-surface transition-colors cursor-pointer" title="Toggle Enabled">
                    <Zap className={`w-4 h-4 ${wh.enabled ? 'text-green-400' : ''}`} />
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
                  <span title={wh.last_delivery_at}>
                    Terakhir: {wh.last_delivery_status || 'Belum pernah'}
                  </span>
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
      <Drawer isOpen={isCreateOpen} onClose={() => setIsCreateOpen(false)} title="Tambah Webhook" subtitle="Kirim notifikasi HTTP saat ada event">
        <form onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Nama</label>
            <input type="text" required value={newWebhook.name} onChange={(e) => setNewWebhook({ ...newWebhook, name: e.target.value })} className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white" placeholder="Contoh: Slack Alerts" />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">URL Endpoint</label>
            <input type="url" required value={newWebhook.url} onChange={(e) => setNewWebhook({ ...newWebhook, url: e.target.value })} className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono" placeholder="https://..." />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Secret (Opsional)</label>
            <input type="password" value={newWebhook.secret} onChange={(e) => setNewWebhook({ ...newWebhook, secret: e.target.value })} className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono" placeholder="Untuk validasi signature (X-Webhook-Signature)" />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Event Subscriptions</label>
            <div className="space-y-2 mt-2">
              {availableEvents.map(ev => (
                <label key={ev} className="flex items-center gap-2 cursor-pointer text-white">
                  <input type="checkbox" checked={newWebhook.events.includes(ev)} onChange={() => toggleEvent(ev)} className="rounded bg-bg-surface-2 border-border" />
                  {ev}
                </label>
              ))}
            </div>
            <p className="mt-1 text-text-muted">Biarkan kosong untuk berlangganan semua event.</p>
          </div>
          <div className="pt-3 flex justify-end gap-2 border-t border-border">
            <Button type="button" variant="secondary" onClick={() => setIsCreateOpen(false)}>Batal</Button>
            <Button type="submit" variant="primary">Simpan Webhook</Button>
          </div>
        </form>
      </Drawer>

      {/* Deliveries Drawer */}
      <Drawer isOpen={selectedWebhook !== null} onClose={() => setSelectedWebhook(null)} title="Webhook Deliveries" subtitle={selectedWebhook?.name || ''} maxWidth="lg">
        {deliveriesLoading ? (
          <p className="text-xs text-text-muted py-6 text-center">Memuat riwayat pengiriman...</p>
        ) : (
          <div className="space-y-4">
            {deliveries.length === 0 ? (
              <p className="text-xs text-text-muted py-6 text-center">Belum ada riwayat pengiriman.</p>
            ) : (
              <div className="space-y-2">
                {deliveries.map(d => (
                  <div key={d.id} className="flex items-center justify-between p-3 bg-bg-surface-2 border border-border rounded-nav cursor-pointer hover:border-brand/30 transition-colors" onClick={() => loadDeliveryDetails(d.id)}>
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

      {/* Delivery Details Modal/Drawer overlay */}
      {selectedDeliveryId !== null && (
        <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className="bg-bg-sidebar border border-border rounded-xl shadow-xl w-full max-w-2xl max-h-[90vh] flex flex-col">
            <div className="px-5 py-4 border-b border-border flex items-center justify-between">
              <div>
                <h2 className="font-semibold text-white">Delivery Details</h2>
                <p className="text-xs text-text-muted">ID: {selectedDeliveryId}</p>
              </div>
              <button onClick={() => setSelectedDeliveryId(null)} className="text-text-muted hover:text-white">
                <X className="w-5 h-5" />
              </button>
            </div>
            
            <div className="p-5 overflow-y-auto space-y-4 text-xs">
              {detailsLoading ? (
                <p className="text-center text-text-muted py-4">Memuat detail...</p>
              ) : deliveryDetails ? (
                <>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="bg-bg-surface-2 p-3 rounded-nav border border-border">
                      <span className="block text-text-secondary uppercase mb-1">Status</span>
                      <Badge variant={deliveryDetails.status === 'success' ? 'success' : deliveryDetails.status === 'failed' ? 'error' : 'warn'}>{deliveryDetails.status}</Badge>
                    </div>
                    <div className="bg-bg-surface-2 p-3 rounded-nav border border-border">
                      <span className="block text-text-secondary uppercase mb-1">Response Code</span>
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
          </div>
        </div>
      )}
    </div>
  );
};
