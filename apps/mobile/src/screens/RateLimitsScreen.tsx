import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TouchableOpacity,
  Modal,
  TextInput,
  Alert,
} from 'react-native';
import { Gauge, Zap, Key, CheckCircle, ShieldCheck } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';
import { api } from '../api/client';
import { RateLimitTierItem } from '../api/types';

export interface RateLimitsScreenProps {
  onBack: () => void;
}

export const RateLimitsScreen: React.FC<RateLimitsScreenProps> = ({ onBack }) => {
  const [tiers, setTiers] = useState<RateLimitTierItem[]>([]);
  const [modalVisible, setModalVisible] = useState(false);

  // Form states
  const [name, setName] = useState('');
  const [rpm, setRpm] = useState('1000');
  const [tpm, setTpm] = useState('250000');
  const [concurrent, setConcurrent] = useState('20');
  const [algorithm, setAlgorithm] = useState<'Token Bucket' | 'Sliding Window' | 'Fixed Window'>('Sliding Window');
  const [description, setDescription] = useState('');

  useEffect(() => {
    loadTiers();
  }, []);

  const loadTiers = async () => {
    const list = await api.getRateLimits();
    setTiers([...list]);
  };

  const handleAddTier = async () => {
    if (!name.trim()) {
      Alert.alert('Validasi Galat', 'Nama tier rate limit wajib diisi.');
      return;
    }
    const rpmNum = parseInt(rpm, 10) || 1000;
    const tpmNum = parseInt(tpm, 10) || 250000;
    const concNum = parseInt(concurrent, 10) || 10;

    await api.addRateLimit({
      name,
      description: description || 'Konfigurasi batas laju kustom untuk kluster API.',
      requestsPerMinute: rpmNum,
      tokensPerMinute: tpmNum,
      maxConcurrent: concNum,
      algorithm,
      activeKeys: 0,
      isDefault: false,
    });

    setModalVisible(false);
    setName('');
    setDescription('');
    Alert.alert('Berhasil', `Tier "${name}" telah ditambahkan ke gateway.`);
    loadTiers();
  };

  return (
    <View style={styles.container}>
      <TopHeader
        mode="subscreen"
        title="Batas Laju (Rate Limits)"
        onBack={onBack}
        onAdd={() => setModalVisible(true)}
      />

      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        <View style={styles.headerInfoRow}>
          <Text style={styles.headerCount}>{tiers.length} Kebijakan Tier Aktif</Text>
          <Text style={styles.headerSub}>Proteksi Lonjakan Beban Gateway</Text>
        </View>

        {/* Global Protection Banner */}
        <View style={styles.bannerCard}>
          <View style={styles.bannerIconBox}>
            <ShieldCheck size={22} color={colors.accentPrimary} />
          </View>
          <View style={styles.bannerTextBox}>
            <Text style={styles.bannerTitle}>DDoS & Token Bucket Guard</Text>
            <Text style={styles.bannerDesc}>
              Batas laju diberlakukan secara atomic di level Redis memory buffer sebelum request diteruskan ke provider LLM.
            </Text>
          </View>
        </View>

        {/* Tier Cards */}
        <View style={styles.tierList}>
          {tiers.map((tier) => (
            <View key={tier.id} style={styles.tierCard}>
              <View style={styles.tierHeader}>
                <View style={styles.tierTitleBox}>
                  <Text style={styles.tierName}>{tier.name}</Text>
                  {tier.isDefault && (
                    <View style={styles.defaultBadge}>
                      <CheckCircle size={11} color={colors.accentLime} />
                      <Text style={styles.defaultBadgeText}>Default Tier</Text>
                    </View>
                  )}
                </View>

                <View style={styles.algoBadge}>
                  <Text style={styles.algoText}>{tier.algorithm}</Text>
                </View>
              </View>

              <Text style={styles.tierDesc}>{tier.description}</Text>

              {/* Metrics Grid */}
              <View style={styles.metricsGrid}>
                <View style={styles.metricCol}>
                  <View style={styles.metricLabelRow}>
                    <Gauge size={12} color={colors.accentPrimary} />
                    <Text style={styles.metricLabel}>Maks RPM</Text>
                  </View>
                  <Text style={styles.metricValue}>
                    {tier.requestsPerMinute.toLocaleString('id-ID')} <Text style={styles.metricUnit}>req/m</Text>
                  </Text>
                </View>

                <View style={styles.metricCol}>
                  <View style={styles.metricLabelRow}>
                    <Zap size={12} color={colors.accentLime} />
                    <Text style={styles.metricLabel}>Maks TPM</Text>
                  </View>
                  <Text style={styles.metricValue}>
                    {(tier.tokensPerMinute / 1000).toLocaleString('id-ID')}k <Text style={styles.metricUnit}>tok/m</Text>
                  </Text>
                </View>

                <View style={styles.metricCol}>
                  <View style={styles.metricLabelRow}>
                    <Key size={12} color={colors.avatarDev} />
                    <Text style={styles.metricLabel}>Kunci Aktif</Text>
                  </View>
                  <Text style={styles.metricValue}>{tier.activeKeys} Kunci</Text>
                </View>
              </View>

              <View style={styles.footerRow}>
                <Text style={styles.footerNote}>
                  Konkurensi simultan: <Text style={styles.footerHighlight}>{tier.maxConcurrent} koneksi paralel</Text>
                </Text>
              </View>
            </View>
          ))}
        </View>
      </ScrollView>

      {/* Modal Tambah Tier */}
      <Modal
        visible={modalVisible}
        transparent={true}
        animationType="slide"
        onRequestClose={() => setModalVisible(false)}
      >
        <View style={styles.modalOverlay}>
          <View style={styles.modalContent}>
            <View style={styles.modalHeader}>
              <Text style={styles.modalTitle}>Tambah Tier Batas Laju</Text>
              <TouchableOpacity onPress={() => setModalVisible(false)}>
                <Text style={styles.closeBtn}>✕</Text>
              </TouchableOpacity>
            </View>

            <ScrollView showsVerticalScrollIndicator={false}>
              <Text style={styles.inputLabel}>Nama Kebijakan Tier</Text>
              <TextInput
                style={styles.textInput}
                placeholder="Contoh: Tier Dedicated FinTech"
                placeholderTextColor={colors.textMuted}
                value={name}
                onChangeText={setName}
              />

              <View style={styles.formRow}>
                <View style={styles.formCol}>
                  <Text style={styles.inputLabel}>Maks Req / Menit</Text>
                  <TextInput
                    style={styles.textInput}
                    placeholder="1000"
                    placeholderTextColor={colors.textMuted}
                    value={rpm}
                    onChangeText={setRpm}
                    keyboardType="numeric"
                  />
                </View>

                <View style={styles.formCol}>
                  <Text style={styles.inputLabel}>Maks Token / Menit</Text>
                  <TextInput
                    style={styles.textInput}
                    placeholder="250000"
                    placeholderTextColor={colors.textMuted}
                    value={tpm}
                    onChangeText={setTpm}
                    keyboardType="numeric"
                  />
                </View>
              </View>

              <Text style={styles.inputLabel}>Batas Konkurensi Simultan</Text>
              <TextInput
                style={styles.textInput}
                placeholder="20"
                placeholderTextColor={colors.textMuted}
                value={concurrent}
                onChangeText={setConcurrent}
                keyboardType="numeric"
              />

              <Text style={styles.inputLabel}>Algoritma Pembatasan</Text>
              <View style={styles.algoSelectorRow}>
                {(['Sliding Window', 'Token Bucket', 'Fixed Window'] as const).map((alg) => (
                  <TouchableOpacity
                    key={alg}
                    style={[styles.algoSelectBtn, algorithm === alg && styles.algoSelectBtnActive]}
                    onPress={() => setAlgorithm(alg)}
                  >
                    <Text
                      style={[
                        styles.algoSelectText,
                        algorithm === alg && styles.algoSelectTextActive,
                      ]}
                    >
                      {alg}
                    </Text>
                  </TouchableOpacity>
                ))}
              </View>

              <Text style={styles.inputLabel}>Deskripsi / Catatan</Text>
              <TextInput
                style={[styles.textInput, styles.textArea]}
                placeholder="Catatan peruntukan tier ini..."
                placeholderTextColor={colors.textMuted}
                value={description}
                onChangeText={setDescription}
                multiline
                numberOfLines={2}
              />

              <TouchableOpacity style={styles.submitBtn} onPress={handleAddTier}>
                <Text style={styles.submitBtnText}>Simpan Tier Rate Limit</Text>
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
    marginBottom: 16,
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
  bannerCard: {
    flexDirection: 'row',
    backgroundColor: colors.bgCard,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    borderRadius: 12,
    padding: 14,
    alignItems: 'center',
    marginBottom: 16,
  },
  bannerIconBox: {
    width: 44,
    height: 44,
    borderRadius: 10,
    backgroundColor: colors.bgElevated,
    justifyContent: 'center',
    alignItems: 'center',
    marginRight: 12,
  },
  bannerTextBox: {
    flex: 1,
  },
  bannerTitle: {
    color: colors.textPrimary,
    fontSize: 14,
    fontWeight: '700',
    marginBottom: 2,
  },
  bannerDesc: {
    color: colors.textMuted,
    fontSize: 12,
    lineHeight: 17,
  },
  tierList: {
    gap: 12,
  },
  tierCard: {
    backgroundColor: colors.bgCard,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    borderRadius: 14,
    padding: 16,
  },
  tierHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'flex-start',
    marginBottom: 8,
  },
  tierTitleBox: {
    flex: 1,
    marginRight: 8,
  },
  tierName: {
    color: colors.textPrimary,
    fontSize: 16,
    fontWeight: '700',
    marginBottom: 4,
  },
  defaultBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: 'rgba(163, 230, 53, 0.1)',
    alignSelf: 'flex-start',
    paddingHorizontal: 7,
    paddingVertical: 2,
    borderRadius: 6,
    gap: 4,
  },
  defaultBadgeText: {
    color: colors.accentLime,
    fontSize: 11,
    fontWeight: '600',
  },
  algoBadge: {
    backgroundColor: colors.bgElevated,
    paddingHorizontal: 8,
    paddingVertical: 4,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
  },
  algoText: {
    color: colors.textSecondary,
    fontSize: 11,
    fontWeight: '600',
  },
  tierDesc: {
    color: colors.textMuted,
    fontSize: 12,
    lineHeight: 18,
    marginBottom: 14,
  },
  metricsGrid: {
    flexDirection: 'row',
    backgroundColor: colors.bgElevated,
    borderRadius: 10,
    padding: 12,
    justifyContent: 'space-between',
    marginBottom: 12,
  },
  metricCol: {
    flex: 1,
  },
  metricLabelRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
    marginBottom: 4,
  },
  metricLabel: {
    color: colors.textMuted,
    fontSize: 11,
  },
  metricValue: {
    color: colors.textPrimary,
    fontSize: 15,
    fontWeight: '700',
  },
  metricUnit: {
    fontSize: 11,
    color: colors.textMuted,
    fontWeight: '400',
  },
  footerRow: {
    borderTopWidth: 1,
    borderTopColor: colors.borderSubtle,
    paddingTop: 10,
  },
  footerNote: {
    color: colors.textMuted,
    fontSize: 12,
  },
  footerHighlight: {
    color: colors.accentPrimary,
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
  formRow: {
    flexDirection: 'row',
    gap: 12,
  },
  formCol: {
    flex: 1,
  },
  textArea: {
    height: 60,
    textAlignVertical: 'top',
  },
  algoSelectorRow: {
    flexDirection: 'row',
    gap: 8,
    marginBottom: 6,
  },
  algoSelectBtn: {
    flex: 1,
    paddingVertical: 8,
    paddingHorizontal: 6,
    borderRadius: 8,
    backgroundColor: colors.bgElevated,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    alignItems: 'center',
  },
  algoSelectBtnActive: {
    borderColor: colors.accentPrimary,
    backgroundColor: 'rgba(56, 189, 248, 0.1)',
  },
  algoSelectText: {
    color: colors.textMuted,
    fontSize: 11,
    fontWeight: '600',
  },
  algoSelectTextActive: {
    color: colors.accentPrimary,
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
