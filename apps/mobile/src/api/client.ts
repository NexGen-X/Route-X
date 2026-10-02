import {
  SystemOverview,
  ActivityItem,
  ProviderItem,
  RoutingStrategyInfo,
  ApiKeyItem,
  UserItem,
  EgressPoolItem,
  BudgetItem,
  WebhookItem,
  DiagnosticVitals,
  GatewaySettings,
} from './types';
import { colors } from '../theme/colors';

// Fallback Mock Data offline sesuai spesifikasi Route-X
const MOCK_OVERVIEW: SystemOverview = {
  totalRequests: {
    label: 'Total Requests',
    value: '1,248',
    trend: '+12%',
    isPositive: true,
    sparklineColor: colors.accentPrimary,
    points: [30, 45, 60, 50, 75, 90, 85, 110, 124],
  },
  activeProviders: {
    label: 'Aktif Providers',
    value: '6',
    trend: '+0%',
    isPositive: true,
    sparklineColor: colors.avatarOperator,
    points: [6, 6, 5, 6, 6, 6, 6, 6],
  },
  totalUsers: {
    label: 'Total Users',
    value: '12',
    trend: '+25%',
    isPositive: true,
    sparklineColor: colors.accentLime,
    points: [8, 8, 9, 9, 10, 11, 12],
  },
  budgetUsage: {
    label: 'Budget Usage',
    value: '$1,248.50',
    trend: '+8%',
    isPositive: true,
    sparklineColor: colors.accentPrimary,
    points: [400, 650, 780, 890, 990, 1100, 1248.5],
  },
};

const MOCK_ACTIVITIES: ActivityItem[] = [
  {
    id: 'act-1',
    title: 'Request Berhasil',
    method: 'POST',
    endpoint: '/v1/chat/completions',
    timestamp: '2m lalu',
    status: 'success',
    latencyMs: 142,
    providerUsed: 'OpenAI (gpt-4o)',
    tokensUsed: 842,
    headers: {
      'content-type': 'application/json',
      'authorization': 'Bearer rk-live-••••••••••••94f2',
      'x-routex-model': 'gpt-4o',
    },
    payload: JSON.stringify({ model: 'gpt-4o', messages: [{ role: 'user', content: 'Generate quarterly financial summary.' }] }, null, 2),
    response: JSON.stringify({ id: 'chatcmpl-9x12', object: 'chat.completion', choices: [{ message: { role: 'assistant', content: 'Here is the summary of Q3 operations...' } }] }, null, 2),
  },
  {
    id: 'act-2',
    title: 'Request Berhasil',
    method: 'POST',
    endpoint: '/v1/chat/completions',
    timestamp: '5m lalu',
    status: 'success',
    latencyMs: 185,
    providerUsed: 'Anthropic (claude-3-5-sonnet)',
    tokensUsed: 1250,
    headers: {
      'content-type': 'application/json',
      'authorization': 'Bearer rk-mobi-••••••••••••83b1',
      'x-routex-model': 'claude-3-5-sonnet',
    },
    payload: JSON.stringify({ model: 'claude-3-5-sonnet-20241022', messages: [{ role: 'user', content: 'Refactor mobile navigation stack.' }] }, null, 2),
    response: JSON.stringify({ id: 'msg_98a7', type: 'message', content: [{ type: 'text', text: 'I have refactored the stack...' }] }, null, 2),
  },
  {
    id: 'act-3',
    title: 'Failover Triggered',
    method: 'PUT',
    endpoint: '/api/admin/providers/anthropic',
    timestamp: '12m lalu',
    status: 'warning',
    latencyMs: 88,
    providerUsed: 'Route-X Failover Engine',
    headers: {
      'x-routex-alert': 'high-latency-threshold',
    },
    payload: JSON.stringify({ event: 'latency_exceeded', provider: 'anthropic', action: 'reroute_to_google' }, null, 2),
    response: JSON.stringify({ status: 'rerouted', new_target: 'google-gemini' }, null, 2),
  },
  {
    id: 'act-4',
    title: 'User Baru Didaftarkan',
    method: 'POST',
    endpoint: '/api/admin/users',
    timestamp: '1j lalu',
    status: 'success',
    latencyMs: 64,
    headers: {
      'content-type': 'application/json',
    },
    payload: JSON.stringify({ name: 'Sandbox User', email: 'sandbox@routex.ai', role: 'User' }, null, 2),
    response: JSON.stringify({ success: true, user_id: 'usr-4' }, null, 2),
  },
];

