import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TouchableOpacity,
  RefreshControl,
} from 'react-native';
import Svg, { Path } from 'react-native-svg';
import { Clock, ArrowUpRight, ShieldCheck, Zap } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { MetricCard } from '../components/MetricCard';
import { api } from '../api/client';
import { SystemOverview, ActivityItem } from '../api/types';

export interface HomeScreenProps {
  onNavigateToScreen?: (screenName: string) => void;
}

export const HomeScreen: React.FC<HomeScreenProps> = ({ onNavigateToScreen }) => {
  const [overview, setOverview] = useState<SystemOverview | null>(null);
  const [activities, setActivities] = useState<ActivityItem[]>([]);
  const [refreshing, setRefreshing] = useState(false);

  const loadData = async () => {
    const [overviewData, activitiesData] = await Promise.all([
      api.getOverview(),
      api.getActivities(),
    ]);
    setOverview(overviewData);
    setActivities(activitiesData);
  };

  useEffect(() => {
    loadData();
  }, []);

  const onRefresh = async () => {
    setRefreshing(true);
    await loadData();
    setRefreshing(false);
  };

  return (
    <ScrollView
      style={styles.container}
      contentContainerStyle={styles.contentContainer}
      showsVerticalScrollIndicator={false}
      refreshControl={
        <RefreshControl
          refreshing={refreshing}
          onRefresh={onRefresh}
          tintColor={colors.accentPrimary}
          colors={[colors.accentPrimary]}
        />
      }
    >
      {/* Hero Card dengan Gelombang Neon Hijau */}
      <View style={styles.heroCard}>
        <View style={styles.heroGlowBackdrop} />

        <View style={styles.heroContent}>
          <View style={styles.heroBadge}>
            <Zap size={13} color={colors.accentPrimary} />
            <Text style={styles.heroBadgeText}>AI GATEWAY ACTIVE</Text>
          </View>
          <Text style={styles.heroTitle}>Selamat datang,</Text>
          <Text style={styles.heroSubtitle}>Administrator</Text>
          <Text style={styles.heroCaption}>
            Sistem runtime beroperasi optimal tanpa gangguan.
          </Text>
        </View>

        {/* Gelombang Neon Kanan (Wave Flow Vector) */}
        <View style={styles.waveSvgContainer} pointerEvents="none">
          <Svg width={140} height={120} viewBox="0 0 140 120" fill="none">
            <Path
              d="M0 60C30 30 70 90 100 40C120 10 135 70 140 90V120H0V60Z"
              fill="rgba(34, 197, 94, 0.08)"
            />
            <Path
              d="M10 70C40 40 80 80 110 35C125 15 138 65 140 80"
              stroke={colors.accentPrimary}
              strokeWidth="2"
              strokeOpacity="0.4"
            />
            <Path
              d="M0 85C35 55 75 95 105 50C120 30 135 75 140 90"
              stroke={colors.accentLime}
              strokeWidth="1.5"
              strokeOpacity="0.6"
            />
          </Svg>
        </View>
      </View>

      {/* 2x2 Metrics Grid */}
      {overview && (
        <View style={styles.metricsGrid}>
          <View style={styles.gridRow}>
            <MetricCard
              label={overview.totalRequests.label}
              value={overview.totalRequests.value}
              trend={overview.totalRequests.trend}
              isPositive={overview.totalRequests.isPositive}
              sparklineColor={overview.totalRequests.sparklineColor}
              points={overview.totalRequests.points}
              onPress={() => onNavigateToScreen && onNavigateToScreen('dashboard')}
            />
            <View style={styles.gridSpacer} />
            <MetricCard
              label={overview.activeProviders.label}
              value={overview.activeProviders.value}
              trend={overview.activeProviders.trend}
              isPositive={overview.activeProviders.isPositive}
              sparklineColor={overview.activeProviders.sparklineColor}
              points={overview.activeProviders.points}
              onPress={() => onNavigateToScreen && onNavigateToScreen('providers')}
            />
          </View>

          <View style={styles.gridSpacerY} />

          <View style={styles.gridRow}>
            <MetricCard
              label={overview.totalUsers.label}
              value={overview.totalUsers.value}
              trend={overview.totalUsers.trend}
              isPositive={overview.totalUsers.isPositive}
              sparklineColor={overview.totalUsers.sparklineColor}
              points={overview.totalUsers.points}
              onPress={() => onNavigateToScreen && onNavigateToScreen('users')}
            />
            <View style={styles.gridSpacer} />
            <MetricCard
              label={overview.budgetUsage.label}
              value={overview.budgetUsage.value}
              trend={overview.budgetUsage.trend}
              isPositive={overview.budgetUsage.isPositive}
              sparklineColor={overview.budgetUsage.sparklineColor}
              points={overview.budgetUsage.points}
              onPress={() => onNavigateToScreen && onNavigateToScreen('apikeys')}
            />
          </View>
        </View>
      )}

      {/* Aktivitas Terbaru Section */}
      <View style={styles.sectionHeader}>
        <View style={styles.sectionTitleRow}>
          <Clock size={16} color={colors.accentPrimary} />
          <Text style={styles.sectionTitle}>Aktivitas Terbaru</Text>
        </View>
        <TouchableOpacity
          style={styles.seeAllButton}
          onPress={() => onNavigateToScreen && onNavigateToScreen('logs')}
        >
          <Text style={styles.seeAllText}>Lihat Semua</Text>
          <ArrowUpRight size={14} color={colors.accentPrimary} />
        </TouchableOpacity>
      </View>

      <View style={styles.activityList}>
        {activities.map((item) => (
          <View key={item.id} style={styles.activityItem}>
            <View style={styles.statusDotWrapper}>
              <View
                style={[
                  styles.activityDot,
                  item.status === 'success' && styles.dotSuccess,
                  item.status === 'warning' && styles.dotWarning,
                  item.status === 'error' && styles.dotError,
                ]}
              />
            </View>

            <View style={styles.activityDetails}>
              <View style={styles.activityTopLine}>
                <Text style={styles.activityTitle}>{item.title}</Text>
                <Text style={styles.activityTime}>{item.timestamp}</Text>
              </View>

              <View style={styles.activityPathRow}>
                <Text style={styles.methodBadge}>{item.method}</Text>
                <Text style={styles.activityEndpoint} numberOfLines={1}>
                  {item.endpoint}
                </Text>
              </View>
            </View>
          </View>
        ))}
      </View>

      {/* Quick Security Badge Footer */}
      <View style={styles.securityBanner}>
        <ShieldCheck size={18} color={colors.accentPrimary} />
        <Text style={styles.securityText}>
          Egress TLS 1.3 Terenkripsi • Gateway v1.3.1
        </Text>
      </View>
    </ScrollView>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.bgCanvas,
  },
  contentContainer: {
    padding: 16,
    paddingBottom: 24,
  },
  heroCard: {
    backgroundColor: colors.bgSurface,
    borderRadius: 18,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 20,
    marginBottom: 16,
    position: 'relative',
    overflow: 'hidden',
  },
  heroGlowBackdrop: {
    position: 'absolute',
    top: -30,
    right: -20,
    width: 140,
    height: 140,
    borderRadius: 70,
    backgroundColor: 'rgba(34, 197, 94, 0.08)',
  },
  heroContent: {
    zIndex: 2,
  },
  heroBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: colors.accentGreenSubtle,
    alignSelf: 'flex-start',
    paddingHorizontal: 8,
    paddingVertical: 3,
    borderRadius: 6,
    gap: 5,
    marginBottom: 10,
    borderWidth: 1,
    borderColor: colors.accentGreenBorder,
  },
  heroBadgeText: {
    fontSize: 10,
    fontWeight: '800',
    color: colors.accentPrimary,
    letterSpacing: 0.5,
  },
  heroTitle: {
    fontSize: 14,
    color: colors.textSecondary,
    fontWeight: '500',
  },
  heroSubtitle: {
    fontSize: 24,
    fontWeight: '800',
    color: colors.textPrimary,
    marginBottom: 6,
    letterSpacing: -0.5,
  },
  heroCaption: {
    fontSize: 13,
    color: colors.textMuted,
    maxWidth: '75%',
    lineHeight: 18,
  },
  waveSvgContainer: {
    position: 'absolute',
    bottom: 0,
    right: 0,
    width: 140,
    height: 120,
    zIndex: 1,
  },
  metricsGrid: {
    marginBottom: 20,
  },
  gridRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
  },
  gridSpacer: {
    width: 12,
  },
  gridSpacerY: {
    height: 12,
  },
  sectionHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 12,
  },
  sectionTitleRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
  },
  sectionTitle: {
    fontSize: 16,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  seeAllButton: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
  },
  seeAllText: {
    fontSize: 13,
    fontWeight: '600',
    color: colors.accentPrimary,
  },
  activityList: {
    backgroundColor: colors.bgSurface,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.border,
    overflow: 'hidden',
    marginBottom: 16,
  },
  activityItem: {
    flexDirection: 'row',
    alignItems: 'center',
    padding: 14,
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
  },
  statusDotWrapper: {
    marginRight: 12,
    justifyContent: 'center',
    alignItems: 'center',
  },
  activityDot: {
    width: 8,
    height: 8,
    borderRadius: 4,
  },
  dotSuccess: {
    backgroundColor: colors.accentPrimary,
  },
  dotWarning: {
    backgroundColor: colors.statusWarning,
  },
  dotError: {
    backgroundColor: colors.statusError,
  },
  activityDetails: {
    flex: 1,
  },
  activityTopLine: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 3,
  },
  activityTitle: {
    fontSize: 14,
    fontWeight: '600',
    color: colors.textPrimary,
  },
  activityTime: {
    fontSize: 11,
    color: colors.textMuted,
  },
  activityPathRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
  },
  methodBadge: {
    fontSize: 10,
    fontWeight: '700',
    color: colors.textSecondary,
    backgroundColor: colors.bgElevated,
    paddingHorizontal: 5,
    paddingVertical: 1,
    borderRadius: 4,
  },
  activityEndpoint: {
    fontSize: 12,
    color: colors.textSecondary,
    fontFamily: 'monospace',
    flex: 1,
  },
  securityBanner: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: colors.bgSurface,
    borderRadius: 10,
    padding: 10,
    gap: 8,
    borderWidth: 1,
    borderColor: colors.border,
  },
  securityText: {
    fontSize: 12,
    color: colors.textMuted,
    fontWeight: '500',
  },
});
