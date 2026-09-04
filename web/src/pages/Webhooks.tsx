import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Webhook, WebhookDelivery } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { Webhook as WebhookIcon, Plus, Send, Trash2, List } from 'lucide-react';

export const Webhooks: React.FC = () => {
  const [webhooks, setWebhooks] = useState<Webhook[]>([]);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [deliveries, setDeliveries] = useState<WebhookDelivery[]>([]);
  const [selectedWebhook, setSelectedWebhook] = useState<Webhook | null>(null);
  const [isDeliveriesOpen, setIsDeliveriesOpen] = useState(false);
  const [testResult, setTestResult] = useState<any | null>(null);
  const [isTesting, setIsTesting] = useState(false);

  const [newWh, setNewWh] = useState({
    name: '',
    url: '',
    secret: '',
    events: ['provider.unhealthy', 'budget.threshold', 'circuit.opened'],
  });

  const loadWebhooks = async () => {
    try {
      const res = await api.webhooks.list();
      setWebhooks(res.items || []);
    } catch (err) {
      console.error(err);
    }
  };

  useEffect(() => {
    loadWebhooks();
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.webhooks.create(newWh);
      setIsCreateOpen(false);
      loadWebhooks();
    } catch (err) {
      alert('Gagal membuat webhook: ' + err);
    }
  };

  const handleTest = async (w: Webhook) => {
    setIsTesting(true);
    setTestResult(null);
    try {
      const res = await api.webhooks.test(w.id);
      setTestResult({ webhookId: w.id, ...res });
    } catch (err: any) {
      setTestResult({ webhookId: w.id, status: 500, response: err.message });
    } finally {
      setIsTesting(false);
    }
  };

  const handleViewDeliveries = async (w: Webhook) => {
    setSelectedWebhook(w);
    setIsDeliveriesOpen(true);
    try {
      const res = await api.webhooks.deliveries(w.id);
      setDeliveries(res.items || []);
    } catch (err) {
      console.error(err);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Hapus webhook ini?')) return;
    try {
      await api.webhooks.delete(id);
      loadWebhooks();
    } catch (err) {
      alert('Gagal menghapus: ' + err);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Webhooks & Automation</h2>
          <p className="text-xs text-text-secondary mt-1">
            Notifikasi peristiwa sistem (HMAC-SHA256, exponential backoff, pengiriman asinkron) ke endpoint pelanggan.
          </p>
        </div>
        <Button
          variant="primary"
          size="sm"
          onClick={() => setIsCreateOpen(true)}
          icon={<Plus className="w-4 h-4" />}
        >
          Daftarkan Webhook
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {webhooks.map((w) => (
          <Card key={w.id} className="p-5 flex flex-col justify-between">
            <div>
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-9 h-9 rounded-full bg-accent/10 text-accent flex items-center justify-center">
                    <WebhookIcon className="w-5 h-5" />
                  </div>
                  <div>
                    <h4 className="text-sm font-bold text-white">{w.name}</h4>
                    <span className="text-[11px] text-text-muted font-mono truncate max-w-[170px] block" title={w.url}>
                      {w.url}
                    </span>
                  </div>
                </div>
                <Badge variant={w.enabled ? 'success' : 'neutral'}>
                  {w.enabled ? 'active' : 'disabled'}
                </Badge>
              </div>

              <div className="mt-4 space-y-2 text-xs">
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">HMAC Secret</span>
                  <span className="font-mono text-accent">{w.masked_hint || '[ENCRYPTED]'}</span>
                </div>
                <div className="py-1">
                  <span className="text-text-muted block text-[10px] uppercase mb-1">Events Terdaftar</span>
                  <div className="flex flex-wrap gap-1">
                    {w.events?.map((ev) => (
                      <span key={ev} className="px-1.5 py-0.5 text-[10px] font-mono rounded bg-bg-surface-2 text-text-secondary">
                        {ev}
                      </span>
                    ))}
                  </div>
                </div>
              </div>

              {testResult && testResult.webhookId === w.id && (
                <div className="mt-3 p-2 rounded-inner bg-bg-surface-2 border border-border text-[11px] font-mono">
                  Status: {testResult.status} ({testResult.duration_ms ?? 0} ms)
                </div>
              )}
            </div>

            <div className="mt-5 pt-3 border-t border-border flex items-center justify-between">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => handleViewDeliveries(w)}
                icon={<List className="w-3.5 h-3.5" />}
              >
                Log
              </Button>
              <div className="flex gap-2">
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => handleTest(w)}
                  isLoading={isTesting}
                  icon={<Send className="w-3.5 h-3.5" />}
                >
                  Ping Test
                </Button>
                <Button
                  variant="danger"
                  size="sm"
                  onClick={() => handleDelete(w.id)}
                  icon={<Trash2 className="w-3.5 h-3.5" />}
                >
                  Hapus
                </Button>
              </div>
            </div>
          </Card>
        ))}
      </div>

      <Modal
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Daftarkan Endpoint Webhook Baru"
        subtitle="Payload akan ditandatangani dengan tanda tangan HMAC-SHA256"
      >
        <form onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Webhook</label>
            <input
              type="text"
              required
              placeholder="Slack Alert Gateway"
              value={newWh.name}
              onChange={(e) => setNewWh({ ...newWh, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">URL Endpoint Target</label>
            <input
              type="url"
              required
              placeholder="https://api.example.com/webhooks/route-x"
              value={newWh.url}
              onChange={(e) => setNewWh({ ...newWh, url: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
            />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Secret Key (Opsional)</label>
            <input
              type="password"
              placeholder="Kosongkan untuk pembuatan otomatis"
              value={newWh.secret}
              onChange={(e) => setNewWh({ ...newWh, secret: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
            />
          </div>
          <Button type="submit" variant="primary" size="md" className="w-full mt-2">
            Simpan Webhook
          </Button>
        </form>
      </Modal>

      {/* Deliveries Modal */}
      {selectedWebhook && (
        <Modal
          isOpen={isDeliveriesOpen}
          onClose={() => setIsDeliveriesOpen(false)}
          title={`Riwayat Pengiriman: ${selectedWebhook.name}`}
          subtitle="Log percobaan pengiriman webhook"
          maxWidth="xl"
        >
          <div className="space-y-2">
            {deliveries.length === 0 ? (
              <p className="text-xs text-text-muted py-6 text-center">Belum ada catatan pengiriman event.</p>
            ) : (
              deliveries.map((d) => (
                <div key={d.id} className="p-3 bg-bg-surface-2 rounded-inner border border-border flex items-center justify-between text-xs">
                  <div>
                    <span className="font-mono font-semibold text-white">{d.event}</span>
                    <div className="text-[11px] text-text-muted font-mono mt-0.5">
                      Percobaan ke-{d.attempt_count} • {new Date(d.created_at).toLocaleTimeString()}
                    </div>
                  </div>
                  <Badge variant={d.status === 'delivered' ? 'success' : 'warn'}>
                    {(d.status || 'pending').toUpperCase()} ({d.response_status_code || 0})
                  </Badge>
                </div>
              ))
            )}
          </div>
        </Modal>
      )}
    </div>
  );
};
