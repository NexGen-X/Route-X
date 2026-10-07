import React, { useState } from 'react';
import { View, Text, StyleSheet, ScrollView, TouchableOpacity } from 'react-native';
import Svg, { Polyline, Rect } from 'react-native-svg';
import { Activity, Zap, Cpu, Clock, CheckCircle2, AlertTriangle, Layers } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';

export interface DashboardScreenProps {
  onBack: () => void;
}

const TIME_RANGES = ['1 Jam', '24 Jam', '7 Hari', '30 Hari'];

export const DashboardScreen: React.FC<DashboardScreenProps> = ({ onBack }) => {
  const [selectedRange, setSelectedRange] = useState('24 Jam');

  const getPointsForRange = () => {
    switch (selectedRange) {
      case '1 Jam':
        return '10,90 40,85 80,60 120,40 160,70 200,30 240,45 280,20 310,25';
      case '7 Hari':
        return '10,70 40,65 80,75 120,50 160,40 200,35 240,25 280,20 310,15';
      case '30 Hari':
        return '10,80 40,75 80,70 120,60 160,50 200,45 240,30 280,20 310,18';
      default: // 24 Jam
        return '10,100 40,80 80,95 120,40 160,55 200,20 240,35 280,15 310,25';
    }
  };

  return (
    <View style={styles.container}>
      <TopHeader mode="subscreen" title="Observabilitas Gateway" onBack={onBack} />

      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        {/* Time Range Chips */}
        <View style={styles.rangeRow}>
          {TIME_RANGES.map((rng) => {
            const isActive = selectedRange === rng;
            return (
              <TouchableOpacity
                key={rng}
                style={[styles.rangeChip, isActive && styles.rangeChipActive]}
                onPress={() => setSelectedRange(rng)}
              >
                <Text style={[styles.rangeText, isActive && styles.rangeTextActive]}>
                  {rng}
                </Text>
              </TouchableOpacity>
            );
          })}
        </View>

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
                points={getPointsForRange()}
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

        {/* Latency Percentile Matrix */}
        <View style={styles.matrixCard}>
          <View style={styles.cardHeader}>
            <View style={styles.headerLeft}>
              <Clock size={18} color={colors.accentLime} />
              <Text style={styles.cardTitle}>Matriks Latensi Gateway</Text>
            </View>
            <Text style={styles.liveIndicator}>Live Realtime</Text>
          </View>

          <View style={styles.percentileGrid}>
            <View style={styles.percentileCol}>
              <Text style={styles.percentileLabel}>P50 Median</Text>
              <Text style={styles.percentileVal}>98 ms</Text>
              <Text style={styles.percentileSub}>Sangat cepat</Text>
            </View>
            <View style={styles.percentileCol}>
              <Text style={styles.percentileLabel}>P90 Normal</Text>
              <Text style={styles.percentileVal}>142 ms</Text>
              <Text style={styles.percentileSub}>90% request</Text>
            </View>
            <View style={styles.percentileCol}>
              <Text style={styles.percentileLabel}>P99 Ekor (Tail)</Text>
              <Text style={[styles.percentileVal, { color: colors.warning }]}>210 ms</Text>
              <Text style={styles.percentileSub}>Worst case</Text>
            </View>
          </View>
        </View>

        {/* Reliability & SLA Matrix */}
        <View style={styles.slaCard}>
          <View style={styles.slaCol}>
            <View style={styles.slaHeaderRow}>
              <CheckCircle2 size={16} color={colors.accentLime} />
              <Text style={styles.slaTitle}>Gateway SLA</Text>
            </View>
            <Text style={styles.slaNumber}>99.98%</Text>
            <Text style={styles.slaNote}>Uptime 30 hari</Text>
          </View>

          <View style={styles.slaDivider} />

          <View style={styles.slaCol}>
            <View style={styles.slaHeaderRow}>
              <AlertTriangle size={16} color={colors.avatarDev} />
              <Text style={styles.slaTitle}>Error Rate</Text>
            </View>
            <Text style={styles.slaNumber}>0.02%</Text>
            <Text style={styles.slaNote}>429 / 5xx error</Text>
          </View>

          <View style={styles.slaDivider} />

          <View style={styles.slaCol}>
            <View style={styles.slaHeaderRow}>
              <Zap size={16} color={colors.accentPrimary} />
              <Text style={styles.slaTitle}>Cache Hit</Text>
            </View>
            <Text style={styles.slaNumber}>34.2%</Text>
            <Text style={styles.slaNote}>Hemat $842/bln</Text>
          </View>
        </View>

        {/* Provider Load Distribution */}
        <View style={styles.distributionCard}>
          <View style={styles.cardHeader}>
            <View style={styles.headerLeft}>
              <Cpu size={18} color={colors.accentPrimary} />
              <Text style={styles.cardTitle}>Distribusi Beban Upstream</Text>
            </View>
            <View style={styles.distBadge}>
              <Layers size={12} color={colors.textMuted} />
              <Text style={styles.distBadgeText}>4 Kluster Aktif</Text>
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
            <View style={styles.legendItem}>
              <View style={[styles.legendDot, { backgroundColor: colors.avatarUser }]} />
              <Text style={styles.legendText}>Lainnya (10%)</Text>
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
    backgroundColor: colors.bgBase,
  },
  scrollArea: {
    flex: 1,
  },
  scrollContent: {
    padding: 16,
    paddingBottom: 40,
  },
  rangeRow: {
    flexDirection: 'row',
    gap: 8,
    marginBottom: 16,
  },
  rangeChip: {
    flex: 1,
    paddingVertical: 8,
    borderRadius: 8,
    backgroundColor: colors.bgCard,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    alignItems: 'center',
  },
  rangeChipActive: {
    borderColor: colors.accentPrimary,
    backgroundColor: 'rgba(56, 189, 248, 0.12)',
  },
  rangeText: {
    color: colors.textMuted,
    fontSize: 12,
    fontWeight: '600',
  },
  rangeTextActive: {
    color: colors.accentPrimary,
    fontWeight: '700',
  },
  chartCard: {
    backgroundColor: colors.bgCard,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    padding: 16,
    marginBottom: 14,
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
    width: '100%',
    justifyContent: 'center',
  },
  timeAxis: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    marginTop: 8,
    paddingHorizontal: 8,
  },
  axisLabel: {
    fontSize: 10,
    color: colors.textMuted,
  },
  matrixCard: {
    backgroundColor: colors.bgCard,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    padding: 16,
    marginBottom: 14,
  },
  liveIndicator: {
    color: colors.accentLime,
    fontSize: 11,
    fontWeight: '700',
    backgroundColor: 'rgba(163, 230, 53, 0.1)',
    paddingHorizontal: 7,
    paddingVertical: 2,
    borderRadius: 4,
  },
  percentileGrid: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    gap: 8,
    marginTop: 6,
  },
  percentileCol: {
    flex: 1,
    backgroundColor: colors.bgElevated,
    borderRadius: 10,
    padding: 12,
    alignItems: 'center',
  },
  percentileLabel: {
    color: colors.textMuted,
    fontSize: 11,
    marginBottom: 4,
  },
  percentileVal: {
    color: colors.textPrimary,
    fontSize: 18,
    fontWeight: '800',
    marginBottom: 2,
  },
  percentileSub: {
    color: colors.textMuted,
    fontSize: 10,
  },
  slaCard: {
    backgroundColor: colors.bgCard,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    padding: 14,
    flexDirection: 'row',
    justifyContent: 'space-between',
    marginBottom: 14,
  },
  slaCol: {
    flex: 1,
    alignItems: 'center',
  },
  slaHeaderRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
    marginBottom: 6,
  },
  slaTitle: {
    color: colors.textMuted,
    fontSize: 11,
    fontWeight: '600',
  },
  slaNumber: {
    color: colors.textPrimary,
    fontSize: 16,
    fontWeight: '800',
    marginBottom: 2,
  },
  slaNote: {
    color: colors.textMuted,
    fontSize: 10,
  },
  slaDivider: {
    width: 1,
    backgroundColor: colors.borderSubtle,
    marginVertical: 4,
  },
  distributionCard: {
    backgroundColor: colors.bgCard,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    padding: 16,
  },
  distBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
    backgroundColor: colors.bgElevated,
    paddingHorizontal: 8,
    paddingVertical: 3,
    borderRadius: 6,
  },
  distBadgeText: {
    color: colors.textMuted,
    fontSize: 11,
  },
  distBarWrapper: {
    height: 12,
    borderRadius: 6,
    flexDirection: 'row',
    overflow: 'hidden',
    gap: 2,
    marginVertical: 12,
  },
  barSegment: {
    height: '100%',
  },
  legendRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 12,
    marginTop: 4,
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