let mockProvidersList: ProviderItem[] = [
  {
    id: 'prov-openai',
    name: 'OpenAI',
    brand: 'openai',
    category: 'LLM & Multimodal',
    active: true,
    modelCount: 8,
    priority: 1,
    latencyMs: 142,
    models: ['gpt-4o', 'gpt-4o-mini', 'o1-preview', 'o1-mini', 'text-embedding-3-large'],
    baseUrl: 'https://api.openai.com/v1',
  },
  {
    id: 'prov-anthropic',
    name: 'Anthropic',
    brand: 'anthropic',
    category: 'Reasoning & Coding',
    active: true,
    modelCount: 5,
    priority: 2,
    latencyMs: 168,
    models: ['claude-3-5-sonnet', 'claude-3-haiku', 'claude-3-opus'],
    baseUrl: 'https://api.anthropic.com/v1',
  },
  {
    id: 'prov-google',
    name: 'Google Gemini',
    brand: 'google',
    category: 'Long-Context & Vision',
    active: true,
    modelCount: 4,
    priority: 3,
    latencyMs: 155,
    models: ['gemini-1.5-pro', 'gemini-1.5-flash', 'gemini-2.0-flash-exp'],
    baseUrl: 'https://generativelanguage.googleapis.com/v1beta',
  },
  {
    id: 'prov-meta',
    name: 'Meta LLaMA',
    brand: 'meta',
    category: 'Open Weights & Local',
    active: true,
    modelCount: 6,
    priority: 4,
    latencyMs: 210,
    models: ['llama-3.1-405b', 'llama-3.1-70b', 'llama-3.2-3b'],
    baseUrl: 'https://api.groq.com/openai/v1',
  },
  {
    id: 'prov-mistral',
    name: 'Mistral AI',
    brand: 'mistral',
    category: 'High-Efficiency',
    active: true,
    modelCount: 3,
    priority: 5,
    latencyMs: 130,
    models: ['mistral-large-2407', 'codestral-latest'],
    baseUrl: 'https://api.mistral.ai/v1',
  },
  {
    id: 'prov-deepseek',
    name: 'DeepSeek',
    brand: 'deepseek',
    category: 'Reasoning & Math',
    active: false,
    modelCount: 2,
    priority: 6,
    latencyMs: 320,
    models: ['deepseek-chat', 'deepseek-coder'],
    baseUrl: 'https://api.deepseek.com/v1',
  },
];

let mockApiKeysList: ApiKeyItem[] = [
  {
    id: 'key-1',
    name: 'Default Production Key',
    keyMasked: 'rk-live-••••••••••••94f2',
    active: true,
    createdAt: '12 Sep 2026',
    lastUsed: 'Baru saja',
    role: 'Full Admin',
    rateLimit: 1200,
  },
  {
    id: 'key-2',
    name: 'Mobile App Companion',
    keyMasked: 'rk-mobi-••••••••••••83b1',
    active: true,
    createdAt: '20 Sep 2026',
    lastUsed: '1m lalu',
    role: 'Mobile Client',
    rateLimit: 600,
  },
  {
    id: 'key-3',
    name: 'Development Sandbox',
    keyMasked: 'rk-test-••••••••••••10c8',
    active: false,
    createdAt: '28 Sep 2026',
    lastUsed: '2 hari lalu',
    role: 'Read Only',
    rateLimit: 100,
  },
  {
    id: 'key-4',
    name: 'Egress Cluster Gateway',
    keyMasked: 'rk-egrs-••••••••••••47e9',
    active: true,
    createdAt: '01 Okt 2026',
    lastUsed: '10m lalu',
    role: 'Internal Service',
    rateLimit: 5000,
  },
];

