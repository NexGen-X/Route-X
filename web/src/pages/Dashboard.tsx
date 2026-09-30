import React, { useState } from 'react';
import { useQueryClient, useIsFetching } from '@tanstack/react-query';
import { useToast } from '../context/ToastContext';
import {
  DashboardHero,
  QuickStartGuide,
  SystemTelemetrySection,
  PerformanceMetricsRibbon,
  TrafficChart,
  LiveRequestsFeed,
  ProviderHealthMatrix,
} from '../components/dashboard';
import type { TimeWindow } from '../components/dashboard';
import { ChevronDown } from 'lucide-react';

export interface DashboardProps {
  onNavigate: (path: string) => void;
}

export const Dashboard: React.FC<DashboardProps> = ({ onNavigate }) => {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [timeWindow, setTimeWindow] = useState<TimeWindow>('24h');
  const [isRefreshing, setIsRefreshing] = useState(false);
  const isFetching = useIsFetching() > 0;

  const [quickStartDismissed, setQuickStartDismissed] = useState<boolean>(
    () => localStorage.getItem('routex-quickstart-dismissed') === '1'
  );
  const [telemetryExpanded, setTelemetryExpanded] = useState<boolean>(false);

  const handleRefresh = async () => {
    setIsRefreshing(true);
    try {
      await queryClient.invalidateQueries();
      toast.success('Metrik telemetri dashboard berhasil disegarkan');
    } catch (err) {
      toast.error('Gagal menyegarkan data: ' + String(err));
    } finally {
      setIsRefreshing(false);
    }
  };

  const handleDismissQuickStart = () => {
    localStorage.setItem('routex-quickstart-dismissed', '1');
    setQuickStartDismissed(true);
  };

  return (
    <div className="space-y-6 pb-12 animate-fade-in">
      <DashboardHero
        isLoading={isFetching && !isRefreshing}
        isRefreshing={isRefreshing}
        onRefresh={handleRefresh}
        onNavigate={onNavigate}
      />

      {!quickStartDismissed && (
        <QuickStartGuide onNavigate={onNavigate} onDismiss={handleDismissQuickStart} />
      )}

      <PerformanceMetricsRibbon timeWindow={timeWindow} />

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 sm:gap-6">
        <TrafficChart
          timeWindow={timeWindow}
          setTimeWindow={setTimeWindow}
        />
        <LiveRequestsFeed onNavigate={onNavigate} />
      </div>

      <ProviderHealthMatrix onNavigate={onNavigate} />

      <div className="border border-border rounded-card overflow-hidden">
        <button
          type="button"
          onClick={() => setTelemetryExpanded((v) => !v)}
          className="w-full flex items-center justify-between px-5 py-3 bg-bg-surface hover:bg-bg-surface-2 transition-colors text-left cursor-pointer"
          aria-expanded={telemetryExpanded}
        >
          <span className="text-xs font-medium text-text-secondary">Runtime Telemetri Gateway</span>
          <ChevronDown
            className={`w-4 h-4 text-text-muted transition-transform duration-200 ${
              telemetryExpanded ? 'rotate-180' : ''
            }`}
          />
        </button>
        {telemetryExpanded && (
          <div className="p-5 border-t border-border bg-bg-surface">
            <SystemTelemetrySection />
          </div>
        )}
      </div>
    </div>
  );
};
