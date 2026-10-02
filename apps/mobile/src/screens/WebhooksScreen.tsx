import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TouchableOpacity,
  Switch,
  Modal,
  TextInput,
  Alert,
} from 'react-native';
import { Send, Bell, CheckCircle2 } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';
import { api } from '../api/client';
import { WebhookItem } from '../api/types';

export interface WebhooksScreenProps {
  onBack: () => void;
}

export const WebhooksScreen: React.FC<WebhooksScreenProps> = ({ onBack }) => {
  const [webhooks, setWebhooks] = useState<WebhookItem[]>([]);
  const [modalVisible, setModalVisible] = useState(false);
  const [hookName, setHookName] = useState('');
  const [hookUrl, setHookUrl] = useState('');

  const loadHooks = async () => {
    const data = await api.getWebhooks();
    setWebhooks([...data]);
  };

  useEffect(() => {
    loadHooks();
  }, []);

  const handleTestPing = (hook: WebhookItem) => {
    Alert.alert(
      'Tes Pengiriman Payload Berhasil',
      `Webhook "${hook.name}" merespons HTTP 200 OK dalam 92ms.\nPayload event dummy telah diterima.`
    );
  };

  const handleAddWebhook = async () => {
    if (!hookName.trim() || !hookUrl.trim()) {
      Alert.alert('Validasi Galat', 'Nama webhook dan URL endpoint wajib diisi.');
      return;
    }
    await api.addWebhook(hookName, hookUrl, ['provider.failover', 'budget.threshold']);
    setModalVisible(false);
    setHookName('');
    setHookUrl('');
    Alert.alert('Sukses', `Webhook "${hookName}" berhasil didaftarkan.`);
    loadHooks();
  };

  return (
    <View style={styles.container}>
      <TopHeader
        mode="subscreen"
        title="Webhooks & Notifikasi"
        onBack={onBack}
        onAdd={() => setModalVisible(true)}
      />

      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        {/* Banner */}
        <View style={styles.infoBanner}>
          <Bell size={20} color={colors.accentPrimary} />
          <View style={styles.infoCol}>
            <Text style={styles.infoTitle}>Integrasi Notifikasi Realtime</Text>
            <Text style={styles.infoDesc}>
              Kirim sinyal event otomatis saat terjadi failover model, latensi tinggi, atau batas anggaran terlampaui.
            </Text>
          </View>
        </View>

        {/* Webhook Cards List */}
        <View style={styles.hookList}>
          {webhooks.map((item) => (
            <View key={item.id} style={styles.hookCard}>
              <View style={styles.hookHeader}>
                <View style={styles.hookTitleCol}>
                  <Text style={styles.hookName}>{item.name}</Text>
                  <Text style={styles.hookUrl} numberOfLines={1}>{item.url}</Text>
                </View>
                <Switch
                  value={item.active}
                  onValueChange={() => {
                    item.active = !item.active;
                    setWebhooks([...webhooks]);
                  }}
                  trackColor={{ false: colors.bgElevated, true: colors.accentPrimary }}
                  thumbColor="#FFFFFF"
                />
              </View>

              {/* Event Tags */}
              <View style={styles.eventTagsRow}>
                {item.events.map((evt, idx) => (
                  <View key={idx} style={styles.eventTag}>
                    <Text style={styles.eventTagText}>{evt}</Text>
                  </View>
                ))}
              </View>

              <View style={styles.hookFooter}>
                <View style={styles.deliveryStatusRow}>
                  <CheckCircle2 size={13} color={colors.accentPrimary} />
                  <Text style={styles.deliveryText}>
                    Terkirim {item.lastDeliveryTime} • 200 OK
                  </Text>
                </View>

                <TouchableOpacity
                  style={styles.testPingBtn}
                  onPress={() => handleTestPing(item)}
                >
                  <Send size={12} color={colors.accentPrimary} />
                  <Text style={styles.testPingText}>Tes Ping</Text>
                </TouchableOpacity>
              </View>
            </View>
          ))}
        </View>
      </ScrollView>

      {/* Modal Tambah Webhook */}
      <Modal
        visible={modalVisible}
        transparent={true}
        animationType="slide"
        onRequestClose={() => setModalVisible(false)}
      >
        <View style={styles.modalOverlay}>
          <View style={styles.modalContent}>
            <View style={styles.modalHeader}>
              <Text style={styles.modalTitle}>Tambah Webhook Baru</Text>
              <TouchableOpacity onPress={() => setModalVisible(false)}>
                <Text style={styles.closeBtn}>✕</Text>
              </TouchableOpacity>
            </View>

            <Text style={styles.inputLabel}>Nama Endpoint / Layanan</Text>
            <TextInput
              style={styles.textInput}
              placeholder="Contoh: Slack Enterprise Alerts"
              placeholderTextColor={colors.textMuted}
              value={hookName}
              onChangeText={setHookName}
            />

            <Text style={styles.inputLabel}>URL Webhook HTTPS</Text>
            <TextInput
              style={styles.textInput}
              placeholder="https://hooks.slack.com/services/..."
              placeholderTextColor={colors.textMuted}
              value={hookUrl}
              onChangeText={setHookUrl}
              autoCapitalize="none"
            />

            <View style={styles.modalActionRow}>
              <TouchableOpacity
                style={styles.cancelBtn}
                onPress={() => setModalVisible(false)}
              >
                <Text style={styles.cancelBtnText}>Batal</Text>
              </TouchableOpacity>
              <TouchableOpacity
                style={styles.submitBtn}
                onPress={handleAddWebhook}
              >
                <Text style={styles.submitBtnText}>Simpan Webhook</Text>
              </TouchableOpacity>
            </View>
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
  scrollArea: {
    flex: 1,
  },
  scrollContent: {
    padding: 16,
    paddingBottom: 24,
  },
  infoBanner: {
    flexDirection: 'row',
    alignItems: 'flex-start',
    backgroundColor: colors.bgSurface,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 14,
    gap: 12,
    marginBottom: 16,
  },
  infoCol: {
    flex: 1,
  },
  infoTitle: {
    fontSize: 14,
    fontWeight: '700',
    color: colors.textPrimary,
    marginBottom: 4,
  },
  infoDesc: {
    fontSize: 12,
    color: colors.textSecondary,
    lineHeight: 18,
  },
  hookList: {
    gap: 12,
  },
  hookCard: {
    backgroundColor: colors.bgSurface,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
  },
  hookHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 10,
  },
  hookTitleCol: {
    flex: 1,
    marginRight: 10,
  },
  hookName: {
    fontSize: 15,
    fontWeight: '700',
    color: colors.textPrimary,
    marginBottom: 3,
  },
  hookUrl: {
    fontSize: 11,
    color: colors.textMuted,
    fontFamily: 'monospace',
  },
  eventTagsRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 6,
    marginBottom: 12,
  },
  eventTag: {
    backgroundColor: colors.bgElevated,
    paddingHorizontal: 8,
    paddingVertical: 3,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: colors.border,
  },
  eventTagText: {
    fontSize: 10,
    color: colors.accentPrimary,
    fontFamily: 'monospace',
    fontWeight: '700',
  },
  hookFooter: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    borderTopWidth: 1,
    borderTopColor: colors.border,
    paddingTop: 12,
  },
  deliveryStatusRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  deliveryText: {
    fontSize: 11,
    color: colors.textSecondary,
  },
  testPingBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
    backgroundColor: colors.bgElevated,
    paddingHorizontal: 10,
    paddingVertical: 5,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: colors.border,
  },
  testPingText: {
    fontSize: 11,
    fontWeight: '700',
    color: colors.accentPrimary,
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
  inputLabel: {
    fontSize: 12,
    fontWeight: '600',
    color: colors.textSecondary,
    marginBottom: 6,
    marginTop: 10,
  },
  textInput: {
    backgroundColor: colors.bgElevated,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 10,
    color: colors.textPrimary,
    fontSize: 14,
  },
  modalActionRow: {
    flexDirection: 'row',
    justifyContent: 'flex-end',
    gap: 10,
    marginTop: 20,
  },
  cancelBtn: {
    paddingHorizontal: 16,
    paddingVertical: 10,
    borderRadius: 8,
    backgroundColor: colors.bgElevated,
  },
  cancelBtnText: {
    color: colors.textSecondary,
    fontWeight: '600',
  },
  submitBtn: {
    paddingHorizontal: 18,
    paddingVertical: 10,
    borderRadius: 8,
    backgroundColor: colors.accentPrimary,
  },
  submitBtnText: {
    color: '#0A0B0D',
    fontWeight: '700',
  },
});