let mockUsersList: UserItem[] = [
  {
    id: 'usr-1',
    name: 'Administrator',
    email: 'admin@routex.ai',
    role: 'Admin',
    active: true,
    initial: 'A',
    avatarColor: colors.avatarAdmin,
  },
  {
    id: 'usr-2',
    name: 'Operations Lead',
    email: 'ops@routex.ai',
    role: 'Operator',
    active: true,
    initial: 'O',
    avatarColor: colors.avatarOperator,
  },
  {
    id: 'usr-3',
    name: 'Core Developer',
    email: 'dev@routex.ai',
    role: 'Developer',
    active: true,
    initial: 'D',
    avatarColor: colors.avatarDev,
  },
  {
    id: 'usr-4',
    name: 'Sandbox User',
    email: 'sandbox@routex.ai',
    role: 'User',
    active: true,
    initial: 'U',
    avatarColor: colors.avatarUser,
  },
];

let mockEgressPoolsList: EgressPoolItem[] = [
  {
    id: 'egr-1',
    name: 'Primary Outbound Pool',
    description: 'Kluster IP statis utama untuk koneksi provider US & Global.',
    region: 'ap-southeast-1 (Singapore)',
    activeIps: 4,
    totalIps: 4,
    status: 'healthy',
    tlsVersion: 'TLS 1.3 Strict',
    priority: 1,
    active: true,
  },
  {
    id: 'egr-2',
    name: 'Low-Latency Direct Proxy',
    description: 'Dedicated routing tunnel untuk query model berkecepatan tinggi.',
    region: 'us-east-1 (N. Virginia)',
    activeIps: 8,
    totalIps: 8,
    status: 'healthy',
    tlsVersion: 'TLS 1.3',
    priority: 2,
    active: true,
  },
  {
    id: 'egr-3',
    name: 'EU Privacy Compliance Pool',
    description: 'Pool IP kawasan Eropa sesuai kepatuhan regulasi data transfer GDPR.',
    region: 'eu-central-1 (Frankfurt)',
    activeIps: 2,
    totalIps: 2,
    status: 'healthy',
    tlsVersion: 'TLS 1.3 Strict',
    priority: 3,
    active: true,
  },
  {
    id: 'egr-4',
    name: 'Disaster Recovery Standby',
    description: 'Secondary failover tunnel jika terjadi degradasi jaringan cloud.',
    region: 'ap-northeast-1 (Tokyo)',
    activeIps: 2,
    totalIps: 2,
    status: 'degraded',
    tlsVersion: 'TLS 1.2 / 1.3',
    priority: 4,
    active: false,
  },
];

let mockBudgetsList: BudgetItem[] = [
  {
    id: 'bgt-1',
    name: 'Total Anggaran Organisasi',
    allocatedMonthly: 3000,
    spentMonthly: 1248.50,
    percentage: 41.6,
    status: 'normal',
    alertThreshold: 85,
    currency: 'USD',
  },
  {
    id: 'bgt-2',
    name: 'Tim Core AI Research',
    allocatedMonthly: 1500,
    spentMonthly: 890.20,
    percentage: 59.3,
    status: 'normal',
    alertThreshold: 80,
    currency: 'USD',
  },
  {
    id: 'bgt-3',
    name: 'Mobile & Edge Companion App',
    allocatedMonthly: 500,
    spentMonthly: 215.10,
    percentage: 43.0,
    status: 'normal',
    alertThreshold: 75,
    currency: 'USD',
  },
  {
    id: 'bgt-4',
    name: 'Customer Production Gateway',
    allocatedMonthly: 1000,
    spentMonthly: 890.00,
    percentage: 89.0,
    status: 'warning',
    alertThreshold: 80,
    currency: 'USD',
  },
];

