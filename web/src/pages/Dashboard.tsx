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

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 lg:grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 sm:gap-6 min-w-0">
        <TrafficChart
          timeWindow={timeWindow}
          setTimeWindow={setTimeWindow}
        />
        <LiveRequestsFeed onNavigate={onNavigate} />
      </div>

      <ProviderHealthMatrix onNavigate={onNavigate} />

      <div className="bg-bg-surface/50 backdrop-blur-sm border border-white/[0.06] rounded-xl overflow-hidden shadow-sm">
        <button
          type="button"
          onClick={() => setTelemetryExpanded((v) => !v)}
          className="w-full flex items-center justify-between px-5 py-3 bg-white/[0.02] hover:bg-white/[0.05] transition-colors text-left cursor-pointer"
          aria-expanded={telemetryExpanded}
        >
          <span className="text-xs font-medium text-zinc-300">Runtime Telemetri Gateway</span>
          <ChevronDown
            className={`w-4 h-4 text-zinc-500 transition-transform duration-200 ${
              telemetryExpanded ? 'rotate-180' : ''
            }`}
          />
        </button>
        {telemetryExpanded && (
          <div className="p-5 border-t border-white/[0.06] bg-transparent">
            <SystemTelemetrySection />
          </div>
        )}
      </div>
    </div>
  );
};
