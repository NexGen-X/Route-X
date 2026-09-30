import { useQuery } from '@tanstack/react-query';
import { api } from '../../api/client';

export type TimeWindow = '1h' | '6h' | '24h' | '7d';
export type MetricType = 'requests' | 'latency' | 'tokens';

export const useSystemOverview = () => {
  return useQuery({
    queryKey: ['systemOverview'],
    queryFn: () => api.system.overview(),
    refetchInterval: 5000,
  });
};

export const useRecentRequests = () => {
  return useQuery({
    queryKey: ['recentRequests'],
    queryFn: () => api.requests.list({ limit: 6 }),
    refetchInterval: 5000,
  });
};

export const useProvidersList = () => {
  return useQuery({
    queryKey: ['providersList'],
    queryFn: () => api.providers.list(),
    refetchInterval: 30000,
  });
};

export const useObservabilitySummary = (timeWindow: TimeWindow) => {
  return useQuery({
    queryKey: ['observabilitySummary', timeWindow],
    queryFn: () => api.observability.summary(timeWindow),
    refetchInterval: 30000,
  });
};

export const useObservabilitySeries = (metricType: MetricType, timeWindow: TimeWindow) => {
  return useQuery({
    queryKey: ['observabilitySeries', metricType, timeWindow],
    queryFn: () => api.observability.series(metricType, timeWindow),
    refetchInterval: 30000,
  });
};
