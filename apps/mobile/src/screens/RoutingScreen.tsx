import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TouchableOpacity,
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

export const RoutingScreen: React.FC<RoutingScreenProps> = ({ onBack }) => {
  const [activeTab, setActiveTab] = useState<'strategy' | 'failover'>('strategy');
  const [strategy, setStrategy] = useState<RoutingStrategyInfo | null>(null);
  const [providers, setProviders] = useState<ProviderItem[]>([]);

  useEffect(() => {
    loadData();
  }, []);

  const loadData = async () => {
    const [strat, provs] = await Promise.all([
      api.getRoutingStrategy(),
      api.getProviders(),
    ]);
    setStrategy(strat);
    // Urutkan berdasarkan prioritas 1..5
    setProviders(provs.sort((a, b) => a.priority - b.priority));
  };

  const handleToggle = async (id: string, nextState: boolean) => {
    setProviders((prev) =>
      prev.map((p) => (p.id === id ? { ...p, active: nextState } : p))
    );
    await api.toggleProvider(id, !nextState);
  };

  return (
    <View style={styles.container}>
      <TopHeader
        mode="subscreen"
        title="Routing & Failover"
        onBack={onBack}
        onSettings={() =>
          Alert.alert('Routing Rules', 'Konfigurasi parameter load balancer & auto-failover.')
        }
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
            {strategy && (
              <View style={styles.strategyCard}>
                <View style={styles.strategyTopRow}>
                  <View style={styles.strategyIconBox}>
                    <Compass size={22} color={colors.accentPrimary} />
                  </View>
                  <View style={styles.strategyTitleCol}>
                    <Text style={styles.strategyLabel}>Strategi Saat Ini</Text>
                    <Text style={styles.strategyName}>{strategy.name}</Text>
                  </View>
                  <View style={styles.badgeActive}>
                    <CheckCircle2 size={13} color={colors.accentPrimary} />
                    <Text style={styles.badgeActiveText}>Aktif</Text>
                  </View>
                </View>

                <Text style={styles.strategyDescription}>
                  {strategy.description}
                </Text>
              </View>
            )}

            {/* Section Prioritas Provider */}
            <View style={styles.priorityHeaderRow}>
              <View>
                <Text style={styles.sectionTitle}>Daftar Prioritas Provider</Text>
                <Text style={styles.sectionSubtitle}>
                  Sakelar hijau ON/OFF untuk alih trafik instan 1-sentuhan
                </Text>
              </View>
            </View>

            {providers.slice(0, 5).map((item) => (
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
                <Text style={styles.ruleLabel}>Fallback Cadangan:</Text>
                <Text style={styles.ruleValue}>Meta LLaMA Local</Text>
              </View>
            </View>

            <TouchableOpacity
              style={styles.simulateButton}
              onPress={() =>
                Alert.alert(
                  'Simulasi Failover',
                  'Simulasi uji coba peralihan beban otomatis berhasil diverifikasi.'
                )
              }
            >
              <Sliders size={16} color="#0A0B0D" />
              <Text style={styles.simulateButtonText}>Uji Simulasi Pemulihan Cepat</Text>
            </TouchableOpacity>
          </View>
        )}
      </ScrollView>
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
});
