import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TouchableOpacity,
  Switch,
  Modal,
  TextInput,
  Alert,
} from 'react-native';
import { Layers, Eye, DollarSign, CornerDownRight } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';
import { api } from '../api/client';
import { ModelCatalogItem } from '../api/types';

export interface ModelsScreenProps {
  onBack: () => void;
}

const CATEGORIES = ['Semua', 'Reasoning', 'Chat & Multimodal', 'Coding'];

export const ModelsScreen: React.FC<ModelsScreenProps> = ({ onBack }) => {
  const [models, setModels] = useState<ModelCatalogItem[]>([]);
  const [selectedCat, setSelectedCat] = useState('Semua');
  const [searchQuery, setSearchQuery] = useState('');
  const [showSearch, setShowSearch] = useState(false);

  // Modal Tambah Model
  const [modalVisible, setModalVisible] = useState(false);
  const [modelName, setModelName] = useState('');
  const [modelAlias, setModelAlias] = useState('');
  const [modelProvider, setModelProvider] = useState('OpenAI');
  const [contextWindow, setContextWindow] = useState('128K Tokens');
  const [fallbackModel, setFallbackModel] = useState('claude-3-5-sonnet');

  useEffect(() => {
    loadModels();
  }, []);

  const loadModels = async () => {
    const list = await api.getModels();
    setModels([...list]);
  };

  const handleToggle = async (id: string, currentState: boolean) => {
    await api.toggleModel(id, currentState);
    loadModels();
  };

  const handleAddModel = async () => {
    if (!modelName.trim() || !modelAlias.trim()) {
      Alert.alert('Validasi Galat', 'Nama model dan alias wajib diisi.');
      return;
    }
    await api.addModel({
      name: modelName,
      alias: modelAlias.toLowerCase().replace(/\s+/g, '-'),
      provider: modelProvider,
      contextWindow,
      inputCostPer1K: 0.002,
      outputCostPer1K: 0.008,
      category: 'Chat & Multimodal',
      isVisionSupported: true,
      active: true,
      fallbackModel,
    });
    setModalVisible(false);
    setModelName('');
    setModelAlias('');
    Alert.alert('Sukses', `Model virtual "${modelAlias}" berhasil dipetakan.`);
    loadModels();
  };

  const filteredModels = models.filter((m) => {
    const matchCat = selectedCat === 'Semua' || m.category === selectedCat;
    const matchSearch =
      searchQuery.trim() === '' ||
      m.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      m.alias.toLowerCase().includes(searchQuery.toLowerCase()) ||
      m.provider.toLowerCase().includes(searchQuery.toLowerCase());
    return matchCat && matchSearch;
  });

  return (
    <View style={styles.container}>
      <TopHeader
        mode="subscreen"
        title="Katalog Model AI"
        onBack={onBack}
        onSearch={() => setShowSearch(!showSearch)}
        onAdd={() => setModalVisible(true)}
      />

      {showSearch && (
        <View style={styles.searchBar}>
          <TextInput
            style={styles.searchInput}
            placeholder="Cari nama model, alias, atau provider..."
            placeholderTextColor={colors.textMuted}
            value={searchQuery}
            onChangeText={setSearchQuery}
            autoFocus
          />
        </View>
      )}

      {/* Filter Category Tabs */}
      <View style={styles.filterSection}>
        <ScrollView
          horizontal
          showsHorizontalScrollIndicator={false}
          contentContainerStyle={styles.filterContent}
        >
          {CATEGORIES.map((cat) => {
            const isActive = selectedCat === cat;
            return (
              <TouchableOpacity
                key={cat}
                style={[styles.filterChip, isActive && styles.filterChipActive]}
                onPress={() => setSelectedCat(cat)}
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

      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        <View style={styles.headerInfoRow}>
          <Text style={styles.headerCount}>{filteredModels.length} Model Terkatalog</Text>
          <Text style={styles.headerSub}>Virtual Model Aliasing Aktif</Text>
        </View>

        <View style={styles.modelList}>
          {filteredModels.map((item) => (
            <View key={item.id} style={styles.modelCard}>
              <View style={styles.modelCardHeader}>
                <View style={styles.titleCol}>
                  <View style={styles.badgeRow}>
                    <View style={styles.providerBadge}>
                      <Text style={styles.providerBadgeText}>{item.provider}</Text>
                    </View>
                    <View style={styles.categoryBadge}>
                      <Text style={styles.categoryBadgeText}>{item.category}</Text>
                    </View>
                  </View>
                  <Text style={styles.modelName}>{item.name}</Text>
                  <Text style={styles.aliasText}>alias: {item.alias}</Text>
                </View>

                <Switch
                  value={item.active}
                  onValueChange={() => handleToggle(item.id, item.active)}
                  trackColor={{ false: colors.bgElevated, true: colors.accentPrimary }}
                  thumbColor="#FFFFFF"
                />
              </View>

              {/* Specs & Pricing */}
              <View style={styles.specsRow}>
                <View style={styles.specItem}>
                  <Layers size={13} color={colors.accentPrimary} />
                  <Text style={styles.specText}>{item.contextWindow}</Text>
                </View>

                <View style={styles.specItem}>
                  <DollarSign size={13} color={colors.accentLime} />
                  <Text style={styles.specText}>
                    ${item.inputCostPer1K} / ${item.outputCostPer1K} per 1K
                  </Text>
                </View>

                {item.isVisionSupported && (
                  <View style={styles.visionBadge}>
                    <Eye size={12} color={colors.avatarDev} />
                    <Text style={styles.visionText}>Vision</Text>
                  </View>
                )}
              </View>

              {/* Fallback Chain */}
              <View style={styles.fallbackRow}>
                <CornerDownRight size={13} color={colors.textMuted} />
                <Text style={styles.fallbackText}>
                  Fallback otomatis: <Text style={styles.fallbackTarget}>{item.fallbackModel}</Text>
                </Text>
              </View>
            </View>
          ))}
        </View>
      </ScrollView>

      {/* Modal Tambah / Petakan Model */}
      <Modal
        visible={modalVisible}
        transparent={true}
        animationType="slide"
        onRequestClose={() => setModalVisible(false)}
      >
        <View style={styles.modalOverlay}>
          <View style={styles.modalContent}>
            <View style={styles.modalHeader}>
              <Text style={styles.modalTitle}>Petakan Model AI Baru</Text>
              <TouchableOpacity onPress={() => setModalVisible(false)}>
                <Text style={styles.closeBtn}>✕</Text>
              </TouchableOpacity>
            </View>

            <Text style={styles.inputLabel}>Nama Model Lengkap</Text>
            <TextInput
              style={styles.textInput}
              placeholder="Contoh: LLaMA 3.3 70B Turbo"
              placeholderTextColor={colors.textMuted}
              value={modelName}
              onChangeText={setModelName}
            />

            <Text style={styles.inputLabel}>Virtual Model Alias (Untuk Klien)</Text>
            <TextInput
              style={styles.textInput}
              placeholder="Contoh: chat-fast"
              placeholderTextColor={colors.textMuted}
              value={modelAlias}
              onChangeText={setModelAlias}
              autoCapitalize="none"
            />

            <Text style={styles.inputLabel}>Penyedia Upstream</Text>
            <TextInput
              style={styles.textInput}
              placeholder="OpenAI / Anthropic / Google / Meta"
              placeholderTextColor={colors.textMuted}
              value={modelProvider}
              onChangeText={setModelProvider}
            />

            <Text style={styles.inputLabel}>Batas Konteks Token</Text>
            <TextInput
              style={styles.textInput}
              placeholder="128K Tokens"
              placeholderTextColor={colors.textMuted}
              value={contextWindow}
              onChangeText={setContextWindow}
            />

            <Text style={styles.inputLabel}>Model Cadangan (Fallback)</Text>
            <TextInput
              style={styles.textInput}
              placeholder="claude-3-5-sonnet"
              placeholderTextColor={colors.textMuted}
              value={fallbackModel}
              onChangeText={setFallbackModel}
              autoCapitalize="none"
            />

            <View style={styles.modalActionRow}>
              <TouchableOpacity
                style={styles.cancelBtn}
                onPress={() => setModalVisible(false)}
              >
                <Text style={styles.cancelBtnText}>Batal</Text>
              </TouchableOpacity>
              <TouchableOpacity
                style={styles.submitBtn}
                onPress={handleAddModel}
              >
                <Text style={styles.submitBtnText}>Simpan Pemetaan</Text>
              </TouchableOpacity>
            </View>
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
  searchBar: {
    paddingHorizontal: 16,
    paddingVertical: 8,
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
  },
  searchInput: {
    backgroundColor: colors.bgSurface,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 12,
    paddingVertical: 8,
    color: colors.textPrimary,
    fontSize: 13,
  },
  filterSection: {
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
  },
  filterContent: {
    paddingHorizontal: 16,
    paddingVertical: 10,
    gap: 8,
  },
  filterChip: {
    backgroundColor: colors.bgSurface,
    paddingHorizontal: 14,
    paddingVertical: 6,
    borderRadius: 16,
    borderWidth: 1,
    borderColor: colors.border,
  },
  filterChipActive: {
    backgroundColor: colors.accentGreenSubtle,
    borderColor: colors.accentPrimary,
  },
  filterChipText: {
    fontSize: 12,
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
  headerInfoRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 12,
  },
  headerCount: {
    fontSize: 13,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  headerSub: {
    fontSize: 11,
    color: colors.accentPrimary,
    fontWeight: '600',
  },
  modelList: {
    gap: 12,
  },
  modelCard: {
    backgroundColor: colors.bgSurface,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
  },
  modelCardHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'flex-start',
    marginBottom: 12,
  },
  titleCol: {
    flex: 1,
  },
  badgeRow: {
    flexDirection: 'row',
    gap: 6,
    marginBottom: 6,
  },
  providerBadge: {
    backgroundColor: colors.bgElevated,
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 4,
    borderWidth: 1,
    borderColor: colors.border,
  },
  providerBadgeText: {
    fontSize: 10,
    fontWeight: '700',
    color: colors.accentPrimary,
  },
  categoryBadge: {
    backgroundColor: colors.bgElevated,
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 4,
  },
  categoryBadgeText: {
    fontSize: 10,
    color: colors.textSecondary,
  },
  modelName: {
    fontSize: 15,
    fontWeight: '700',
    color: colors.textPrimary,
    marginBottom: 2,
  },
  aliasText: {
    fontSize: 11,
    color: colors.textMuted,
    fontFamily: 'monospace',
  },
  specsRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: 12,
    paddingVertical: 10,
    borderTopWidth: 1,
    borderBottomWidth: 1,
    borderColor: colors.border,
    marginVertical: 4,
  },
  specItem: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
  },
  specText: {
    fontSize: 11,
    color: colors.textSecondary,
    fontFamily: 'monospace',
  },
  visionBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
    backgroundColor: 'rgba(168, 85, 247, 0.12)',
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 4,
  },
  visionText: {
    fontSize: 10,
    fontWeight: '700',
    color: colors.avatarDev,
  },
  fallbackRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    marginTop: 8,
  },
  fallbackText: {
    fontSize: 11,
    color: colors.textMuted,
  },
  fallbackTarget: {
    color: colors.accentPrimary,
    fontWeight: '700',
    fontFamily: 'monospace',
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
    maxHeight: '85%',
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
    marginBottom: 14,
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
    fontSize: 11,
    fontWeight: '600',
    color: colors.textSecondary,
    marginBottom: 4,
    marginTop: 8,
  },
  textInput: {
    backgroundColor: colors.bgElevated,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 8,
    color: colors.textPrimary,
    fontSize: 13,
  },
  modalActionRow: {
    flexDirection: 'row',
    justifyContent: 'flex-end',
    gap: 10,
    marginTop: 18,
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
});
