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
import { GitBranch, ArrowRight, ShieldAlert, Cpu } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';
import { api } from '../api/client';
import { CustomRoutingRuleItem } from '../api/types';

export interface RoutingRulesScreenProps {
  onBack: () => void;
}

export const RoutingRulesScreen: React.FC<RoutingRulesScreenProps> = ({ onBack }) => {
  const [rules, setRules] = useState<CustomRoutingRuleItem[]>([]);
  const [modalVisible, setModalVisible] = useState(false);

  // Form states
  const [name, setName] = useState('');
  const [condition, setCondition] = useState('');
  const [targetProvider, setTargetProvider] = useState('OpenAI');
  const [targetModel, setTargetModel] = useState('gpt-4o');
  const [fallbackProvider, setFallbackProvider] = useState('Anthropic Claude');
  const [priority, setPriority] = useState('1');
  const [matchType, setMatchType] = useState<'Header' | 'Prompt Length' | 'User Role' | 'Latency'>('Prompt Length');

  useEffect(() => {
    loadRules();
  }, []);

  const loadRules = async () => {
    const list = await api.getRoutingRules();
    // Sort by priority ascending
    setRules([...list].sort((a, b) => a.priority - b.priority));
  };

  const handleToggle = async (ruleId: string, currentActive: boolean) => {
    await api.toggleRoutingRule(ruleId, currentActive);
    loadRules();
  };

  const handleAddRule = async () => {
    if (!name.trim() || !condition.trim()) {
      Alert.alert('Validasi Galat', 'Nama aturan dan kondisi wajib diisi.');
      return;
    }
    const prioNum = parseInt(priority, 10) || 1;

    await api.addRoutingRule({
      name,
      conditionDescription: condition,
      targetProvider,
      targetModel,
      fallbackProvider,
      priority: prioNum,
      active: true,
      matchType,
    });

    setModalVisible(false);
    setName('');
    setCondition('');
    Alert.alert('Berhasil', `Aturan routing "${name}" berhasil diaktifkan.`);
    loadRules();
  };

  const getMatchTypeColor = (type: CustomRoutingRuleItem['matchType']) => {
    switch (type) {
      case 'Prompt Length':
        return colors.accentPrimary;
      case 'Header':
        return colors.accentLime;
      case 'Latency':
        return colors.warning;
      case 'User Role':
        return colors.avatarDev;
      default:
        return colors.textMuted;
    }
  };

  return (
    <View style={styles.container}>
      <TopHeader
        mode="subscreen"
        title="Aturan Routing Kustom"
        onBack={onBack}
        onAdd={() => setModalVisible(true)}
      />

      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        <View style={styles.headerInfoRow}>
          <Text style={styles.headerCount}>{rules.length} Aturan Evaluasi Dinamis</Text>
          <Text style={styles.headerSub}>Urutan Prioritas P1 → Pn</Text>
        </View>

        {/* Info Banner */}
        <View style={styles.infoBanner}>
          <GitBranch size={20} color={colors.accentPrimary} />
          <Text style={styles.infoBannerText}>
            Setiap request dievaluasi dari prioritas tertinggi. Jika kondisi terpenuhi, rute dialihkan otomatis ke target provider yang ditentukan.
          </Text>
        </View>

        {/* Rules List */}
        <View style={styles.rulesList}>
          {rules.map((rule) => {
            const matchColor = getMatchTypeColor(rule.matchType);
            return (
              <View key={rule.id} style={styles.ruleCard}>
                <View style={styles.cardHeader}>
                  <View style={styles.headerBadges}>
                    <View style={styles.prioBadge}>
                      <Text style={styles.prioText}>P{rule.priority}</Text>
                    </View>
                    <View style={[styles.matchBadge, { borderColor: matchColor }]}>
                      <Text style={[styles.matchText, { color: matchColor }]}>
                        {rule.matchType}
                      </Text>
                    </View>
                  </View>

                  <Switch
                    value={rule.active}
                    onValueChange={() => handleToggle(rule.id, rule.active)}
                    trackColor={{ false: colors.bgElevated, true: colors.accentPrimary }}
                    thumbColor="#FFFFFF"
                  />
                </View>

                <Text style={styles.ruleName}>{rule.name}</Text>
                <Text style={styles.conditionText}>
                  <Text style={styles.conditionLabel}>Kondisi: </Text>
                  {rule.conditionDescription}
                </Text>

                {/* Routing Flow */}
                <View style={styles.flowContainer}>
                  <View style={styles.flowTargetBox}>
                    <Cpu size={14} color={colors.accentLime} />
                    <View style={styles.flowTextBox}>
                      <Text style={styles.flowLabel}>Rute Target Utama</Text>
                      <Text style={styles.flowMainText}>
                        {rule.targetProvider} ({rule.targetModel})
                      </Text>
                    </View>
                  </View>

                  <View style={styles.flowArrowRow}>
                    <ArrowRight size={14} color={colors.textMuted} />
                    <ShieldAlert size={13} color={colors.warning} />
                    <Text style={styles.fallbackLabel}>
                      Fallback: <Text style={styles.fallbackValue}>{rule.fallbackProvider}</Text>
                    </Text>
                  </View>
                </View>
              </View>
            );
          })}
        </View>
      </ScrollView>

      {/* Modal Tambah Aturan */}
      <Modal
        visible={modalVisible}
        transparent={true}
        animationType="slide"
        onRequestClose={() => setModalVisible(false)}
      >
        <View style={styles.modalOverlay}>
          <View style={styles.modalContent}>
            <View style={styles.modalHeader}>
              <Text style={styles.modalTitle}>Tambah Aturan Routing Baru</Text>
              <TouchableOpacity onPress={() => setModalVisible(false)}>
                <Text style={styles.closeBtn}>✕</Text>
              </TouchableOpacity>
            </View>

            <ScrollView showsVerticalScrollIndicator={false}>
              <Text style={styles.inputLabel}>Nama Aturan</Text>
              <TextInput
                style={styles.textInput}
                placeholder="Contoh: Rute Khusus Coding Model"
                placeholderTextColor={colors.textMuted}
                value={name}
                onChangeText={setName}
              />

              <Text style={styles.inputLabel}>Tipe Pencocokan (Match Type)</Text>
              <View style={styles.matchTypeGrid}>
                {(['Prompt Length', 'Header', 'Latency', 'User Role'] as const).map((type) => (
                  <TouchableOpacity
                    key={type}
                    style={[styles.typeBtn, matchType === type && styles.typeBtnActive]}
                    onPress={() => setMatchType(type)}
                  >
                    <Text
                      style={[
                        styles.typeBtnText,
                        matchType === type && styles.typeBtnTextActive,
                      ]}
                    >
                      {type}
                    </Text>
                  </TouchableOpacity>
                ))}
              </View>

              <Text style={styles.inputLabel}>Kondisi Logika / Kriteria</Text>
              <TextInput
                style={styles.textInput}
                placeholder="Contoh: Jika header X-Department bernilai 'Engineering'"
                placeholderTextColor={colors.textMuted}
                value={condition}
                onChangeText={setCondition}
              />

              <View style={styles.formRow}>
                <View style={styles.formCol}>
                  <Text style={styles.inputLabel}>Target Provider</Text>
                  <TextInput
                    style={styles.textInput}
                    placeholder="Contoh: Anthropic"
                    placeholderTextColor={colors.textMuted}
                    value={targetProvider}
                    onChangeText={setTargetProvider}
                  />
                </View>

                <View style={styles.formCol}>
                  <Text style={styles.inputLabel}>Target Model</Text>
                  <TextInput
                    style={styles.textInput}
                    placeholder="claude-3-5-sonnet"
                    placeholderTextColor={colors.textMuted}
                    value={targetModel}
                    onChangeText={setTargetModel}
                  />
                </View>
              </View>

              <View style={styles.formRow}>
                <View style={styles.formCol}>
                  <Text style={styles.inputLabel}>Fallback Provider</Text>
                  <TextInput
                    style={styles.textInput}
                    placeholder="Contoh: OpenAI GPT-4o"
                    placeholderTextColor={colors.textMuted}
                    value={fallbackProvider}
                    onChangeText={setFallbackProvider}
                  />
                </View>

                <View style={styles.formColSmall}>
                  <Text style={styles.inputLabel}>Prioritas</Text>
                  <TextInput
                    style={styles.textInput}
                    placeholder="1"
                    placeholderTextColor={colors.textMuted}
                    value={priority}
                    onChangeText={setPriority}
                    keyboardType="numeric"
                  />
                </View>
              </View>

              <TouchableOpacity style={styles.submitBtn} onPress={handleAddRule}>
                <Text style={styles.submitBtnText}>Aktifkan Aturan Routing</Text>
              </TouchableOpacity>
            </ScrollView>
          </View>
        </View>
      </Modal>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.bgBase,
  },
  scrollArea: {
    flex: 1,
  },
  scrollContent: {
    padding: 16,
    paddingBottom: 40,
  },
  headerInfoRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 14,
  },
  headerCount: {
    color: colors.textPrimary,
    fontSize: 14,
    fontWeight: '700',
  },
  headerSub: {
    color: colors.textMuted,
    fontSize: 12,
  },
  infoBanner: {
    flexDirection: 'row',
    backgroundColor: colors.bgCard,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    borderRadius: 12,
    padding: 14,
    alignItems: 'center',
    gap: 12,
    marginBottom: 16,
  },
  infoBannerText: {
    flex: 1,
    color: colors.textSecondary,
    fontSize: 12,
    lineHeight: 18,
  },
  rulesList: {
    gap: 12,
  },
  ruleCard: {
    backgroundColor: colors.bgCard,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    borderRadius: 14,
    padding: 16,
  },
  cardHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 10,
  },
  headerBadges: {
    flexDirection: 'row',
    gap: 8,
    alignItems: 'center',
  },
  prioBadge: {
    backgroundColor: colors.accentPrimary,
    paddingHorizontal: 8,
    paddingVertical: 2,
    borderRadius: 6,
  },
  prioText: {
    color: colors.bgBase,
    fontSize: 11,
    fontWeight: '800',
  },
  matchBadge: {
    borderWidth: 1,
    paddingHorizontal: 8,
    paddingVertical: 2,
    borderRadius: 6,
    backgroundColor: colors.bgElevated,
  },
  matchText: {
    fontSize: 11,
    fontWeight: '600',
  },
  ruleName: {
    color: colors.textPrimary,
    fontSize: 16,
    fontWeight: '700',
    marginBottom: 6,
  },
  conditionText: {
    color: colors.textSecondary,
    fontSize: 13,
    lineHeight: 18,
    marginBottom: 14,
  },
  conditionLabel: {
    color: colors.textMuted,
    fontWeight: '600',
  },
  flowContainer: {
    backgroundColor: colors.bgElevated,
    borderRadius: 10,
    padding: 12,
    gap: 10,
  },
  flowTargetBox: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 10,
  },
  flowTextBox: {
    flex: 1,
  },
  flowLabel: {
    color: colors.textMuted,
    fontSize: 10,
    textTransform: 'uppercase',
    letterSpacing: 0.5,
  },
  flowMainText: {
    color: colors.accentLime,
    fontSize: 14,
    fontWeight: '700',
  },
  flowArrowRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    borderTopWidth: 1,
    borderTopColor: colors.borderSubtle,
    paddingTop: 8,
  },
  fallbackLabel: {
    color: colors.textMuted,
    fontSize: 12,
  },
  fallbackValue: {
    color: colors.textPrimary,
    fontWeight: '600',
  },
  // Modal styles
  modalOverlay: {
    flex: 1,
    backgroundColor: 'rgba(0, 0, 0, 0.75)',
    justifyContent: 'flex-end',
  },
  modalContent: {
    backgroundColor: colors.bgCard,
    borderTopLeftRadius: 20,
    borderTopRightRadius: 20,
    padding: 20,
    maxHeight: '85%',
    borderWidth: 1,
    borderColor: colors.borderSubtle,
  },
  modalHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 16,
  },
  modalTitle: {
    color: colors.textPrimary,
    fontSize: 18,
    fontWeight: '700',
  },
  closeBtn: {
    color: colors.textMuted,
    fontSize: 20,
    padding: 4,
  },
  inputLabel: {
    color: colors.textSecondary,
    fontSize: 12,
    fontWeight: '600',
    marginBottom: 6,
    marginTop: 10,
  },
  textInput: {
    backgroundColor: colors.bgElevated,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 10,
    color: colors.textPrimary,
    fontSize: 14,
  },
  matchTypeGrid: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 8,
  },
  typeBtn: {
    paddingVertical: 8,
    paddingHorizontal: 12,
    borderRadius: 8,
    backgroundColor: colors.bgElevated,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
  },
  typeBtnActive: {
    borderColor: colors.accentPrimary,
    backgroundColor: 'rgba(56, 189, 248, 0.1)',
  },
  typeBtnText: {
    color: colors.textMuted,
    fontSize: 12,
    fontWeight: '600',
  },
  typeBtnTextActive: {
    color: colors.accentPrimary,
  },
  formRow: {
    flexDirection: 'row',
    gap: 12,
  },
  formCol: {
    flex: 1,
  },
  formColSmall: {
    width: 80,
  },
  submitBtn: {
    backgroundColor: colors.accentPrimary,
    borderRadius: 10,
    paddingVertical: 14,
    alignItems: 'center',
    marginTop: 20,
    marginBottom: 10,
  },
  submitBtnText: {
    color: colors.bgBase,
    fontSize: 15,
    fontWeight: '700',
  },
});
