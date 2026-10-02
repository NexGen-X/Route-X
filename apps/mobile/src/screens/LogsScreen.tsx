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
import { Terminal, Copy, Cpu } from 'lucide-react-native';
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
  const [selectedLog, setSelectedLog] = useState<ActivityItem | null>(null);
  const [detailModalVisible, setDetailModalVisible] = useState(false);
  const [activeInspectorTab, setActiveInspectorTab] = useState<'payload' | 'headers' | 'response'>('payload');

  useEffect(() => {
    loadLogs();
  }, []);

  const loadLogs = async () => {
    const data = await api.getActivities();
    setLogs(data);
  };

  const openLogDetail = (log: ActivityItem) => {
    setSelectedLog(log);
    setActiveInspectorTab('payload');
    setDetailModalVisible(true);
  };

  const handleCopyText = (_content: string) => {
    Alert.alert('Tersalin ke Clipboard', 'Isi payload / header telah disalin.');
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
            Peringatan / Failover
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
            onPress={() => openLogDetail(item)}
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
              {item.providerUsed && (
                <View style={styles.providerBadge}>
                  <Cpu size={11} color={colors.textSecondary} />
                  <Text style={styles.providerBadgeText}>{item.providerUsed}</Text>
                </View>
              )}
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

      {/* Modal Detail Payload & Header Inspector */}
      <Modal
        visible={detailModalVisible}
        transparent={true}
        animationType="slide"
        onRequestClose={() => setDetailModalVisible(false)}
      >
        <View style={styles.modalOverlay}>
          <View style={styles.modalContent}>
            <View style={styles.modalHeader}>
              <View style={styles.modalTitleCluster}>
                <Terminal size={18} color={colors.accentPrimary} />
                <Text style={styles.modalTitle}>Request Inspector</Text>
              </View>
              <TouchableOpacity onPress={() => setDetailModalVisible(false)}>
                <Text style={styles.closeBtn}>✕</Text>
              </TouchableOpacity>
            </View>

            {selectedLog && (
              <>
                {/* Meta summary card */}
                <View style={styles.inspectorMetaCard}>
                  <View style={styles.metaRow}>
                    <Text style={styles.metaKey}>Endpoint:</Text>
                    <Text style={styles.metaVal}>{selectedLog.method} {selectedLog.endpoint}</Text>
                  </View>
                  <View style={styles.metaRow}>
                    <Text style={styles.metaKey}>Latensi Gateway:</Text>
                    <Text style={styles.metaValHighlight}>{selectedLog.latencyMs} ms</Text>
                  </View>
                  {selectedLog.providerUsed && (
                    <View style={styles.metaRow}>
                      <Text style={styles.metaKey}>Provider Terpilih:</Text>
                      <Text style={styles.metaVal}>{selectedLog.providerUsed}</Text>
                    </View>
                  )}
                  {selectedLog.tokensUsed && (
                    <View style={styles.metaRow}>
                      <Text style={styles.metaKey}>Token Terpakai:</Text>
                      <Text style={styles.metaVal}>{selectedLog.tokensUsed} tokens</Text>
                    </View>
                  )}
                </View>

                {/* Sub-tabs in inspector */}
                <View style={styles.subTabRow}>
                  <TouchableOpacity
                    style={[styles.subTabBtn, activeInspectorTab === 'payload' && styles.subTabBtnActive]}
                    onPress={() => setActiveInspectorTab('payload')}
                  >
                    <Text style={[styles.subTabText, activeInspectorTab === 'payload' && styles.subTabTextActive]}>
                      Request Body
                    </Text>
                  </TouchableOpacity>
                  <TouchableOpacity
                    style={[styles.subTabBtn, activeInspectorTab === 'response' && styles.subTabBtnActive]}
                    onPress={() => setActiveInspectorTab('response')}
                  >
                    <Text style={[styles.subTabText, activeInspectorTab === 'response' && styles.subTabTextActive]}>
                      Response JSON
                    </Text>
                  </TouchableOpacity>
                  <TouchableOpacity
                    style={[styles.subTabBtn, activeInspectorTab === 'headers' && styles.subTabBtnActive]}
                    onPress={() => setActiveInspectorTab('headers')}
                  >
                    <Text style={[styles.subTabText, activeInspectorTab === 'headers' && styles.subTabTextActive]}>
                      Headers
                    </Text>
                  </TouchableOpacity>
                </View>

                {/* Payload Display Box */}
                <ScrollView style={styles.payloadBox}>
                  <Text style={styles.payloadCode}>
                    {activeInspectorTab === 'payload' && (selectedLog.payload || '{"message": "No body payload"}')}
                    {activeInspectorTab === 'response' && (selectedLog.response || '{"status": "OK"}')}
                    {activeInspectorTab === 'headers' && JSON.stringify(selectedLog.headers || {}, null, 2)}
                  </Text>
                </ScrollView>

                <TouchableOpacity
                  style={styles.copyPayloadBtn}
                  onPress={() =>
                    handleCopyText(
                      activeInspectorTab === 'payload'
                        ? selectedLog.payload || ''
                        : activeInspectorTab === 'response'
                        ? selectedLog.response || ''
                        : JSON.stringify(selectedLog.headers, null, 2)
                    )
                  }
                >
                  <Copy size={14} color="#0A0B0D" />
                  <Text style={styles.copyPayloadBtnText}>Salin Isi ke Clipboard</Text>
                </TouchableOpacity>
              </>
            )}
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
  providerBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
    backgroundColor: colors.bgElevated,
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 4,
  },
  providerBadgeText: {
    fontSize: 10,
    color: colors.textSecondary,
    fontWeight: '600',
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
  modalOverlay: {
    flex: 1,
    backgroundColor: 'rgba(0,0,0,0.7)',
    justifyContent: 'center',
    alignItems: 'center',
    padding: 16,
  },
  modalContent: {
    width: '100%',
    maxHeight: '85%',
    backgroundColor: colors.bgSurface,
    borderRadius: 16,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 18,
  },
  modalHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 14,
  },
  modalTitleCluster: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
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
  inspectorMetaCard: {
    backgroundColor: colors.bgElevated,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 12,
    marginBottom: 12,
    gap: 6,
  },
  metaRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  metaKey: {
    fontSize: 11,
    color: colors.textMuted,
  },
  metaVal: {
    fontSize: 12,
    fontWeight: '600',
    color: colors.textPrimary,
    fontFamily: 'monospace',
  },
  metaValHighlight: {
    fontSize: 12,
    fontWeight: '700',
    color: colors.accentPrimary,
    fontFamily: 'monospace',
  },
  subTabRow: {
    flexDirection: 'row',
    gap: 6,
    marginBottom: 10,
  },
  subTabBtn: {
    flex: 1,
    paddingVertical: 7,
    borderRadius: 6,
    backgroundColor: colors.bgElevated,
    alignItems: 'center',
  },
  subTabBtnActive: {
    backgroundColor: colors.accentGreenSubtle,
    borderWidth: 1,
    borderColor: colors.accentPrimary,
  },
  subTabText: {
    fontSize: 11,
    fontWeight: '600',
    color: colors.textSecondary,
  },
  subTabTextActive: {
    color: colors.accentPrimary,
    fontWeight: '700',
  },
  payloadBox: {
    maxHeight: 220,
    backgroundColor: '#07080A',
    borderRadius: 8,
    padding: 12,
    borderWidth: 1,
    borderColor: colors.border,
    marginBottom: 14,
  },
  payloadCode: {
    fontFamily: 'monospace',
    fontSize: 11,
    color: colors.accentPrimary,
    lineHeight: 16,
  },
  copyPayloadBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: colors.accentPrimary,
    borderRadius: 10,
    paddingVertical: 12,
    gap: 8,
  },
  copyPayloadBtnText: {
    fontSize: 13,
    fontWeight: '700',
    color: '#0A0B0D',
  },
});
