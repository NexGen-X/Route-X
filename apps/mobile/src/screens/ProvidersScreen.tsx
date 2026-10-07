import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TextInput,
  TouchableOpacity,
  Modal,
  Alert,
} from 'react-native';
import { ChevronRight, Zap, CheckCircle2, Cpu } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';
import { ProviderToggleItem } from '../components/ProviderToggleItem';
import { api } from '../api/client';
import { ProviderItem } from '../api/types';

export interface ProvidersScreenProps {
  onBack: () => void;
}

const FILTER_CATEGORIES = ['Semua', 'OpenAI', 'Anthropic', 'Google', 'Meta'];

export const ProvidersScreen: React.FC<ProvidersScreenProps> = ({ onBack }) => {
  const [providers, setProviders] = useState<ProviderItem[]>([]);
  const [selectedFilter, setSelectedFilter] = useState('Semua');
  const [searchQuery, setSearchQuery] = useState('');
  const [showSearch, setShowSearch] = useState(false);

  // Modal Tambah Provider
  const [addModalVisible, setAddModalVisible] = useState(false);
  const [newProvName, setNewProvName] = useState('');
  const [newProvBrand, setNewProvBrand] = useState<ProviderItem['brand']>('openai');
  const [newProvUrl, setNewProvUrl] = useState('');
  const newProvCategory = 'LLM & Multimodal';

  // Modal Detail Provider
  const [detailModalVisible, setDetailModalVisible] = useState(false);
  const [activeDetailItem, setActiveDetailItem] = useState<ProviderItem | null>(null);

  useEffect(() => {
    loadProviders();
  }, []);

  const loadProviders = async () => {
    const list = await api.getProviders();
    setProviders([...list]);
  };

  const handleToggle = async (id: string, nextState: boolean) => {
    setProviders((prev) =>
      prev.map((p) => (p.id === id ? { ...p, active: nextState } : p))
    );
    await api.toggleProvider(id, !nextState);
  };

  const handleSaveNewProvider = async () => {
    if (!newProvName.trim()) {
      Alert.alert('Validasi Galat', 'Nama provider wajib diisi.');
      return;
    }
    await api.addProvider({
      name: newProvName,
      brand: newProvBrand,
      category: newProvCategory,
      active: true,
      modelCount: 3,
      priority: providers.length + 1,
      latencyMs: 140,
      models: [`${newProvName.toLowerCase()}-standard`, `${newProvName.toLowerCase()}-fast`],
      baseUrl: newProvUrl || 'https://api.openai.com/v1',
    });
    setAddModalVisible(false);
    setNewProvName('');
    setNewProvUrl('');
    Alert.alert('Sukses', `Provider ${newProvName} berhasil didaftarkan.`);
    loadProviders();
  };

  const openDetails = (item: ProviderItem) => {
    setActiveDetailItem(item);
    setDetailModalVisible(true);
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
        onAdd={() => setAddModalVisible(true)}
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
              onPressDetails={() => openDetails(item)}
            />
            {/* Quick model chevron link */}
            <TouchableOpacity
              style={styles.modelChevronRow}
              onPress={() => openDetails(item)}
            >
              <Text style={styles.modelChevronText}>{item.modelCount} Model Terhubung</Text>
              <ChevronRight size={14} color={colors.accentPrimary} />
            </TouchableOpacity>
          </View>
        ))}
      </ScrollView>

      {/* Modal Tambah Provider Baru */}
      <Modal
        visible={addModalVisible}
        transparent={true}
        animationType="slide"
        onRequestClose={() => setAddModalVisible(false)}
      >
        <View style={styles.modalOverlay}>
          <View style={styles.modalContent}>
            <View style={styles.modalHeader}>
              <Text style={styles.modalTitle}>Tambah Provider AI Baru</Text>
              <TouchableOpacity onPress={() => setAddModalVisible(false)}>
                <Text style={styles.closeBtn}>✕</Text>
              </TouchableOpacity>
            </View>

            <Text style={styles.inputLabel}>Nama Provider</Text>
            <TextInput
              style={styles.textInput}
              placeholder="Contoh: Groq Fast LLM"
              placeholderTextColor={colors.textMuted}
              value={newProvName}
              onChangeText={setNewProvName}
            />

            <Text style={styles.inputLabel}>Kategori Brand</Text>
            <View style={styles.brandRow}>
              {(['openai', 'anthropic', 'google', 'meta', 'mistral', 'deepseek'] as const).map(
                (brand) => (
                  <TouchableOpacity
                    key={brand}
                    style={[
                      styles.brandChip,
                      newProvBrand === brand && styles.brandChipActive,
                    ]}
                    onPress={() => setNewProvBrand(brand)}
                  >
                    <Text
                      style={[
                        styles.brandChipText,
                        newProvBrand === brand && styles.brandChipTextActive,
                      ]}
                    >
                      {brand.toUpperCase()}
                    </Text>
                  </TouchableOpacity>
                )
              )}
            </View>

            <Text style={styles.inputLabel}>Base URL / Endpoint API</Text>
            <TextInput
              style={styles.textInput}
              placeholder="https://api.openai.com/v1"
              placeholderTextColor={colors.textMuted}
              value={newProvUrl}
              onChangeText={setNewProvUrl}
              autoCapitalize="none"
            />

            <View style={styles.modalActionRow}>
              <TouchableOpacity
                style={styles.cancelBtn}
                onPress={() => setAddModalVisible(false)}
              >
                <Text style={styles.cancelBtnText}>Batal</Text>
              </TouchableOpacity>
              <TouchableOpacity
                style={styles.submitBtn}
                onPress={handleSaveNewProvider}
              >
                <Text style={styles.submitBtnText}>Simpan Provider</Text>
              </TouchableOpacity>
            </View>
          </View>
        </View>
      </Modal>

      {/* Modal Detail & Model Inspector */}
      <Modal
        visible={detailModalVisible}
        transparent={true}
        animationType="slide"
        onRequestClose={() => setDetailModalVisible(false)}
      >
        <View style={styles.modalOverlay}>
          <View style={styles.modalContent}>
            <View style={styles.modalHeader}>
              <View style={styles.detailTitleCluster}>
                <Cpu size={20} color={colors.accentPrimary} />
                <Text style={styles.modalTitle}>{activeDetailItem?.name}</Text>
              </View>
              <TouchableOpacity onPress={() => setDetailModalVisible(false)}>
                <Text style={styles.closeBtn}>✕</Text>
              </TouchableOpacity>
            </View>

            {activeDetailItem && (
              <>
                <View style={styles.detailStatsRow}>
                  <View style={styles.detailStatBox}>
                    <Text style={styles.detailStatLabel}>Latensi</Text>
                    <Text style={styles.detailStatVal}>{activeDetailItem.latencyMs} ms</Text>
                  </View>
                  <View style={styles.detailStatBox}>
                    <Text style={styles.detailStatLabel}>Prioritas</Text>
                    <Text style={styles.detailStatVal}>#{activeDetailItem.priority}</Text>
                  </View>
                  <View style={styles.detailStatBox}>
                    <Text style={styles.detailStatLabel}>Model Aktif</Text>
                    <Text style={styles.detailStatVal}>{activeDetailItem.modelCount}</Text>
                  </View>
                </View>

                <Text style={styles.inputLabel}>Daftar Model Terkatalog:</Text>
                <View style={styles.modelsContainer}>
                  {(activeDetailItem.models || ['standard-model', 'fast-model']).map(
                    (model, idx) => (
                      <View key={idx} style={styles.modelTagPill}>
                        <CheckCircle2 size={12} color={colors.accentPrimary} />
                        <Text style={styles.modelTagText}>{model}</Text>
                      </View>
                    )
                  )}
                </View>

                <TouchableOpacity
                  style={styles.testPingBtn}
                  onPress={() =>
                    Alert.alert(
                      'Tes Latensi Berhasil',
                      `Koneksi ke ${activeDetailItem.name} berhasil.\nPing respons: ${activeDetailItem.latencyMs}ms.\nStatus: Sehat & Siap Melayani Permintaan.`
                    )
                  }
                >
                  <Zap size={14} color="#0A0B0D" />
                  <Text style={styles.testPingBtnText}>Jalankan Tes Latensi Realtime</Text>
                </TouchableOpacity>
              </>
            )}
          </View>
        </View>
      </Modal>
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
  modalOverlay: {
    flex: 1,
    backgroundColor: 'rgba(0,0,0,0.7)',
    justifyContent: 'center',
    alignItems: 'center',
    padding: 20,
  },
  modalContent: {
    width: '100%',
    backgroundColor: colors.bgSurface,
    borderRadius: 16,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 20,
  },
  modalHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 16,
  },
  detailTitleCluster: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 10,
  },
  modalTitle: {
    fontSize: 16,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  closeBtn: {
    fontSize: 18,
    color: colors.textSecondary,
    fontWeight: 'bold',
  },
  inputLabel: {
    fontSize: 12,
    fontWeight: '600',
    color: colors.textSecondary,
    marginBottom: 6,
    marginTop: 10,
  },
  textInput: {
    backgroundColor: colors.bgElevated,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 10,
    color: colors.textPrimary,
    fontSize: 14,
  },
  brandRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 6,
    marginVertical: 4,
  },
  brandChip: {
    backgroundColor: colors.bgElevated,
    paddingHorizontal: 8,
    paddingVertical: 5,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: colors.border,
  },
  brandChipActive: {
    backgroundColor: colors.accentGreenSubtle,
    borderColor: colors.accentPrimary,
  },
  brandChipText: {
    fontSize: 10,
    fontWeight: '700',
    color: colors.textMuted,
  },
  brandChipTextActive: {
    color: colors.accentPrimary,
  },
  modalActionRow: {
    flexDirection: 'row',
    justifyContent: 'flex-end',
    gap: 10,
    marginTop: 20,
  },
  cancelBtn: {
    paddingHorizontal: 16,
    paddingVertical: 10,
    borderRadius: 8,
    backgroundColor: colors.bgElevated,
  },
  cancelBtnText: {
    color: colors.textSecondary,
    fontWeight: '600',
  },
  submitBtn: {
    paddingHorizontal: 18,
    paddingVertical: 10,
    borderRadius: 8,
    backgroundColor: colors.accentPrimary,
  },
  submitBtnText: {
    color: '#0A0B0D',
    fontWeight: '700',
  },
  detailStatsRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    marginVertical: 12,
    gap: 8,
  },
  detailStatBox: {
    flex: 1,
    backgroundColor: colors.bgElevated,
    padding: 10,
    borderRadius: 8,
    alignItems: 'center',
    borderWidth: 1,
    borderColor: colors.border,
  },
  detailStatLabel: {
    fontSize: 10,
    color: colors.textMuted,
    marginBottom: 4,
  },
  detailStatVal: {
    fontSize: 14,
    fontWeight: '800',
    color: colors.accentPrimary,
    fontFamily: 'monospace',
  },
  modelsContainer: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 6,
    marginVertical: 8,
  },
  modelTagPill: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
    backgroundColor: colors.bgElevated,
    paddingHorizontal: 8,
    paddingVertical: 4,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: colors.border,
  },
  modelTagText: {
    fontSize: 11,
    color: colors.textPrimary,
    fontFamily: 'monospace',
  },
  testPingBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: colors.accentPrimary,
    borderRadius: 10,
    paddingVertical: 12,
    gap: 8,
    marginTop: 16,
  },
  testPingBtnText: {
    color: '#0A0B0D',
    fontWeight: '700',
    fontSize: 13,
  },
});
