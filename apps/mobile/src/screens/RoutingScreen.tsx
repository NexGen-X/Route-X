import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TouchableOpacity,
  Modal,
  Alert,
} from 'react-native';
import { Compass, ShieldAlert, CheckCircle2, Sliders } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';
import { ProviderToggleItem } from '../components/ProviderToggleItem';
import { api } from '../api/client';
import { ProviderItem, RoutingStrategyInfo } from '../api/types';

export interface RoutingScreenProps {
  onBack: () => void;
}

const AVAILABLE_STRATEGIES: RoutingStrategyInfo[] = [
  {
    id: 'strat-rr',
    name: 'Round Robin',
    description: 'Distribusi beban seimbang bergiliran antar semua provider aktif dengan prioritas setara.',
    current: true,
  },
  {
    id: 'strat-prio',
    name: 'Strict Priority & Fallback',
    description: 'Selalu gunakan Provider Prioritas #1 (OpenAI); hanya alihkan ke #2 jika latency > 2.5s atau error 5xx.',
    current: false,
  },
  {
    id: 'strat-latency',
    name: 'Latency-Weighted Routing',
    description: 'Pilih secara cerdas provider AI dengan kecepatan respons (ms ping) paling rendah di jaringan.',
    current: false,
  },
  {
    id: 'strat-cost',
    name: 'Cost & Token Optimization',
    description: 'Arahkan request ke model yang paling efisien anggaran token sebelum beralih ke reasoning model kelas atas.',
    current: false,
  },
];

