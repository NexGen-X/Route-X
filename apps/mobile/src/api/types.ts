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
  headers?: Record<string, string>;
  payload?: string;
  response?: string;
  providerUsed?: string;
  tokensUsed?: number;
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
  models?: string[];
  baseUrl?: string;
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
  role?: string;
  rateLimit?: number;
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

export interface EgressPoolItem {
  id: string;
  name: string;
  description: string;
  region: string;
  activeIps: number;
  totalIps: number;
  status: 'healthy' | 'degraded';
  tlsVersion: string;
  priority: number;
  active: boolean;
}

export interface BudgetItem {
  id: string;
  name: string;
  allocatedMonthly: number;
  spentMonthly: number;
  percentage: number;
  status: 'normal' | 'warning' | 'exceeded';
  alertThreshold: number;
  currency: string;
}

export interface WebhookItem {
  id: string;
  name: string;
  url: string;
  events: string[];
  active: boolean;
  lastDeliveryStatus: 'success' | 'failed';
  lastDeliveryTime: string;
}

export interface DiagnosticVitals {
  uptime: string;
  cpuUsagePercent: number;
  ramUsageMb: number;
  ramTotalMb: number;
  activeGoroutines: number;
  activeHttpConns: number;
  dbPoolStatus: 'connected' | 'reconnecting';
  dbLatencyMs: number;
  redisStatus: 'connected' | 'warning';
  redisLatencyMs: number;
  egressTlsHealth: 'optimal' | 'warning';
}

export interface GatewaySettings {
  gatewayPort: number;
  defaultTimeoutSec: number;
  maxRequestBodyMb: number;
  rateLimitPerMinute: number;
  enableStrictTls: boolean;
  logRetentionDays: number;
  cacheResponses: boolean;
  corsOrigins: string;
}

export interface ModelCatalogItem {
  id: string;
  name: string;
  alias: string;
  provider: string;
  contextWindow: string;
  inputCostPer1K: number;
  outputCostPer1K: number;
  category: 'Reasoning' | 'Chat & Multimodal' | 'Coding' | 'Fast Embeddings';
  isVisionSupported: boolean;
  active: boolean;
  fallbackModel: string;
}

export interface RateLimitTierItem {
  id: string;
  name: string;
  description: string;
  requestsPerMinute: number;
  tokensPerMinute: number;
  maxConcurrent: number;
  algorithm: 'Token Bucket' | 'Sliding Window' | 'Fixed Window';
  activeKeys: number;
  isDefault: boolean;
}

export interface CustomRoutingRuleItem {
  id: string;
  name: string;
  conditionDescription: string;
  targetProvider: string;
  targetModel: string;
  fallbackProvider: string;
  priority: number;
  active: boolean;
  matchType: 'Header' | 'Prompt Length' | 'User Role' | 'Latency';
}

export interface SecurityProfile {
  adminName: string;
  email: string;
  role: string;
  mfaEnabled: boolean;
  biometricAppLock: boolean;
  activeSessionsCount: number;
  lastLoginIp: string;
  lastLoginTime: string;
}

export interface AuthUser {
  id: string;
  email: string;
  displayName: string;
  status: string;
  roles: string[];
}

export interface LoginResult {
  success: boolean;
  user?: AuthUser;
  token?: string;
  error?: string;
}

