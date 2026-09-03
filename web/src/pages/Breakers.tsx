import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { CircuitBreakerStatus } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { ZapOff, RotateCcw, RefreshCw } from 'lucide-react';

export const Breakers: React.FC = () => {
  const [breakers, setBreakers] = useState<CircuitBreakerStatus[]>([]);
  const [isLoading, setIsLoading] = useState(false);

  const loadBreakers = async () => {
    setIsLoading(true);
    try {
      const res = await api.breakers.list();
      setBreakers(res.items || []);
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadBreakers();
  }, []);

  const handleReset = async (providerId: string, model: string) => {
    try {
      await api.breakers.reset(providerId, model);
      loadBreakers();
    } catch (err) {
      alert('Gagal mereset circuit breaker: ' + err);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Circuit Breakers</h2>
          <p className="text-xs text-text-secondary mt-1">
            Status pemutus arus terdistribusi (Redis) untuk isolasi kegagalan upstream secara instan.
          </p>
        </div>
        <Button variant="secondary" size="sm" onClick={loadBreakers} isLoading={isLoading}>
          <RefreshCw className="w-3.5 h-3.5" />
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {breakers.length === 0 ? (
          <p className="text-xs text-text-muted py-8 text-center col-span-3">
            Semua pemutus arus dalam kondisi normal (Closed).
          </p>
        ) : (
          breakers.map((b, idx) => (
            <Card key={idx} className="p-5 flex flex-col justify-between">
              <div>
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div className="w-9 h-9 rounded-full bg-accent/10 text-accent flex items-center justify-center">
                      <ZapOff className="w-5 h-5" />
                    </div>
                    <div>
                      <h4 className="text-sm font-bold text-white font-mono">{b.model || 'All Models'}</h4>
                      <span className="text-[11px] text-text-muted font-mono">{b.provider_id}</span>
                    </div>
                  </div>
                  <Badge variant={b.state === 'open' ? 'error' : b.state === 'half-open' ? 'warn' : 'success'}>
                    {b.state.toUpperCase()}
                  </Badge>
                </div>

                <div className="mt-4 space-y-2 text-xs">
                  <div className="flex justify-between py-1 border-b border-border/40">
                    <span className="text-text-muted">Kegagalan Beruntun</span>
                    <span className="font-mono text-white font-bold">{b.failure_count}</span>
                  </div>
                  <div className="flex justify-between py-1">
                    <span className="text-text-muted">Kegagalan Terakhir</span>
                    <span className="font-mono text-text-secondary">{b.last_failure_at ? new Date(b.last_failure_at).toLocaleTimeString() : '-'}</span>
                  </div>
                </div>
              </div>

              <div className="mt-5 pt-3 border-t border-border flex justify-end">
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => handleReset(b.provider_id, b.model)}
                  icon={<RotateCcw className="w-3.5 h-3.5" />}
                >
                  Reset Status
                </Button>
              </div>
            </Card>
          ))
        )}
      </div>
    </div>
  );
};