let mockWebhooksList: WebhookItem[] = [
  {
    id: 'wh-1',
    name: 'Slack Dev Alert Channel',
    url: 'https://hooks.slack.com/services/T00/B00/X00',
    events: ['provider.failover', 'budget.threshold', 'anomaly.latency'],
    active: true,
    lastDeliveryStatus: 'success',
    lastDeliveryTime: '5m lalu',
  },
  {
    id: 'wh-2',
    name: 'Discord Ops Notification',
    url: 'https://discord.com/api/webhooks/109823/routex',
    events: ['system.restart', 'provider.error'],
    active: true,
    lastDeliveryStatus: 'success',
    lastDeliveryTime: '1j lalu',
  },
  {
    id: 'wh-3',
    name: 'PagerDuty Emergency On-Call',
    url: 'https://events.pagerduty.com/v2/enqueue',
    events: ['provider.all_down', 'security.auth_failure'],
    active: true,
    lastDeliveryStatus: 'success',
    lastDeliveryTime: '3 hari lalu',
  },
];

let mockSettingsData: GatewaySettings = {
  gatewayPort: 8080,
  defaultTimeoutSec: 60,
  maxRequestBodyMb: 10,
  rateLimitPerMinute: 600,
  enableStrictTls: true,
  logRetentionDays: 30,
  cacheResponses: true,
  corsOrigins: '*',
};

const MOCK_DIAGNOSTICS: DiagnosticVitals = {
  uptime: '18 hari, 4 jam 12 menit',
  cpuUsagePercent: 14.8,
  ramUsageMb: 2615,
  ramTotalMb: 3831,
  activeGoroutines: 48,
  activeHttpConns: 12,
  dbPoolStatus: 'connected',
  dbLatencyMs: 1.8,
  redisStatus: 'connected',
  redisLatencyMs: 0.9,
  egressTlsHealth: 'optimal',
};

export class RouteXApiClient {
  private baseUrl: string;

  constructor(baseUrl: string = 'https://13.212.164.108/api/admin') {
    this.baseUrl = baseUrl;
  }

  public async getOverview(): Promise<SystemOverview> {
    try {
      const response = await fetch(`${this.baseUrl}/metrics`);
      if (!response.ok) throw new Error('Network error');
      const data = (await response.json()) as Partial<SystemOverview>;
      return { ...MOCK_OVERVIEW, ...data };
    } catch {
      return MOCK_OVERVIEW;
    }
  }

  public async getActivities(): Promise<ActivityItem[]> {
    try {
      const response = await fetch(`${this.baseUrl}/activities`);
      if (!response.ok) throw new Error('Network error');
      const data = (await response.json()) as ActivityItem[];
      return data;
    } catch {
      return MOCK_ACTIVITIES;
    }
  }

  public async getProviders(): Promise<ProviderItem[]> {
    try {
      const response = await fetch(`${this.baseUrl}/providers`);
      if (!response.ok) throw new Error('Network error');
      const data = (await response.json()) as ProviderItem[];
      return data;
    } catch {
      return mockProvidersList;
    }
  }

  public async addProvider(provider: Omit<ProviderItem, 'id'>): Promise<ProviderItem> {
    const newProvider: ProviderItem = {
      ...provider,
      id: `prov-${Date.now()}`,
    };
    mockProvidersList = [newProvider, ...mockProvidersList];
    return newProvider;
  }

  public async toggleProvider(providerId: string, currentActive: boolean): Promise<boolean> {
    mockProvidersList = mockProvidersList.map((p) =>
      p.id === providerId ? { ...p, active: !currentActive } : p
    );
    return !currentActive;
  }

  public async getRoutingStrategy(): Promise<RoutingStrategyInfo> {
    return {
      id: 'strat-rr',
      name: 'Round Robin',
      description: 'Distribusi request seimbang antar semua provider aktif dengan prioritas setara.',
      current: true,
    };
  }

  public async getApiKeys(): Promise<ApiKeyItem[]> {
    try {
      const response = await fetch(`${this.baseUrl}/keys`);
      if (!response.ok) throw new Error('Network error');
      return (await response.json()) as ApiKeyItem[];
    } catch {
      return mockApiKeysList;
    }
  }

