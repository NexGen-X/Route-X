import {
  SystemOverview,
  ActivityItem,
  ProviderItem,
  RoutingStrategyInfo,
  ApiKeyItem,
  UserItem,
} from './types';
import { colors } from '../theme/colors';

// Fallback Mock Data offline sesuai 6 layar desain mockup resmi
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
    value: '$12.48',
    trend: '+8%',
    isPositive: true,
    sparklineColor: colors.accentPrimary,
    points: [4, 6, 7, 8.5, 9.2, 11.0, 12.48],
  },
};

const MOCK_ACTIVITIES: ActivityItem[] = [
  {
    id: 'act-1',
    title: 'Request berhasil',
    method: 'POST',
    endpoint: '/v1/chat/completions',
    timestamp: '2m lalu',
    status: 'success',
    latencyMs: 142,
  },
  {
    id: 'act-2',
    title: 'Request berhasil',
    method: 'POST',
    endpoint: '/v1/chat/completions',
    timestamp: '5m lalu',
    status: 'success',
    latencyMs: 185,
  },
  {
    id: 'act-3',
    title: 'Provider diubah',
    method: 'PUT',
    endpoint: '/api/admin/providers/anthropic',
    timestamp: '12m lalu',
    status: 'warning',
    latencyMs: 88,
  },
  {
    id: 'act-4',
    title: 'User ditambahkan',
    method: 'POST',
    endpoint: '/api/admin/users',
    timestamp: '1j lalu',
    status: 'success',
    latencyMs: 64,
  },
];

const MOCK_PROVIDERS: ProviderItem[] = [
  {
    id: 'prov-openai',
    name: 'OpenAI',
    brand: 'openai',
    category: 'LLM & Multimodal',
    active: true,
    modelCount: 8,
    priority: 1,
    latencyMs: 142,
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
  },
];

const MOCK_ROUTING_STRATEGY: RoutingStrategyInfo = {
  id: 'strat-rr',
  name: 'Round Robin',
  description: 'Distribusi request seimbang antar semua provider aktif dengan prioritas setara.',
  current: true,
};

const MOCK_API_KEYS: ApiKeyItem[] = [
  {
    id: 'key-1',
    name: 'Default Production Key',
    keyMasked: 'rk-live-••••••••••••94f2',
    active: true,
    createdAt: '12 Sep 2026',
    lastUsed: 'Baru saja',
  },
  {
    id: 'key-2',
    name: 'Mobile App Companion',
    keyMasked: 'rk-mobi-••••••••••••83b1',
    active: true,
    createdAt: '20 Sep 2026',
    lastUsed: '1m lalu',
  },
  {
    id: 'key-3',
    name: 'Development Sandbox',
    keyMasked: 'rk-test-••••••••••••10c8',
    active: false,
    createdAt: '28 Sep 2026',
    lastUsed: '2 hari lalu',
  },
  {
    id: 'key-4',
    name: 'Egress Cluster Gateway',
    keyMasked: 'rk-egrs-••••••••••••47e9',
    active: true,
    createdAt: '01 Okt 2026',
    lastUsed: '10m lalu',
  },
];

const MOCK_USERS: UserItem[] = [
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
      return MOCK_PROVIDERS;
    }
  }

  public async toggleProvider(providerId: string, currentActive: boolean): Promise<boolean> {
    try {
      const response = await fetch(`${this.baseUrl}/providers/${providerId}/toggle`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ active: !currentActive }),
      });
      return response.ok;
    } catch {
      return !currentActive;
    }
  }

  public async getRoutingStrategy(): Promise<RoutingStrategyInfo> {
    return MOCK_ROUTING_STRATEGY;
  }

  public async getApiKeys(): Promise<ApiKeyItem[]> {
    try {
      const response = await fetch(`${this.baseUrl}/keys`);
      if (!response.ok) throw new Error('Network error');
      return (await response.json()) as ApiKeyItem[];
    } catch {
      return MOCK_API_KEYS;
    }
  }

  public async getUsers(): Promise<UserItem[]> {
    try {
      const response = await fetch(`${this.baseUrl}/users`);
      if (!response.ok) throw new Error('Network error');
      return (await response.json()) as UserItem[];
    } catch {
      return MOCK_USERS;
    }
  }
}

export const api = new RouteXApiClient();
