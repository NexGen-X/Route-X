import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TouchableOpacity,
  Alert,
} from 'react-native';
import { Terminal } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';
import { api } from '../api/client';
import { ActivityItem } from '../api/types';

export interface LogsScreenProps {
  onBack: () => void;
}

export const LogsScreen: React.FC<LogsScreenProps> = ({ onBack }) => {
  const [logs, setLogs] = useState<ActivityItem[]>([]);
  const [filterStatus, setFilterStatus] = useState<'all' | 'success' | 'warning' | 'error'>('all');

  useEffect(() => {
    loadLogs();
  }, []);

  const loadLogs = async () => {
    const data = await api.getActivities();
    setLogs(data);
  };

  const filteredLogs = logs.filter((log) => {
    if (filterStatus === 'all') return true;
    return log.status === filterStatus;
  });

  return (
    <View style={styles.container}>
      <TopHeader
        mode="subscreen"
        title="Live Request Logs"
        onBack={onBack}
        onSearch={() =>
          Alert.alert('Filter Log', 'Cari log berdasarkan request ID, model, atau endpoint path.')
        }
      />

      {/* Filter Row */}
      <View style={styles.filterRow}>
        <TouchableOpacity
          style={[styles.filterBtn, filterStatus === 'all' && styles.filterBtnActive]}
          onPress={() => setFilterStatus('all')}
        >
          <Text style={[styles.filterBtnText, filterStatus === 'all' && styles.filterBtnTextActive]}>
            Semua ({logs.length})
          </Text>
        </TouchableOpacity>
        <TouchableOpacity
          style={[styles.filterBtn, filterStatus === 'success' && styles.filterBtnActive]}
          onPress={() => setFilterStatus('success')}
        >
          <Text style={[styles.filterBtnText, filterStatus === 'success' && styles.filterBtnTextActive]}>
            Sukses
          </Text>
        </TouchableOpacity>
        <TouchableOpacity
          style={[styles.filterBtn, filterStatus === 'warning' && styles.filterBtnActive]}
          onPress={() => setFilterStatus('warning')}
        >
          <Text style={[styles.filterBtnText, filterStatus === 'warning' && styles.filterBtnTextActive]}>
            Peringatan
          </Text>
        </TouchableOpacity>
      </View>

      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        {filteredLogs.map((item) => (
          <TouchableOpacity
            key={item.id}
            style={styles.logCard}
            onPress={() =>
              Alert.alert(
                item.title,
                `Method: ${item.method}\nEndpoint: ${item.endpoint}\nStatus: ${item.status}\nLatensi: ${item.latencyMs}ms\nWaktu: ${item.timestamp}`
              )
            }
            activeOpacity={0.7}
          >
            <View style={styles.topRow}>
              <View style={styles.methodBadge}>
                <Text style={styles.methodText}>{item.method}</Text>
              </View>
              <Text style={styles.endpointText} numberOfLines={1}>
                {item.endpoint}
              </Text>
              <Text style={styles.timestampText}>{item.timestamp}</Text>
            </View>

            <View style={styles.bottomRow}>
              <View style={styles.latencyBadge}>
                <Terminal size={12} color={colors.accentPrimary} />
                <Text style={styles.latencyText}>{item.latencyMs} ms</Text>
              </View>
              <View
                style={[
                  styles.statusBadge,
                  item.status === 'success' && styles.statusBadgeSuccess,
                  item.status === 'warning' && styles.statusBadgeWarning,
                  item.status === 'error' && styles.statusBadgeError,
                ]}
              >
                <Text
                  style={[
                    styles.statusBadgeText,
                    item.status === 'success' && styles.statusTextSuccess,
                    item.status === 'warning' && styles.statusTextWarning,
                    item.status === 'error' && styles.statusTextError,
                  ]}
                >
                  {item.status.toUpperCase()}
                </Text>
              </View>
            </View>
          </TouchableOpacity>
        ))}
      </ScrollView>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.bgCanvas,
  },
  filterRow: {
    flexDirection: 'row',
    paddingHorizontal: 16,
    paddingVertical: 10,
    backgroundColor: colors.bgSurface,
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
    gap: 8,
  },
  filterBtn: {
    paddingHorizontal: 12,
    paddingVertical: 6,
    borderRadius: 8,
    backgroundColor: colors.bgElevated,
  },
  filterBtnActive: {
    backgroundColor: colors.accentGreenSubtle,
    borderWidth: 1,
    borderColor: colors.accentPrimary,
  },
  filterBtnText: {
    fontSize: 12,
    fontWeight: '600',
    color: colors.textSecondary,
  },
  filterBtnTextActive: {
    color: colors.accentPrimary,
  },
  scrollArea: {
    flex: 1,
  },
  scrollContent: {
    padding: 16,
    paddingBottom: 24,
  },
  logCard: {
    backgroundColor: colors.bgSurface,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 12,
    marginBottom: 10,
  },
  topRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    marginBottom: 8,
  },
  methodBadge: {
    backgroundColor: colors.bgElevated,
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 4,
    borderWidth: 1,
    borderColor: colors.border,
  },
  methodText: {
    fontSize: 11,
    fontWeight: '800',
    color: colors.accentPrimary,
    fontFamily: 'monospace',
  },
  endpointText: {
    flex: 1,
    fontSize: 13,
    color: colors.textPrimary,
    fontFamily: 'monospace',
  },
  timestampText: {
    fontSize: 11,
    color: colors.textMuted,
  },
  bottomRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  latencyBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
  },
  latencyText: {
    fontSize: 12,
    color: colors.textSecondary,
    fontFamily: 'monospace',
  },
  statusBadge: {
    paddingHorizontal: 8,
    paddingVertical: 2,
    borderRadius: 6,
  },
  statusBadgeSuccess: {
    backgroundColor: colors.accentGreenSubtle,
  },
  statusBadgeWarning: {
    backgroundColor: 'rgba(234, 179, 8, 0.12)',
  },
  statusBadgeError: {
    backgroundColor: 'rgba(239, 68, 68, 0.12)',
  },
  statusBadgeText: {
    fontSize: 10,
    fontWeight: '800',
  },
  statusTextSuccess: {
    color: colors.accentPrimary,
  },
  statusTextWarning: {
    color: colors.statusWarning,
  },
  statusTextError: {
    color: colors.statusError,
  },
});
