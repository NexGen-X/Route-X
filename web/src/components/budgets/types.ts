import type { Budget, RateLimit, APIKey, Model } from '../../types';

export type BudgetTab = 'budgets' | 'limits';

export interface ScopeDisplayInfo {
  label: string;
  isRaw: boolean;
}

export interface BudgetFormData {
  name: string;
  scope: string;
  scope_id: string;
  period: string;
  max_spend_usd: string;
  alert_threshold: number;
  action: string;
}

export interface RateLimitFormData {
  scope: string;
  scope_id: string;
  requests_per_minute: number;
  tokens_per_minute: number;
  requests_per_second: number;
}

export interface BudgetCardProps {
  budget: Budget;
  scopeDisplay: ScopeDisplayInfo;
  onToggle: (budget: Budget) => void | Promise<void>;
  onReset: (id: string) => void | Promise<void>;
  onDelete: (id: string) => void | Promise<void>;
}

export interface RateLimitCardProps {
  limit: RateLimit;
  scopeDisplay: ScopeDisplayInfo;
  onDelete: (id: string) => void | Promise<void>;
}

export interface CreateBudgetDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  onSubmit: (formData: BudgetFormData) => void | Promise<void>;
  apiKeys: APIKey[];
  models: Model[];
  initialValues?: Partial<BudgetFormData>;
}

export interface CreateRateLimitDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  onSubmit: (formData: RateLimitFormData) => void | Promise<void>;
  apiKeys: APIKey[];
  initialValues?: Partial<RateLimitFormData>;
}
