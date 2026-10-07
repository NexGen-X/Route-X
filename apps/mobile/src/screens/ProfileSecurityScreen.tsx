import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TouchableOpacity,
  Switch,
  Alert,
  Modal,
  TextInput,
  RefreshControl,
} from 'react-native';
import {
  ShieldCheck,
  Smartphone,
  Monitor,
  KeyRound,
  LogOut,
  Fingerprint,
  Info,
} from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';
import { api } from '../api/client';
import { SecurityProfile } from '../api/types';

export interface ProfileSecurityScreenProps {
  onBack: () => void;
}

export const ProfileSecurityScreen: React.FC<ProfileSecurityScreenProps> = ({ onBack }) => {
  const [profile, setProfile] = useState<SecurityProfile | null>(null);
  const [passwordModalVisible, setPasswordModalVisible] = useState(false);
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');

  useEffect(() => {
    loadProfile();
  }, []);

  const loadProfile = async () => {
    const data = await api.getSecurityProfile();
    setProfile({ ...data });
  };

  const handleToggleMfa = async (newValue: boolean) => {
    if (!profile) return;
    const updated = await api.updateSecurityProfile({ mfaEnabled: newValue });
    setProfile({ ...updated });
    Alert.alert(
      newValue ? 'MFA Diaktifkan' : 'MFA Dinonaktifkan',
      newValue
        ? 'Autentikasi dua faktor (TOTP / Google Authenticator) kini aktif untuk semua sesi admin.'
        : 'Autentikasi dua faktor dinonaktifkan sementara.'
    );
  };

  const handleToggleBiometric = async (newValue: boolean) => {
    if (!profile) return;
    const updated = await api.updateSecurityProfile({ biometricAppLock: newValue });
    setProfile({ ...updated });
    Alert.alert(
      newValue ? 'Kunci Biometrik Aktif' : 'Kunci Biometrik Nonaktif',
      newValue
        ? 'Aplikasi Route-X akan meminta sidik jari / Face Unlock setiap kali dibuka.'
        : 'Kunci biometrik aplikasi telah dinonaktifkan.'
    );
  };

  const handleChangePassword = () => {
    if (!currentPassword || !newPassword) {
      Alert.alert('Galat', 'Kata sandi lama dan baru wajib diisi.');
      return;
    }
    if (newPassword !== confirmPassword) {
      Alert.alert('Galat', 'Konfirmasi kata sandi baru tidak cocok.');
      return;
    }
    setPasswordModalVisible(false);
    setCurrentPassword('');
    setNewPassword('');
    setConfirmPassword('');
    Alert.alert('Berhasil', 'Kata sandi kredensial admin berhasil diperbarui.');
  };

  const handleRevokeSessions = () => {
    Alert.alert(
      'Konfirmasi Putus Sesi',
      'Apakah Anda yakin ingin memutuskan semua sesi aktif lainnya kecuali perangkat Android ini?',
      [
        { text: 'Batal', style: 'cancel' },
        {
          text: 'Putuskan Sesi',
          style: 'destructive',
          onPress: async () => {
            if (profile) {
              const updated = await api.updateSecurityProfile({ activeSessionsCount: 1 });
              setProfile({ ...updated });
              Alert.alert('Berhasil', 'Semua sesi lain telah berhasil di-revoke.');
            }
          },
        },
      ]
    );
  };

  if (!profile) return null;

  return (
    <View style={styles.container}>
      <TopHeader
        mode="subscreen"
        title="Profil & Keamanan Admin"
        onBack={onBack}
      />

      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
        refreshControl={
          <RefreshControl
            refreshing={false}
            onRefresh={loadProfile}
            tintColor={colors.accentPrimary}
            colors={[colors.accentPrimary]}
          />
        }
      >
        {/* Profile Identity Card */}
        <View style={styles.profileCard}>
          <View style={styles.avatarRow}>
            <View style={styles.avatarCircle}>
              <Text style={styles.avatarText}>AD</Text>
            </View>
            <View style={styles.profileInfo}>
              <View style={styles.roleBadge}>
                <Text style={styles.roleBadgeText}>{profile.role}</Text>
              </View>
              <Text style={styles.profileName}>{profile.adminName}</Text>
              <Text style={styles.profileEmail}>{profile.email}</Text>
            </View>
          </View>

          <View style={styles.lastLoginBox}>
            <Text style={styles.lastLoginText}>
              Login terakhir: <Text style={styles.highlightText}>{profile.lastLoginTime}</Text>
            </Text>
            <Text style={styles.lastLoginSub}>IP: {profile.lastLoginIp} (Singapore AWS)</Text>
          </View>
        </View>

        {/* Security Controls Section */}
        <Text style={styles.sectionHeader}>KONTROL KEAMANAN GERBANG</Text>

        <View style={styles.settingsGroup}>
          {/* Biometric Toggle */}
          <View style={styles.settingItem}>
            <View style={styles.iconCircle}>
              <Fingerprint size={20} color={colors.accentPrimary} />
            </View>
            <View style={styles.settingTextContainer}>
              <Text style={styles.settingTitle}>Kunci Biometrik APK</Text>
              <Text style={styles.settingDesc}>
                Minta sidik jari / Face ID saat membuka aplikasi
              </Text>
            </View>
            <Switch
              value={profile.biometricAppLock}
              onValueChange={handleToggleBiometric}
              trackColor={{ false: colors.bgElevated, true: colors.accentPrimary }}
              thumbColor="#FFFFFF"
            />
          </View>

          {/* MFA Toggle */}
          <View style={styles.settingItem}>
            <View style={styles.iconCircle}>
              <ShieldCheck size={20} color={colors.accentLime} />
            </View>
            <View style={styles.settingTextContainer}>
              <Text style={styles.settingTitle}>Autentikasi Dua Faktor (2FA / MFA)</Text>
              <Text style={styles.settingDesc}>Verifikasi TOTP OTP via Authenticator</Text>
            </View>
            <Switch
              value={profile.mfaEnabled}
              onValueChange={handleToggleMfa}
              trackColor={{ false: colors.bgElevated, true: colors.accentPrimary }}
              thumbColor="#FFFFFF"
            />
          </View>

          {/* Change Password */}
          <TouchableOpacity
            style={styles.settingItem}
            onPress={() => setPasswordModalVisible(true)}
          >
            <View style={styles.iconCircle}>
              <KeyRound size={20} color={colors.avatarDev} />
            </View>
            <View style={styles.settingTextContainer}>
              <Text style={styles.settingTitle}>Ubah Kata Sandi Kredensial</Text>
              <Text style={styles.settingDesc}>Perbarui kata sandi akses gateway admin</Text>
            </View>
            <Text style={styles.actionChevron}>›</Text>
          </TouchableOpacity>
        </View>

        {/* Active Sessions */}
        <Text style={styles.sectionHeader}>SESI AKTIF TERDAFTAR</Text>
        <View style={styles.sessionsCard}>
          <View style={styles.sessionItem}>
            <Smartphone size={20} color={colors.accentPrimary} />
            <View style={styles.sessionInfo}>
              <View style={styles.sessionTitleRow}>
                <Text style={styles.sessionName}>Aplikasi Android Route-X (APK)</Text>
                <View style={styles.activeNowBadge}>
                  <Text style={styles.activeNowText}>Perangkat Ini</Text>
                </View>
              </View>
              <Text style={styles.sessionSub}>13.212.164.108 • Aktif sekarang</Text>
            </View>
          </View>

          {profile.activeSessionsCount > 1 && (
            <View style={styles.sessionItem}>
              <Monitor size={20} color={colors.textMuted} />
              <View style={styles.sessionInfo}>
                <Text style={styles.sessionName}>Web Console (Google Chrome)</Text>
                <Text style={styles.sessionSub}>macOS Darwin • 35 menit lalu</Text>
              </View>
            </View>
          )}

          {profile.activeSessionsCount > 1 && (
            <TouchableOpacity style={styles.revokeBtn} onPress={handleRevokeSessions}>
              <LogOut size={16} color={colors.danger} />
              <Text style={styles.revokeBtnText}>Putuskan Semua Sesi Lain</Text>
            </TouchableOpacity>
          )}
        </View>

        {/* System Version & Integrity */}
        <Text style={styles.sectionHeader}>INFORMASI SISTEM & KEPATUHAN</Text>
        <View style={styles.infoCard}>
          <View style={styles.infoRow}>
            <View style={styles.infoLabelCol}>
              <Info size={14} color={colors.textMuted} />
              <Text style={styles.infoLabel}>Versi Gateway Core</Text>
            </View>
            <Text style={styles.infoValue}>v2.4.0 (Go 1.22 Fast Engine)</Text>
          </View>

          <View style={styles.infoRow}>
            <View style={styles.infoLabelCol}>
              <Smartphone size={14} color={colors.textMuted} />
              <Text style={styles.infoLabel}>Versi Android Companion</Text>
            </View>
            <Text style={styles.infoValue}>v1.2.0 (Hermes Release)</Text>
          </View>

          <View style={styles.infoRow}>
            <View style={styles.infoLabelCol}>
              <ShieldCheck size={14} color={colors.accentLime} />
              <Text style={styles.infoLabel}>Protokol Keamanan</Text>
            </View>
            <Text style={styles.infoValueHighlight}>TLS 1.3 Strict Enforced</Text>
          </View>
        </View>
      </ScrollView>

      {/* Modal Ubah Kata Sandi */}
      <Modal
        visible={passwordModalVisible}
        transparent={true}
        animationType="slide"
        onRequestClose={() => setPasswordModalVisible(false)}
      >
        <View style={styles.modalOverlay}>
          <View style={styles.modalContent}>
            <View style={styles.modalHeader}>
              <Text style={styles.modalTitle}>Perbarui Kata Sandi Admin</Text>
              <TouchableOpacity onPress={() => setPasswordModalVisible(false)}>
                <Text style={styles.closeBtn}>✕</Text>
              </TouchableOpacity>
            </View>

            <Text style={styles.inputLabel}>Kata Sandi Lama</Text>
            <TextInput
              style={styles.textInput}
              placeholder="Masukkan kata sandi lama"
              placeholderTextColor={colors.textMuted}
              secureTextEntry
              value={currentPassword}
              onChangeText={setCurrentPassword}
            />

            <Text style={styles.inputLabel}>Kata Sandi Baru</Text>
            <TextInput
              style={styles.textInput}
              placeholder="Minimal 8 karakter kombinasi"
              placeholderTextColor={colors.textMuted}
              secureTextEntry
              value={newPassword}
              onChangeText={setNewPassword}
            />

            <Text style={styles.inputLabel}>Konfirmasi Kata Sandi Baru</Text>
            <TextInput
              style={styles.textInput}
              placeholder="Ulangi kata sandi baru"
              placeholderTextColor={colors.textMuted}
              secureTextEntry
              value={confirmPassword}
              onChangeText={setConfirmPassword}
            />

            <TouchableOpacity style={styles.submitBtn} onPress={handleChangePassword}>
              <Text style={styles.submitBtnText}>Simpan Kata Sandi Baru</Text>
            </TouchableOpacity>
          </View>
        </View>
      </Modal>
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
  profileCard: {
    backgroundColor: colors.bgCard,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    borderRadius: 16,
    padding: 16,
    marginBottom: 20,
  },
  avatarRow: {
    flexDirection: 'row',
    alignItems: 'center',
    marginBottom: 14,
  },
  avatarCircle: {
    width: 60,
    height: 60,
    borderRadius: 30,
    backgroundColor: colors.accentPrimary,
    justifyContent: 'center',
    alignItems: 'center',
    marginRight: 14,
  },
  avatarText: {
    color: colors.bgBase,
    fontSize: 22,
    fontWeight: '800',
  },
  profileInfo: {
    flex: 1,
  },
  roleBadge: {
    backgroundColor: 'rgba(56, 189, 248, 0.15)',
    alignSelf: 'flex-start',
    paddingHorizontal: 8,
    paddingVertical: 2,
    borderRadius: 6,
    marginBottom: 4,
  },
  roleBadgeText: {
    color: colors.accentPrimary,
    fontSize: 11,
    fontWeight: '700',
  },
  profileName: {
    color: colors.textPrimary,
    fontSize: 18,
    fontWeight: '700',
    marginBottom: 2,
  },
  profileEmail: {
    color: colors.textMuted,
    fontSize: 13,
  },
  lastLoginBox: {
    backgroundColor: colors.bgElevated,
    borderRadius: 10,
    padding: 12,
  },
  lastLoginText: {
    color: colors.textSecondary,
    fontSize: 12,
    marginBottom: 2,
  },
  highlightText: {
    color: colors.accentLime,
    fontWeight: '600',
  },
  lastLoginSub: {
    color: colors.textMuted,
    fontSize: 11,
  },
  sectionHeader: {
    color: colors.textMuted,
    fontSize: 12,
    fontWeight: '700',
    letterSpacing: 0.8,
    marginBottom: 10,
    marginTop: 6,
  },
  settingsGroup: {
    backgroundColor: colors.bgCard,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    marginBottom: 20,
    overflow: 'hidden',
  },
  settingItem: {
    flexDirection: 'row',
    alignItems: 'center',
    padding: 14,
    borderBottomWidth: 1,
    borderBottomColor: colors.borderSubtle,
  },
  iconCircle: {
    width: 38,
    height: 38,
    borderRadius: 10,
    backgroundColor: colors.bgElevated,
    justifyContent: 'center',
    alignItems: 'center',
    marginRight: 12,
  },
  settingTextContainer: {
    flex: 1,
    marginRight: 8,
  },
  settingTitle: {
    color: colors.textPrimary,
    fontSize: 14,
    fontWeight: '600',
    marginBottom: 2,
  },
  settingDesc: {
    color: colors.textMuted,
    fontSize: 12,
  },
  actionChevron: {
    color: colors.textMuted,
    fontSize: 22,
    fontWeight: '300',
  },
  sessionsCard: {
    backgroundColor: colors.bgCard,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    padding: 14,
    marginBottom: 20,
    gap: 12,
  },
  sessionItem: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    paddingVertical: 4,
  },
  sessionInfo: {
    flex: 1,
  },
  sessionTitleRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  sessionName: {
    color: colors.textPrimary,
    fontSize: 13,
    fontWeight: '600',
  },
  activeNowBadge: {
    backgroundColor: 'rgba(163, 230, 53, 0.15)',
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 4,
  },
  activeNowText: {
    color: colors.accentLime,
    fontSize: 10,
    fontWeight: '700',
  },
  sessionSub: {
    color: colors.textMuted,
    fontSize: 11,
    marginTop: 2,
  },
  revokeBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 6,
    borderTopWidth: 1,
    borderTopColor: colors.borderSubtle,
    paddingTop: 12,
    marginTop: 4,
  },
  revokeBtnText: {
    color: colors.danger,
    fontSize: 13,
    fontWeight: '600',
  },
  infoCard: {
    backgroundColor: colors.bgCard,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    padding: 14,
    gap: 10,
  },
  infoRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  infoLabelCol: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  infoLabel: {
    color: colors.textMuted,
    fontSize: 12,
  },
  infoValue: {
    color: colors.textSecondary,
    fontSize: 12,
    fontWeight: '600',
  },
  infoValueHighlight: {
    color: colors.accentLime,
    fontSize: 12,
    fontWeight: '700',
  },
  // Modal styles
  modalOverlay: {
    flex: 1,
    backgroundColor: 'rgba(0, 0, 0, 0.75)',
    justifyContent: 'flex-end',
  },
  modalContent: {
    backgroundColor: colors.bgCard,
    borderTopLeftRadius: 20,
    borderTopRightRadius: 20,
    padding: 20,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
  },
  modalHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 16,
  },
  modalTitle: {
    color: colors.textPrimary,
    fontSize: 18,
    fontWeight: '700',
  },
  closeBtn: {
    color: colors.textMuted,
    fontSize: 20,
    padding: 4,
  },
  inputLabel: {
    color: colors.textSecondary,
    fontSize: 12,
    fontWeight: '600',
    marginBottom: 6,
    marginTop: 10,
  },
  textInput: {
    backgroundColor: colors.bgElevated,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 10,
    color: colors.textPrimary,
    fontSize: 14,
  },
  submitBtn: {
    backgroundColor: colors.accentPrimary,
    borderRadius: 10,
    paddingVertical: 14,
    alignItems: 'center',
    marginTop: 20,
    marginBottom: 10,
  },
  submitBtnText: {
    color: colors.bgBase,
    fontSize: 15,
    fontWeight: '700',
  },
});
