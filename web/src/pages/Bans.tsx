import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Ban } from '../types';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { Ban as BanIcon, Plus, Unlock } from 'lucide-react';

export const Bans: React.FC = () => {
  const [bans, setBans] = useState<Ban[]>([]);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [newBan, setNewBan] = useState({
    subject_kind: 'ip',
    subject: '',
    reason: '',
  });

  const loadBans = async () => {
    try {
      const res = await api.bans.list();
      setBans(res.items || []);
    } catch (err) {
      console.error(err);
    }
  };

  useEffect(() => {
    loadBans();
  }, []);

  const handleLift = async (id: string) => {
    if (!confirm('Cabut pemblokiran subjek ini?')) return;
    try {
      await api.bans.lift(id);
      loadBans();
    } catch (err) {
      alert('Gagal mencabut blokir: ' + err);
    }
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.bans.create(newBan);
      setIsCreateOpen(false);
      loadBans();
    } catch (err) {
      alert('Gagal memblokir: ' + err);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Active Bans</h2>
          <p className="text-xs text-text-secondary mt-1">
            Daftar pemblokiran aktif subjek (IP atau Kunci API) dengan penolakan segera di lapisan gateway.
          </p>
        </div>
        <Button
          variant="danger"
          size="sm"
          onClick={() => setIsCreateOpen(true)}
          icon={<Plus className="w-4 h-4" />}
        >
          Blokir Subjek
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {bans.map((b) => (
          <Card key={b.id || b.subject} className="p-5 flex flex-col justify-between border-status-error/30">
            <div>
              <div className="flex items-center gap-3">
                <div className="w-9 h-9 rounded-full bg-status-error/10 border border-status-error/20 text-status-error flex items-center justify-center">
                  <BanIcon className="w-5 h-5" />
                </div>
                <div>
                  <h4 className="text-sm font-bold text-white font-mono">{b.subject}</h4>
                  <span className="text-[11px] text-text-muted uppercase font-mono">{b.subject_kind}</span>
                </div>
              </div>

              <div className="mt-4 p-3 bg-bg-surface-2 rounded-inner border border-border text-xs">
                <span className="text-text-muted block mb-1">Alasan Pemblokiran:</span>
                <p className="text-text-primary">{b.reason || '(tanpa alasan tertulis)'}</p>
              </div>
            </div>

            <div className="mt-5 pt-3 border-t border-border flex justify-end">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => handleLift(b.id || b.subject)}
                icon={<Unlock className="w-3.5 h-3.5" />}
              >
                Cabut Blokir
              </Button>
            </div>
          </Card>
        ))}
      </div>

      <Modal
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Blokir Subjek Secara Permanen/Sementara"
        subtitle="Permintaan dari subjek ini akan langsung ditolak HTTP 403"
      >
        <form onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Jenis Subjek</label>
            <select
              value={newBan.subject_kind}
              onChange={(e) => setNewBan({ ...newBan, subject_kind: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            >
              <option value="ip">Alamat IP Klien</option>
              <option value="api_key">ID Kunci API</option>
            </select>
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Subjek (IP / ID Key)</label>
            <input
              type="text"
              required
              placeholder="192.168.1.50 atau uuid"
              value={newBan.subject}
              onChange={(e) => setNewBan({ ...newBan, subject: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
            />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Alasan Pemblokiran</label>
            <textarea
              required
              rows={3}
              placeholder="Aktivitas mencurigakan atau serangan brute-force"
              value={newBan.reason}
              onChange={(e) => setNewBan({ ...newBan, reason: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <Button type="submit" variant="danger" size="md" className="w-full mt-2">
            Blokir Sekarang
          </Button>
        </form>
      </Modal>
    </div>
  );
};
