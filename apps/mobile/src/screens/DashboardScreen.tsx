import React from 'react';
import { View, Text, StyleSheet, ScrollView } from 'react-native';
import Svg, { Polyline, Rect } from 'react-native-svg';
import { Activity, Zap, Cpu, Clock } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';

export interface DashboardScreenProps {
  onBack: () => void;
}

export const DashboardScreen: React.FC<DashboardScreenProps> = ({ onBack }) => {
  return (
    <View style={styles.container}>
      <TopHeader mode="subscreen" title="Dashboard Runtime" onBack={onBack} />

      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        {/* Throughput Chart Card */}
        <View style={styles.chartCard}>
          <View style={styles.cardHeader}>
            <View style={styles.headerLeft}>
              <Activity size={18} color={colors.accentPrimary} />
              <Text style={styles.cardTitle}>Gateway Throughput (Req/s)</Text>
            </View>
            <Text style={styles.statValue}>148 req/s</Text>
          </View>

          <View style={styles.svgWrapper}>
            <Svg width="100%" height={120} viewBox="0 0 320 120">
              <Rect x="0" y="0" width="320" height="120" fill="transparent" />
              <Polyline
                points="10,100 40,80 80,95 120,40 160,55 200,20 240,35 280,15 310,25"
                fill="none"
                stroke={colors.accentPrimary}
                strokeWidth="2.5"
                strokeLinecap="round"
              />
            </Svg>
          </View>
          <View style={styles.timeAxis}>
            <Text style={styles.axisLabel}>00:00</Text>
            <Text style={styles.axisLabel}>06:00</Text>
            <Text style={styles.axisLabel}>12:00</Text>
            <Text style={styles.axisLabel}>18:00</Text>
            <Text style={styles.axisLabel}>Sekarang</Text>
          </View>
        </View>

        {/* Latensi & Token Usage */}
        <View style={styles.statsRow}>
          <View style={styles.miniStatCard}>
            <View style={styles.miniIconCircle}>
              <Clock size={16} color={colors.accentLime} />
            </View>
            <Text style={styles.miniLabel}>P95 Latency</Text>
            <Text style={styles.miniValue}>164 ms</Text>
            <Text style={styles.miniSub}>-18ms vs kemarin</Text>
          </View>

          <View style={styles.miniStatCard}>
            <View style={styles.miniIconCircle}>
              <Zap size={16} color={colors.avatarOperator} />
            </View>
            <Text style={styles.miniLabel}>Total Tokens</Text>
            <Text style={styles.miniValue}>2.4M</Text>
            <Text style={styles.miniSub}>+340K hari ini</Text>
          </View>
        </View>

        {/* Provider Load Distribution */}
        <View style={styles.distributionCard}>
          <View style={styles.cardHeader}>
            <View style={styles.headerLeft}>
              <Cpu size={18} color={colors.accentPrimary} />
              <Text style={styles.cardTitle}>Distribusi Beban Upstream</Text>
            </View>
          </View>

          <View style={styles.distBarWrapper}>
            <View style={[styles.barSegment, { flex: 45, backgroundColor: colors.accentPrimary }]} />
            <View style={[styles.barSegment, { flex: 30, backgroundColor: colors.avatarDev }]} />
            <View style={[styles.barSegment, { flex: 15, backgroundColor: colors.avatarOperator }]} />
            <View style={[styles.barSegment, { flex: 10, backgroundColor: colors.avatarUser }]} />
          </View>

          <View style={styles.legendRow}>
            <View style={styles.legendItem}>
              <View style={[styles.legendDot, { backgroundColor: colors.accentPrimary }]} />
              <Text style={styles.legendText}>OpenAI (45%)</Text>
            </View>
            <View style={styles.legendItem}>
              <View style={[styles.legendDot, { backgroundColor: colors.avatarDev }]} />
              <Text style={styles.legendText}>Anthropic (30%)</Text>
            </View>
            <View style={styles.legendItem}>
              <View style={[styles.legendDot, { backgroundColor: colors.avatarOperator }]} />
              <Text style={styles.legendText}>Google (15%)</Text>
            </View>
          </View>
        </View>
      </ScrollView>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.bgCanvas,
  },
  scrollArea: {
    flex: 1,
  },
  scrollContent: {
    padding: 16,
    paddingBottom: 24,
  },
  chartCard: {
    backgroundColor: colors.bgSurface,
    borderRadius: 16,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
    marginBottom: 16,
  },
  cardHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 12,
  },
  headerLeft: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
  },
  cardTitle: {
    fontSize: 14,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  statValue: {
    fontSize: 16,
    fontWeight: '800',
    color: colors.accentPrimary,
  },
  svgWrapper: {
    height: 120,
    justifyContent: 'center',
  },
  timeAxis: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    marginTop: 6,
    paddingHorizontal: 6,
  },
  axisLabel: {
    fontSize: 10,
    color: colors.textMuted,
  },
  statsRow: {
    flexDirection: 'row',
    gap: 12,
    marginBottom: 16,
  },
  miniStatCard: {
    flex: 1,
    backgroundColor: colors.bgSurface,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 14,
  },
  miniIconCircle: {
    width: 32,
    height: 32,
    borderRadius: 8,
    backgroundColor: colors.bgElevated,
    justifyContent: 'center',
    alignItems: 'center',
    marginBottom: 8,
  },
  miniLabel: {
    fontSize: 12,
    color: colors.textSecondary,
    marginBottom: 2,
  },
  miniValue: {
    fontSize: 20,
    fontWeight: '800',
    color: colors.textPrimary,
    marginBottom: 2,
  },
  miniSub: {
    fontSize: 11,
    color: colors.accentPrimary,
    fontWeight: '600',
  },
  distributionCard: {
    backgroundColor: colors.bgSurface,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
  },
  distBarWrapper: {
    flexDirection: 'row',
    height: 10,
    borderRadius: 5,
    overflow: 'hidden',
    marginBottom: 12,
    backgroundColor: colors.bgElevated,
  },
  barSegment: {
    height: '100%',
  },
  legendRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 12,
  },
  legendItem: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  legendDot: {
    width: 8,
    height: 8,
    borderRadius: 4,
  },
  legendText: {
    fontSize: 12,
    color: colors.textSecondary,
  },
});
