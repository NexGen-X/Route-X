import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TextInput,
  TouchableOpacity,
  Alert,
} from 'react-native';
import { ChevronRight } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';
import { ProviderToggleItem } from '../components/ProviderToggleItem';
import { api } from '../api/client';
import { ProviderItem } from '../api/types';

export interface ProvidersScreenProps {
  onBack: () => void;
}

const FILTER_CATEGORIES = ['Semua', 'OpenAI', 'Anthropic', 'Google'];

export const ProvidersScreen: React.FC<ProvidersScreenProps> = ({ onBack }) => {
  const [providers, setProviders] = useState<ProviderItem[]>([]);
  const [selectedFilter, setSelectedFilter] = useState('Semua');
  const [searchQuery, setSearchQuery] = useState('');
  const [showSearch, setShowSearch] = useState(false);

  useEffect(() => {
    loadProviders();
  }, []);

  const loadProviders = async () => {
    const list = await api.getProviders();
    setProviders(list);
  };

  const handleToggle = async (id: string, nextState: boolean) => {
    // Optimistic UI update
    setProviders((prev) =>
      prev.map((p) => (p.id === id ? { ...p, active: nextState } : p))
    );
    await api.toggleProvider(id, !nextState);
  };

  const filteredProviders = providers.filter((p) => {
    const matchesFilter =
      selectedFilter === 'Semua' ||
      p.name.toLowerCase().includes(selectedFilter.toLowerCase()) ||
      p.brand.toLowerCase().includes(selectedFilter.toLowerCase());

    const matchesSearch =
      searchQuery.trim() === '' ||
      p.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      p.category.toLowerCase().includes(searchQuery.toLowerCase());

    return matchesFilter && matchesSearch;
  });

  return (
    <View style={styles.container}>
      <TopHeader
        mode="subscreen"
        title="Providers"
        onBack={onBack}
        onSearch={() => setShowSearch(!showSearch)}
        onAdd={() =>
          Alert.alert('Tambah Provider', 'Formulir integrasi upstream provider AI baru.')
        }
      />

      {showSearch && (
        <View style={styles.searchContainer}>
          <TextInput
            style={styles.searchInput}
            placeholder="Cari provider AI..."
            placeholderTextColor={colors.textMuted}
            value={searchQuery}
            onChangeText={setSearchQuery}
            autoFocus
          />
        </View>
      )}

      {/* Filter Chips Horizontal */}
      <View style={styles.filterSection}>
        <ScrollView
          horizontal
          showsHorizontalScrollIndicator={false}
          contentContainerStyle={styles.filterContent}
        >
          {FILTER_CATEGORIES.map((cat) => {
            const isActive = selectedFilter === cat;
            return (
              <TouchableOpacity
                key={cat}
                style={[styles.filterChip, isActive && styles.filterChipActive]}
                onPress={() => setSelectedFilter(cat)}
                activeOpacity={0.8}
              >
                <Text
                  style={[
                    styles.filterChipText,
                    isActive && styles.filterChipTextActive,
                  ]}
                >
                  {cat}
                </Text>
              </TouchableOpacity>
            );
          })}
        </ScrollView>
      </View>

      {/* List Providers */}
      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        <View style={styles.listHeaderRow}>
          <Text style={styles.listSubtitle}>
            {filteredProviders.length} Provider Terdaftar
          </Text>
          <View style={styles.activeCountBadge}>
            <View style={styles.greenDot} />
            <Text style={styles.activeCountText}>
              {providers.filter((p) => p.active).length} Aktif
            </Text>
          </View>
        </View>

        {filteredProviders.map((item) => (
          <View key={item.id} style={styles.itemWrapper}>
            <ProviderToggleItem
              id={item.id}
              name={item.name}
              brand={item.brand}
              category={item.category}
              active={item.active}
              modelCount={item.modelCount}
              latencyMs={item.latencyMs}
              onToggle={handleToggle}
              onPressDetails={() =>
                Alert.alert(
                  item.name,
                  `Provider ${item.name} memiliki ${item.modelCount} model AI aktif dengan rata-rata latensi ${item.latencyMs}ms.`
                )
              }
            />
            {/* Quick model chevron link */}
            <TouchableOpacity
              style={styles.modelChevronRow}
              onPress={() =>
                Alert.alert(item.name, `Melihat daftar ${item.modelCount} model ${item.name}`)
              }
            >
              <Text style={styles.modelChevronText}>{item.modelCount} Model Terhubung</Text>
              <ChevronRight size={14} color={colors.accentPrimary} />
            </TouchableOpacity>
          </View>
        ))}
      </ScrollView>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.bgCanvas,
  },
  searchContainer: {
    paddingHorizontal: 16,
    paddingVertical: 8,
    backgroundColor: colors.bgCanvas,
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
  },
  searchInput: {
    backgroundColor: colors.bgSurface,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 14,
    paddingVertical: 8,
    color: colors.textPrimary,
    fontSize: 14,
  },
  filterSection: {
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
    backgroundColor: colors.bgCanvas,
  },
  filterContent: {
    paddingHorizontal: 16,
    paddingVertical: 12,
    gap: 8,
  },
  filterChip: {
    backgroundColor: colors.bgSurface,
    paddingHorizontal: 16,
    paddingVertical: 8,
    borderRadius: 20,
    borderWidth: 1,
    borderColor: colors.border,
  },
  filterChipActive: {
    backgroundColor: colors.accentGreenSubtle,
    borderColor: colors.accentPrimary,
  },
  filterChipText: {
    fontSize: 13,
    fontWeight: '600',
    color: colors.textSecondary,
  },
  filterChipTextActive: {
    color: colors.accentPrimary,
    fontWeight: '700',
  },
  scrollArea: {
    flex: 1,
  },
  scrollContent: {
    padding: 16,
    paddingBottom: 24,
  },
  listHeaderRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 12,
  },
  listSubtitle: {
    fontSize: 13,
    fontWeight: '600',
    color: colors.textSecondary,
  },
  activeCountBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    backgroundColor: colors.bgSurface,
    paddingHorizontal: 8,
    paddingVertical: 3,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
  },
  greenDot: {
    width: 6,
    height: 6,
    borderRadius: 3,
    backgroundColor: colors.accentPrimary,
  },
  activeCountText: {
    fontSize: 11,
    fontWeight: '700',
    color: colors.accentPrimary,
  },
  itemWrapper: {
    marginBottom: 10,
  },
  modelChevronRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'flex-end',
    gap: 4,
    marginTop: -4,
    marginBottom: 4,
    marginRight: 6,
  },
  modelChevronText: {
    fontSize: 11,
    color: colors.accentPrimary,
    fontWeight: '600',
  },
});
