import type { Model, ProviderModel, Price } from '../../types';

export type ViewMode = 'table' | 'grid';

export interface CreateModelFormData {
  model_id: string;
  display_name: string;
  family: string;
  context_window: number;
  max_output_tokens: number;
}

export interface PricingFormData {
  input_per_1m_usd: string;
  output_per_1m_usd: string;
  cached_input_per_1m_usd: string;
}

export interface ModelFilterBarProps {
  searchQuery: string;
  onSearchChange: (query: string) => void;
  viewMode: ViewMode;
  onViewModeChange: (mode: ViewMode) => void;
  selectedFamily: string;
  onSelectFamily: (family: string) => void;
  availableFamilies: string[];
  familyCounts: Record<string, number>;
  totalCount: number;
  filteredCount: number;
}

export interface ModelTableViewProps {
  models: Model[];
  onOpenPricing: (model: Model) => void;
  onDeleteModel: (id: string, name: string) => void;
}

export interface ModelGridViewProps {
  models: Model[];
  onOpenPricing: (model: Model) => void;
  onDeleteModel: (id: string, name: string) => void;
}

export interface ModelPaginationProps {
  currentPage: number;
  pageSize: number;
  totalItems: number;
  totalPages: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
  totalUnfilteredItems?: number;
}

export interface CreateModelDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  onSubmit: (formData: CreateModelFormData) => Promise<void> | void;
  isSubmitting?: boolean;
  existingModelIds?: string[];
}

export interface ModelPricingDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  model: Model | null;
  mappings: ProviderModel[];
  selectedMappingId: string;
  onSelectMappingId: (id: string) => void;
  pricingHistory: Price[];
  pricingForm: PricingFormData;
  onPricingFormChange: (form: PricingFormData) => void;
  onSavePrice: () => Promise<void> | void;
  isLoading?: boolean;
  isSaving?: boolean;
}