  public async createApiKey(name: string, role: string, rateLimit: number): Promise<ApiKeyItem> {
    const newKey: ApiKeyItem = {
      id: `key-${Date.now()}`,
      name,
      keyMasked: `rk-app-••••••••••••${Math.floor(1000 + Math.random() * 9000)}`,
      active: true,
      createdAt: 'Hari ini',
      lastUsed: 'Belum digunakan',
      role,
      rateLimit,
    };
    mockApiKeysList = [newKey, ...mockApiKeysList];
    return newKey;
  }

  public async revokeApiKey(keyId: string): Promise<boolean> {
    mockApiKeysList = mockApiKeysList.filter((k) => k.id !== keyId);
    return true;
  }

  public async getUsers(): Promise<UserItem[]> {
    try {
      const response = await fetch(`${this.baseUrl}/users`);
      if (!response.ok) throw new Error('Network error');
      return (await response.json()) as UserItem[];
    } catch {
      return mockUsersList;
    }
  }

  public async addUser(name: string, email: string, role: UserItem['role']): Promise<UserItem> {
    const roleColors: Record<string, string> = {
      Admin: colors.avatarAdmin,
      Operator: colors.avatarOperator,
      Developer: colors.avatarDev,
      User: colors.avatarUser,
    };
    const newUser: UserItem = {
      id: `usr-${Date.now()}`,
      name,
      email,
      role,
      active: true,
      initial: name.charAt(0).toUpperCase() || 'U',
      avatarColor: roleColors[role] || colors.avatarUser,
    };
    mockUsersList = [newUser, ...mockUsersList];
    return newUser;
  }

  public async deleteUser(userId: string): Promise<boolean> {
    mockUsersList = mockUsersList.filter((u) => u.id !== userId);
    return true;
  }

  public async getEgressPools(): Promise<EgressPoolItem[]> {
    return mockEgressPoolsList;
  }

  public async addEgressPool(pool: Omit<EgressPoolItem, 'id'>): Promise<EgressPoolItem> {
    const newPool: EgressPoolItem = {
      ...pool,
      id: `egr-${Date.now()}`,
    };
    mockEgressPoolsList = [newPool, ...mockEgressPoolsList];
    return newPool;
  }

  public async toggleEgressPool(poolId: string, currentActive: boolean): Promise<boolean> {
    mockEgressPoolsList = mockEgressPoolsList.map((p) =>
      p.id === poolId ? { ...p, active: !currentActive } : p
    );
    return !currentActive;
  }

  public async getBudgets(): Promise<BudgetItem[]> {
    return mockBudgetsList;
  }

  public async updateBudgetLimit(budgetId: string, newAllocated: number): Promise<boolean> {
    mockBudgetsList = mockBudgetsList.map((b) =>
      b.id === budgetId
        ? {
            ...b,
            allocatedMonthly: newAllocated,
            percentage: Number(((b.spentMonthly / newAllocated) * 100).toFixed(1)),
          }
        : b
    );
    return true;
  }

  public async getWebhooks(): Promise<WebhookItem[]> {
    return mockWebhooksList;
  }

  public async addWebhook(name: string, url: string, events: string[]): Promise<WebhookItem> {
    const newHook: WebhookItem = {
      id: `wh-${Date.now()}`,
      name,
      url,
      events,
      active: true,
      lastDeliveryStatus: 'success',
      lastDeliveryTime: 'Baru saja',
    };
    mockWebhooksList = [newHook, ...mockWebhooksList];
    return newHook;
  }

  public async getDiagnostics(): Promise<DiagnosticVitals> {
    return MOCK_DIAGNOSTICS;
  }

  public async getSettings(): Promise<GatewaySettings> {
    return mockSettingsData;
  }

  public async updateSettings(newSettings: Partial<GatewaySettings>): Promise<GatewaySettings> {
    mockSettingsData = { ...mockSettingsData, ...newSettings };
    return mockSettingsData;
  }
}

export const api = new RouteXApiClient();
