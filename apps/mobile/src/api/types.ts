export interface MetricTrend {
  label: string;
  value: string;
  trend: string;
  isPositive: boolean;
  sparklineColor: string;
  points: number[];
}

export interface SystemOverview {
  totalRequests: MetricTrend;
  activeProviders: MetricTrend;
  totalUsers: MetricTrend;
  budgetUsage: MetricTrend;
}

export interface ActivityItem {
  id: string;
  title: string;
  method: 'GET' | 'POST' | 'PUT' | 'DELETE';
  endpoint: string;
  timestamp: string;
  status: 'success' | 'warning' | 'error';
  latencyMs: number;
}

export interface ProviderItem {
  id: string;
  name: string;
  brand: 'openai' | 'anthropic' | 'google' | 'meta' | 'mistral' | 'deepseek';
  category: string;
  active: boolean;
  modelCount: number;
  priority: number;
  latencyMs: number;
}

export interface RoutingStrategyInfo {
  id: string;
  name: string;
  description: string;
  current: boolean;
}

export interface ApiKeyItem {
  id: string;
  name: string;
  keyMasked: string;
  active: boolean;
  createdAt: string;
  lastUsed: string;
}

export interface UserItem {
  id: string;
  name: string;
  email: string;
  role: 'Admin' | 'Operator' | 'Developer' | 'User';
  active: boolean;
  initial: string;
  avatarColor: string;
}
