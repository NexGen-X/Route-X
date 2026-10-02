import React from 'react';
import { View, Text, StyleSheet, TouchableOpacity } from 'react-native';
import Svg, { Polyline } from 'react-native-svg';
import { TrendingUp, TrendingDown } from 'lucide-react-native';
import { colors } from '../theme/colors';

export interface MetricCardProps {
  label: string;
  value: string;
  trend: string;
  isPositive: boolean;
  sparklineColor?: string;
  points: number[];
  onPress?: () => void;
}

export const MetricCard: React.FC<MetricCardProps> = ({
  label,
  value,
  trend,
  isPositive,
  sparklineColor = colors.accentPrimary,
  points,
  onPress,
}) => {
  // Hitung koordinat sparkline SVG
  const svgWidth = 84;
  const svgHeight = 28;
  const paddingVertical = 3;

  const minVal = points.length > 0 ? Math.min(...points) : 0;
  const maxVal = points.length > 0 ? Math.max(...points) : 1;
  const range = maxVal - minVal === 0 ? 1 : maxVal - minVal;

  const polylinePoints = points
    .map((pt, idx) => {
      const x = points.length > 1 ? (idx / (points.length - 1)) * svgWidth : 0;
      const y =
        svgHeight -
        paddingVertical -
        ((pt - minVal) / range) * (svgHeight - paddingVertical * 2);
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(' ');

  const TrendIcon = isPositive ? TrendingUp : TrendingDown;
  const trendBgColor = isPositive ? colors.accentGreenSubtle : 'rgba(239, 68, 68, 0.12)';
  const trendTextColor = isPositive ? colors.accentPrimary : colors.statusError;

  return (
    <TouchableOpacity
      activeOpacity={0.8}
      onPress={onPress}
      disabled={!onPress}
      style={styles.card}
    >
      <View style={styles.topRow}>
        <Text style={styles.label} numberOfLines={1}>
          {label}
        </Text>
        <View style={[styles.trendBadge, { backgroundColor: trendBgColor }]}>
          <TrendIcon size={12} color={trendTextColor} />
          <Text style={[styles.trendText, { color: trendTextColor }]}>{trend}</Text>
        </View>
      </View>

      <View style={styles.bottomRow}>
        <Text style={styles.value} numberOfLines={1}>
          {value}
        </Text>
        <View style={styles.sparklineContainer}>
          <Svg width={svgWidth} height={svgHeight}>
            <Polyline
              points={polylinePoints}
              fill="none"
              stroke={sparklineColor}
              strokeWidth="2.2"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </Svg>
        </View>
      </View>
    </TouchableOpacity>
  );
};

const styles = StyleSheet.create({
  card: {
    flex: 1,
    backgroundColor: colors.bgSurface,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 14,
    minHeight: 104,
    justifyContent: 'space-between',
  },
  topRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginBottom: 8,
  },
  label: {
    fontSize: 12,
    fontWeight: '600',
    color: colors.textSecondary,
    flex: 1,
    marginRight: 4,
  },
  trendBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 6,
    gap: 3,
  },
  trendText: {
    fontSize: 11,
    fontWeight: '700',
  },
  bottomRow: {
    flexDirection: 'row',
    alignItems: 'flex-end',
    justifyContent: 'space-between',
  },
  value: {
    fontSize: 22,
    fontWeight: '800',
    color: colors.textPrimary,
    letterSpacing: -0.5,
  },
  sparklineContainer: {
    height: 28,
    justifyContent: 'center',
  },
});
