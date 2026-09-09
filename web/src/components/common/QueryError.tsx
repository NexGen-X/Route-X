import React from 'react';
import { AlertCircle, RefreshCw } from 'lucide-react';
import { Button } from './Button';

interface QueryErrorProps {
  message: string;
  onRetry?: () => void;
}

export const QueryError: React.FC<QueryErrorProps> = ({ message, onRetry }) => (
  <div role="alert" aria-live="assertive" className="p-5 rounded-xl border border-rose-500/30 bg-rose-500/10 text-rose-200 flex items-start justify-between gap-4">
    <div className="flex items-start gap-3 min-w-0">
      <AlertCircle className="w-5 h-5 text-rose-400 flex-shrink-0 mt-0.5" aria-hidden="true" />
      <div>
        <p className="text-sm font-semibold text-white">Data gagal dimuat</p>
        <p className="text-xs mt-1 break-words">{message}</p>
      </div>
    </div>
    {onRetry && (
      <Button variant="secondary" size="sm" onClick={onRetry} icon={<RefreshCw className="w-3.5 h-3.5" />}>
        Coba Lagi
      </Button>
    )}
  </div>
);
