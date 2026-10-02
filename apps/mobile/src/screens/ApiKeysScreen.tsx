import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TouchableOpacity,
  Modal,
  TextInput,
  Alert,
} from 'react-native';
import { KeyRound, MoreVertical, Copy, Shield, Plus, Check } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';
import { api } from '../api/client';
import { ApiKeyItem } from '../api/types';

export interface ApiKeysScreenProps {
  onBack: () => void;
}

export const ApiKeysScreen: React.FC<ApiKeysScreenProps> = ({ onBack }) => {
  const [keys, setKeys] = useState<ApiKeyItem[]>([]);
  const [copiedId, setCopiedId] = useState<string | null>(null);

  // Modal Buat Kunci
  const [modalVisible, setModalVisible] = useState(false);
  const [keyName, setKeyName] = useState('');
  const [keyRole, setKeyRole] = useState('Full Admin');
  const [rateLimit, setRateLimit] = useState('1000');

  useEffect(() => {
    loadKeys();
  }, []);

  const loadKeys = async () => {
    const list = await api.getApiKeys();
    setKeys([...list]);
  };

  const handleCopy = (id: string, name: string) => {
    setCopiedId(id);
    Alert.alert('Tersalin ke Clipboard', `API Key untuk "${name}" telah disalin.`);
    setTimeout(() => setCopiedId(null), 2500);
  };

  const handleCreateKey = async () => {
    if (!keyName.trim()) {
      Alert.alert('Validasi Galat', 'Nama kunci API tidak boleh kosong.');
      return;
    }
    await api.createApiKey(keyName, keyRole, parseInt(rateLimit, 10) || 1000);
    setModalVisible(false);
    setKeyName('');
    Alert.alert('Sukses', `API Key "${keyName}" berhasil dibuat dan siap digunakan.`);
    loadKeys();
  };

  const handleMenuAction = (item: ApiKeyItem) => {
    Alert.alert(
      item.name,
      'Pilih tindakan untuk kunci ini:',
      [
        {
          text: item.active ? 'Nonaktifkan Kunci' : 'Aktifkan Kunci',
          onPress: () => {
            setKeys((prev) =>
              prev.map((k) => (k.id === item.id ? { ...k, active: !k.active } : k))
            );
          },
        },
        {
          text: 'Hapus / Revoke Kunci',
          style: 'destructive',
          onPress: async () => {
            await api.revokeApiKey(item.id);
            loadKeys();
            Alert.alert('Kunci Dihapus', `API Key "${item.name}" telah dinonaktifkan permanen.`);
          },
        },
        { text: 'Batal', style: 'cancel' },
      ]
    );
  };

  return (
    <View style={styles.container}>
      <TopHeader
        mode="subscreen"
        title="API Keys"
        onBack={onBack}
        onAdd={() => setModalVisible(true)}
      />

      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        {/* Banner Panduan */}
        <View style={styles.guideBanner}>
          <View style={styles.guideIconCircle}>
            <KeyRound size={20} color={colors.accentPrimary} />
          </View>
          <View style={styles.guideContent}>
            <Text style={styles.guideTitle}>Kelola Kunci API Gateway</Text>
            <Text style={styles.guideDesc}>
              Kunci API digunakan untuk autentikasi request klien upstream, SDK, dan integrasi CLI.
            </Text>
          </View>
        </View>

        {/* Header List */}
        <View style={styles.headerRow}>
          <Text style={styles.headerTitle}>Kunci Terdaftar ({keys.length})</Text>
          <TouchableOpacity
            style={styles.newKeyBtn}
            onPress={() => setModalVisible(true)}
          >
            <Plus size={14} color={colors.accentPrimary} />
            <Text style={styles.newKeyBtnText}>Tambah Kunci</Text>
          </TouchableOpacity>
        </View>

        {/* Daftar Kunci API */}
        {keys.map((item) => (
          <View key={item.id} style={styles.keyCard}>
            <View style={styles.keyCardTop}>
              <View style={styles.keyTitleCol}>
                <Text style={styles.keyName}>{item.name}</Text>
                <View style={styles.keyStatusRow}>
                  <View
                    style={[
                      styles.statusDot,
                      { backgroundColor: item.active ? colors.accentPrimary : colors.textMuted },
                    ]}
                  />
                  <Text
                    style={[
                      styles.statusText,
                      { color: item.active ? colors.accentPrimary : colors.textMuted },
                    ]}
                  >
                    {item.active ? 'Aktif' : 'Nonaktif'}
                  </Text>
                  {item.role && (
                    <Text style={styles.roleBadge}>• {item.role}</Text>
                  )}
                </View>
              </View>

              <TouchableOpacity
                style={styles.moreBtn}
                onPress={() => handleMenuAction(item)}
                accessibilityLabel="Opsi Kunci"
                accessibilityRole="button"
              >
                <MoreVertical size={18} color={colors.textSecondary} />
              </TouchableOpacity>
            </View>

            {/* Masked Key Box */}
            <View style={styles.keyBox}>
              <Shield size={14} color={colors.textMuted} />
              <Text style={styles.keyMaskedText}>{item.keyMasked}</Text>
              <TouchableOpacity
                style={styles.copyBtn}
                onPress={() => handleCopy(item.id, item.name)}
                accessibilityLabel="Salin Kunci"
              >
                {copiedId === item.id ? (
                  <Check size={15} color={colors.accentPrimary} />
                ) : (
                  <Copy size={15} color={colors.textSecondary} />
                )}
              </TouchableOpacity>
            </View>

            <View style={styles.metaFooter}>
              <Text style={styles.metaDate}>Dibuat: {item.createdAt}</Text>
              <Text style={styles.metaUsed}>Terakhir: {item.lastUsed}</Text>
            </View>
          </View>
        ))}
      </ScrollView>

      {/* Modal Buat API Key Baru */}
      <Modal
        visible={modalVisible}
        transparent={true}
        animationType="slide"
        onRequestClose={() => setModalVisible(false)}
      >
        <View style={styles.modalOverlay}>
          <View style={styles.modalContent}>
            <View style={styles.modalHeader}>
              <Text style={styles.modalTitle}>Buat Kunci API Baru</Text>
              <TouchableOpacity onPress={() => setModalVisible(false)}>
                <Text style={styles.closeBtn}>✕</Text>
              </TouchableOpacity>
            </View>

            <Text style={styles.inputLabel}>Nama Identitas Kunci</Text>
            <TextInput
              style={styles.textInput}
              placeholder="Contoh: Mobile Production Client"
              placeholderTextColor={colors.textMuted}
              value={keyName}
              onChangeText={setKeyName}
            />

            <Text style={styles.inputLabel}>Peran Hak Akses (Scope)</Text>
            <View style={styles.roleChipsRow}>
              {['Full Admin', 'Mobile Client', 'Read Only'].map((r) => (
                <TouchableOpacity
                  key={r}
                  style={[styles.roleChip, keyRole === r && styles.roleChipActive]}
                  onPress={() => setKeyRole(r)}
                >
                  <Text
                    style={[styles.roleChipText, keyRole === r && styles.roleChipTextActive]}
                  >
                    {r}
                  </Text>
                </TouchableOpacity>
              ))}
            </View>

            <Text style={styles.inputLabel}>Batas Rate Limit (Req / Menit)</Text>
            <TextInput
              style={styles.textInput}
              keyboardType="numeric"
              placeholder="1000"
              placeholderTextColor={colors.textMuted}
              value={rateLimit}
              onChangeText={setRateLimit}
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
                onPress={handleCreateKey}
              >
                <Text style={styles.submitBtnText}>Terbitkan Kunci</Text>
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
  guideBanner: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: colors.bgSurface,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 14,
    gap: 12,
    marginBottom: 18,
  },
  guideIconCircle: {
    width: 40,
    height: 40,
    borderRadius: 10,
    backgroundColor: colors.accentGreenSubtle,
    borderWidth: 1,
    borderColor: colors.accentGreenBorder,
    justifyContent: 'center',
    alignItems: 'center',
  },
  guideContent: {
    flex: 1,
  },
  guideTitle: {
    fontSize: 14,
    fontWeight: '700',
    color: colors.textPrimary,
    marginBottom: 3,
  },
  guideDesc: {
    fontSize: 12,
    color: colors.textSecondary,
    lineHeight: 16,
  },
  headerRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 12,
  },
  headerTitle: {
    fontSize: 14,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  newKeyBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
    backgroundColor: colors.accentGreenSubtle,
    paddingHorizontal: 10,
    paddingVertical: 5,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: colors.accentGreenBorder,
  },
  newKeyBtnText: {
    fontSize: 12,
    fontWeight: '700',
    color: colors.accentPrimary,
  },
  keyCard: {
    backgroundColor: colors.bgSurface,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 14,
    marginBottom: 12,
  },
  keyCardTop: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'flex-start',
    marginBottom: 10,
  },
  keyTitleCol: {
    flex: 1,
  },
  keyName: {
    fontSize: 15,
    fontWeight: '700',
    color: colors.textPrimary,
    marginBottom: 4,
  },
  keyStatusRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
  },
  statusDot: {
    width: 6,
    height: 6,
    borderRadius: 3,
  },
  statusText: {
    fontSize: 11,
    fontWeight: '700',
  },
  roleBadge: {
    fontSize: 11,
    color: colors.textMuted,
    fontWeight: '600',
  },
  moreBtn: {
    padding: 4,
  },
  keyBox: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: colors.bgElevated,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 12,
    paddingVertical: 10,
    gap: 8,
    marginBottom: 10,
  },
  keyMaskedText: {
    flex: 1,
    fontFamily: 'monospace',
    fontSize: 13,
    color: colors.textPrimary,
    letterSpacing: 1,
  },
  copyBtn: {
    padding: 4,
  },
  metaFooter: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  metaDate: {
    fontSize: 11,
    color: colors.textMuted,
  },
  metaUsed: {
    fontSize: 11,
    color: colors.textSecondary,
    fontWeight: '500',
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
  roleChipsRow: {
    flexDirection: 'row',
    gap: 8,
    marginVertical: 4,
  },
  roleChip: {
    paddingHorizontal: 10,
    paddingVertical: 6,
    borderRadius: 8,
    backgroundColor: colors.bgElevated,
    borderWidth: 1,
    borderColor: colors.border,
  },
  roleChipActive: {
    backgroundColor: colors.accentGreenSubtle,
    borderColor: colors.accentPrimary,
  },
  roleChipText: {
    fontSize: 11,
    fontWeight: '600',
    color: colors.textSecondary,
  },
  roleChipTextActive: {
    color: colors.accentPrimary,
    fontWeight: '700',
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