export const RoutingScreen: React.FC<RoutingScreenProps> = ({ onBack }) => {
  const [activeTab, setActiveTab] = useState<'strategy' | 'failover'>('strategy');
  const [selectedStrategy, setSelectedStrategy] = useState<RoutingStrategyInfo>(AVAILABLE_STRATEGIES[0]);
  const [providers, setProviders] = useState<ProviderItem[]>([]);
  const [strategyModalVisible, setStrategyModalVisible] = useState(false);

  useEffect(() => {
    loadData();
  }, []);

  const loadData = async () => {
    const provs = await api.getProviders();
    setProviders([...provs].sort((a, b) => a.priority - b.priority));
  };

  const handleToggle = async (id: string, nextState: boolean) => {
    setProviders((prev) =>
      prev.map((p) => (p.id === id ? { ...p, active: nextState } : p))
    );
    await api.toggleProvider(id, !nextState);
  };

  const selectStrategy = (strat: RoutingStrategyInfo) => {
    setSelectedStrategy(strat);
    setStrategyModalVisible(false);
    Alert.alert(
      'Strategi Berubah',
      `Strategi perutean aktif saat ini: ${strat.name}.\nEngine load balancer Route-X telah diperbarui.`
    );
  };

  return (
    <View style={styles.container}>
      <TopHeader
        mode="subscreen"
        title="Routing & Failover"
        onBack={onBack}
        onSettings={() => setStrategyModalVisible(true)}
      />

      {/* Segmented Tabs (Routing vs Failover) */}
      <View style={styles.segmentedContainer}>
        <TouchableOpacity
          style={[
            styles.segmentButton,
            activeTab === 'strategy' && styles.segmentButtonActive,
          ]}
          onPress={() => setActiveTab('strategy')}
          activeOpacity={0.8}
        >
          <Text
            style={[
              styles.segmentText,
              activeTab === 'strategy' && styles.segmentTextActive,
            ]}
          >
            Strategi Routing
          </Text>
        </TouchableOpacity>

        <TouchableOpacity
          style={[
            styles.segmentButton,
            activeTab === 'failover' && styles.segmentButtonActive,
          ]}
          onPress={() => setActiveTab('failover')}
          activeOpacity={0.8}
        >
          <Text
            style={[
              styles.segmentText,
              activeTab === 'failover' && styles.segmentTextActive,
            ]}
          >
            Failover & Recovery
          </Text>
        </TouchableOpacity>
      </View>

      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        {activeTab === 'strategy' ? (
          <>
            {/* Kartu Strategi Aktif */}
            <TouchableOpacity
              style={styles.strategyCard}
              onPress={() => setStrategyModalVisible(true)}
              activeOpacity={0.8}
            >
              <View style={styles.strategyTopRow}>
                <View style={styles.strategyIconBox}>
                  <Compass size={22} color={colors.accentPrimary} />
                </View>
                <View style={styles.strategyTitleCol}>
                  <Text style={styles.strategyLabel}>Strategi Aktif (Ketuk untuk ganti)</Text>
                  <Text style={styles.strategyName}>{selectedStrategy.name}</Text>
                </View>
                <View style={styles.badgeActive}>
                  <CheckCircle2 size={13} color={colors.accentPrimary} />
                  <Text style={styles.badgeActiveText}>Aktif</Text>
                </View>
              </View>

              <Text style={styles.strategyDescription}>
                {selectedStrategy.description}
              </Text>
            </TouchableOpacity>

            {/* Section Prioritas Provider */}
            <View style={styles.priorityHeaderRow}>
              <View>
                <Text style={styles.sectionTitle}>Urutan Prioritas Provider</Text>
                <Text style={styles.sectionSubtitle}>
                  Sakelar hijau ON/OFF untuk alih trafik instan 1-sentuhan
                </Text>
              </View>
            </View>

            {providers.slice(0, 6).map((item) => (
              <ProviderToggleItem
                key={item.id}
                id={item.id}
                name={item.name}
                brand={item.brand}
                category={item.category}
                active={item.active}
                priority={item.priority}
                latencyMs={item.latencyMs}
                showPriorityBadge={true}
                onToggle={handleToggle}
              />
            ))}
          </>
        ) : (
          /* Tab Failover */
          <View style={styles.failoverContainer}>
            <View style={styles.failoverCard}>
              <View style={styles.failoverHeader}>
                <ShieldAlert size={20} color={colors.statusWarning} />
                <Text style={styles.failoverTitle}>Otomatisasi Failover Aktif</Text>
              </View>
              <Text style={styles.failoverText}>
                Jika upstream provider mengembalikan status 5xx atau mengalami timeout
                melebihi 2.500ms, sistem secara otomatis merutekan ulang payload ke
                penyedia prioritas berikutnya tanpa mengganggu klien.
              </Text>

              <View style={styles.ruleItem}>
                <Text style={styles.ruleLabel}>Maksimum Percobaan Ulang:</Text>
                <Text style={styles.ruleValue}>3x Percobaan</Text>
              </View>
              <View style={styles.ruleItem}>
                <Text style={styles.ruleLabel}>Threshold Latensi Ambang:</Text>
                <Text style={styles.ruleValue}>2,500 ms</Text>
              </View>
              <View style={styles.ruleItem}>
                <Text style={styles.ruleLabel}>Fallback Cadangan Otomatis:</Text>
                <Text style={styles.ruleValue}>Google Gemini / LLaMA</Text>
              </View>
            </View>

            <TouchableOpacity
              style={styles.simulateButton}
              onPress={() =>
                Alert.alert(
                  'Simulasi Failover Berhasil',
                  'Simulasi berhasil: Payload dialihkan dari OpenAI ke Anthropic dalam 88ms tanpa error koneksi ke pengguna.'
                )
              }
            >
              <Sliders size={16} color="#0A0B0D" />
              <Text style={styles.simulateButtonText}>Uji Simulasi Pemulihan Cepat</Text>
            </TouchableOpacity>
          </View>
        )}
      </ScrollView>

      {/* Modal Ganti Strategi Perutean */}
      <Modal
        visible={strategyModalVisible}
        transparent={true}
        animationType="slide"
        onRequestClose={() => setStrategyModalVisible(false)}
      >
        <View style={styles.modalOverlay}>
          <View style={styles.modalContent}>
            <View style={styles.modalHeader}>
              <Text style={styles.modalTitle}>Pilih Strategi Perutean AI</Text>
              <TouchableOpacity onPress={() => setStrategyModalVisible(false)}>
                <Text style={styles.closeBtn}>✕</Text>
              </TouchableOpacity>
            </View>

            <ScrollView style={styles.modalScroll}>
              {AVAILABLE_STRATEGIES.map((item) => {
                const isCurrent = selectedStrategy.id === item.id;
                return (
                  <TouchableOpacity
                    key={item.id}
                    style={[styles.strategyOptionCard, isCurrent && styles.strategyOptionActive]}
                    onPress={() => selectStrategy(item)}
                  >
                    <View style={styles.optionHeader}>
                      <Text style={[styles.optionTitle, isCurrent && styles.optionTitleActive]}>
                        {item.name}
                      </Text>
                      {isCurrent && (
                        <View style={styles.optionBadgeActive}>
                          <Text style={styles.optionBadgeText}>AKTIF</Text>
                        </View>
                      )}
                    </View>
                    <Text style={styles.optionDesc}>{item.description}</Text>
                  </TouchableOpacity>
                );
              })}
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
    backgroundColor: colors.bgCanvas,
  },
  segmentedContainer: {
    flexDirection: 'row',
    marginHorizontal: 16,
    marginVertical: 12,
    backgroundColor: colors.bgSurface,
    borderRadius: 12,
    padding: 4,
    borderWidth: 1,
    borderColor: colors.border,
  },
  segmentButton: {
    flex: 1,
    paddingVertical: 10,
    alignItems: 'center',
    borderRadius: 8,
  },
  segmentButtonActive: {
    backgroundColor: colors.bgElevated,
    borderWidth: 1,
    borderColor: colors.border,
  },
  segmentText: {
    fontSize: 13,
    fontWeight: '600',
    color: colors.textSecondary,
  },
  segmentTextActive: {
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
  strategyCard: {
    backgroundColor: colors.bgSurface,
    borderRadius: 16,
    borderWidth: 1,
    borderColor: colors.accentGreenBorder,
    padding: 16,
    marginBottom: 20,
  },
  strategyTopRow: {
    flexDirection: 'row',
    alignItems: 'center',
    marginBottom: 12,
  },
  strategyIconBox: {
    width: 44,
    height: 44,
    borderRadius: 12,
    backgroundColor: colors.accentGreenSubtle,
    justifyContent: 'center',
    alignItems: 'center',
    marginRight: 12,
    borderWidth: 1,
    borderColor: colors.accentGreenBorder,
  },
  strategyTitleCol: {
    flex: 1,
  },
  strategyLabel: {
    fontSize: 11,
    fontWeight: '600',
    color: colors.textSecondary,
    textTransform: 'uppercase',
    letterSpacing: 0.5,
  },
  strategyName: {
    fontSize: 17,
    fontWeight: '800',
    color: colors.textPrimary,
  },
  badgeActive: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
    backgroundColor: colors.accentGreenSubtle,
    paddingHorizontal: 8,
    paddingVertical: 3,
    borderRadius: 8,
  },
  badgeActiveText: {
    fontSize: 11,
    fontWeight: '700',
    color: colors.accentPrimary,
  },
  strategyDescription: {
    fontSize: 13,
    color: colors.textSecondary,
    lineHeight: 19,
  },
  priorityHeaderRow: {
    marginBottom: 12,
  },
  sectionTitle: {
    fontSize: 15,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  sectionSubtitle: {
    fontSize: 12,
    color: colors.textMuted,
    marginTop: 2,
  },
  failoverContainer: {
    gap: 16,
  },
  failoverCard: {
    backgroundColor: colors.bgSurface,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
  },
  failoverHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    marginBottom: 10,
  },
  failoverTitle: {
    fontSize: 15,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  failoverText: {
    fontSize: 13,
    color: colors.textSecondary,
    lineHeight: 19,
    marginBottom: 16,
  },
  ruleItem: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    paddingVertical: 8,
    borderTopWidth: 1,
    borderTopColor: colors.border,
  },
  ruleLabel: {
    fontSize: 13,
    color: colors.textSecondary,
  },
  ruleValue: {
    fontSize: 13,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  simulateButton: {
    backgroundColor: colors.accentPrimary,
    borderRadius: 12,
    paddingVertical: 14,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 8,
  },
  simulateButtonText: {
    fontSize: 14,
    fontWeight: '700',
    color: '#0A0B0D',
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
    maxHeight: '80%',
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
  modalScroll: {
    maxHeight: 380,
  },
  strategyOptionCard: {
    backgroundColor: colors.bgElevated,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 14,
    marginBottom: 10,
  },
  strategyOptionActive: {
    borderColor: colors.accentPrimary,
    backgroundColor: colors.accentGreenSubtle,
  },
  optionHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 6,
  },
  optionTitle: {
    fontSize: 14,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  optionTitleActive: {
    color: colors.accentPrimary,
  },
  optionBadgeActive: {
    backgroundColor: colors.accentPrimary,
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 4,
  },
  optionBadgeText: {
    fontSize: 9,
    fontWeight: '800',
    color: '#0A0B0D',
  },
  optionDesc: {
    fontSize: 12,
    color: colors.textSecondary,
    lineHeight: 17,
  },
});
