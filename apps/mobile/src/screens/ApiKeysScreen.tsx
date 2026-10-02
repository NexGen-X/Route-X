import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TouchableOpacity,
  Alert,
} from 'react-native';
import { KeyRound, MoreVertical, Copy, Shield, Plus } from 'lucide-react-native';
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

  useEffect(() => {
    loadKeys();
  }, []);

  const loadKeys = async () => {
    const list = await api.getApiKeys();
    setKeys(list);
  };

  const handleCopy = (id: string, name: string) => {
    setCopiedId(id);
    Alert.alert('Tersalin', `API Key untuk "${name}" telah disalin ke clipboard.`);
    setTimeout(() => setCopiedId(null), 2500);
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
          text: 'Hapus Kunci',
          style: 'destructive',
          onPress: () => {
            setKeys((prev) => prev.filter((k) => k.id !== item.id));
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
        onAdd={() =>
          Alert.alert('Buat API Key Baru', 'Masukkan nama kunci dan izin scope akses API.')
        }
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
          <Text style={styles.headerTitle}>Kunci Aktif ({keys.length})</Text>
          <TouchableOpacity
            style={styles.newKeyBtn}
            onPress={() =>
              Alert.alert('Buat API Key Baru', 'Formulir pembuatan kunci baru.')
            }
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
                <Copy
                  size={15}
                  color={copiedId === item.id ? colors.accentPrimary : colors.textSecondary}
                />
              </TouchableOpacity>
            </View>

            <View style={styles.metaFooter}>
              <Text style={styles.metaDate}>Dibuat: {item.createdAt}</Text>
              <Text style={styles.metaUsed}>Terakhir: {item.lastUsed}</Text>
            </View>
          </View>
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
});
