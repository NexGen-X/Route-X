import React from 'react';
import { View, Text, StyleSheet, Switch, TouchableOpacity } from 'react-native';
import { Cpu } from 'lucide-react-native';
import { colors } from '../theme/colors';

export interface ProviderToggleItemProps {
  id: string;
  name: string;
  brand: string;
  category?: string;
  active: boolean;
  priority?: number;
  modelCount?: number;
  latencyMs?: number;
  onToggle: (id: string, nextState: boolean) => void;
  onPressDetails?: (id: string) => void;
  showPriorityBadge?: boolean;
}

export const ProviderToggleItem: React.FC<ProviderToggleItemProps> = ({
  id,
  name,
  category,
  active,
  priority,
  modelCount,
  latencyMs,
  onToggle,
  onPressDetails,
  showPriorityBadge = false,
}) => {
  const getBrandBadge = (brandName: string) => {
    const initial = brandName.charAt(0).toUpperCase();
    return (
      <View style={styles.brandIconBox}>
        <Cpu size={16} color={active ? colors.accentPrimary : colors.textMuted} />
        <Text style={[styles.brandInitial, { color: active ? colors.textPrimary : colors.textMuted }]}>
          {initial}
        </Text>
      </View>
    );
  };

  return (
    <View style={[styles.container, !active && styles.containerInactive]}>
      {showPriorityBadge && priority !== undefined && (
        <View style={styles.priorityBox}>
          <Text style={styles.priorityText}>#{priority}</Text>
        </View>
      )}

      {getBrandBadge(name)}

      <TouchableOpacity
        style={styles.infoCol}
        activeOpacity={0.7}
        onPress={() => onPressDetails && onPressDetails(id)}
      >
        <View style={styles.nameRow}>
          <Text style={[styles.providerName, !active && styles.textInactive]}>{name}</Text>
          {active && (
            <View style={styles.activePill}>
              <View style={styles.greenDot} />
              <Text style={styles.activePillText}>Aktif</Text>
            </View>
          )}
        </View>

        <View style={styles.metaRow}>
          {category && (
            <Text style={styles.metaText} numberOfLines={1}>
              {category}
            </Text>
          )}
          {modelCount !== undefined && (
            <>
              <Text style={styles.metaSeparator}>•</Text>
              <Text style={styles.metaText}>{modelCount} Model</Text>
            </>
          )}
          {latencyMs !== undefined && (
            <>
              <Text style={styles.metaSeparator}>•</Text>
              <Text style={styles.latencyText}>{latencyMs}ms</Text>
            </>
          )}
        </View>
      </TouchableOpacity>

      <View style={styles.toggleCol}>
        <Switch
          value={active}
          onValueChange={(val) => onToggle(id, val)}
          trackColor={{
            false: '#212631',
            true: colors.accentPrimary,
          }}
          thumbColor={colors.textPrimary}
          ios_backgroundColor="#212631"
        />
      </View>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    backgroundColor: colors.bgSurface,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 14,
    flexDirection: 'row',
    alignItems: 'center',
    marginBottom: 10,
    gap: 12,
  },
  containerInactive: {
    opacity: 0.65,
    backgroundColor: '#0F1217',
  },
  priorityBox: {
    width: 24,
    height: 24,
    borderRadius: 6,
    backgroundColor: colors.bgElevated,
    justifyContent: 'center',
    alignItems: 'center',
  },
  priorityText: {
    fontSize: 11,
    fontWeight: '700',
    color: colors.textSecondary,
  },
  brandIconBox: {
    width: 40,
    height: 40,
    borderRadius: 10,
    backgroundColor: colors.bgElevated,
    borderWidth: 1,
    borderColor: colors.border,
    justifyContent: 'center',
    alignItems: 'center',
    flexDirection: 'row',
    gap: 2,
  },
  brandInitial: {
    fontSize: 12,
    fontWeight: '800',
  },
  infoCol: {
    flex: 1,
  },
  nameRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    marginBottom: 3,
  },
  providerName: {
    fontSize: 15,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  textInactive: {
    color: colors.textSecondary,
  },
  activePill: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: colors.accentGreenSubtle,
    paddingHorizontal: 6,
    paddingVertical: 1.5,
    borderRadius: 6,
    gap: 4,
  },
  greenDot: {
    width: 6,
    height: 6,
    borderRadius: 3,
    backgroundColor: colors.accentPrimary,
  },
  activePillText: {
    fontSize: 10,
    fontWeight: '700',
    color: colors.accentPrimary,
  },
  metaRow: {
    flexDirection: 'row',
    alignItems: 'center',
    flexWrap: 'wrap',
  },
  metaText: {
    fontSize: 12,
    color: colors.textSecondary,
  },
  metaSeparator: {
    fontSize: 12,
    color: colors.textMuted,
    marginHorizontal: 5,
  },
  latencyText: {
    fontSize: 12,
    fontWeight: '600',
    color: colors.accentLime,
  },
  toggleCol: {
    paddingLeft: 4,
  },
});
