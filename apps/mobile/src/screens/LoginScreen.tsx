import React, { useState } from 'react';
import {
  View,
  Text,
  StyleSheet,
  TextInput,
  TouchableOpacity,
  ActivityIndicator,
  Alert,
  ScrollView,
  KeyboardAvoidingView,
  Platform,
} from 'react-native';
import { Shield, Server, Mail, Lock, ArrowRight, Zap, CheckCircle2 } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { api } from '../api/client';

export interface LoginScreenProps {
  onLoginSuccess: () => void;
}

export const LoginScreen: React.FC<LoginScreenProps> = ({ onLoginSuccess }) => {
  const [serverUrl, setServerUrl] = useState(api.getServerUrl());
  const [email, setEmail] = useState('admin@routex.local');
  const [password, setPassword] = useState('admin');
  const [loading, setLoading] = useState(false);

  const handleLogin = async (overrideEmail?: string, overridePass?: string) => {
    const targetEmail = overrideEmail || email;
    const targetPass = overridePass || password;

    if (!targetEmail.trim() || !targetPass.trim()) {
      Alert.alert('Validasi Kredensial', 'Email dan kata sandi admin wajib diisi.');
      return;
    }

    setLoading(true);
    try {
      const res = await api.login(targetEmail.trim(), targetPass.trim(), serverUrl.trim());
      if (res.success) {
        onLoginSuccess();
      } else {
        Alert.alert('Gagal Masuk', res.error || 'Autentikasi ditolak oleh server gateway.');
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Koneksi gagal.';
      Alert.alert('Galat Koneksi', `Tidak dapat menghubungi gateway di ${serverUrl}: ${msg}`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <KeyboardAvoidingView
      style={styles.container}
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}
    >
      <ScrollView
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
        keyboardShouldPersistTaps="handled"
      >
        {/* Brand Header */}
        <View style={styles.brandContainer}>
          <View style={styles.logoBadge}>
            <Text style={styles.logoText}>RX</Text>
          </View>
          <Text style={styles.appTitle}>Route-X AI Gateway</Text>
          <Text style={styles.appSubtitle}>LIVE SYNCHRONIZED COMPANION</Text>
        </View>

        {/* Sync Info Banner */}
        <View style={styles.syncCard}>
          <View style={styles.syncHeaderRow}>
            <CheckCircle2 size={16} color={colors.accentLime} />
            <Text style={styles.syncTitle}>Sinkronisasi Database 100% Real</Text>
          </View>
          <Text style={styles.syncDesc}>
            Data yang Bapak kelola di aplikasi ini terhubung langsung ke PostgreSQL dan otomatis sinkron dengan web dashboard browser.
          </Text>
        </View>

        {/* Login Card */}
        <View style={styles.formCard}>
          {/* Server URL Input */}
          <Text style={styles.inputLabel}>ENDPOINT SERVER GATEWAY</Text>
          <View style={styles.inputRow}>
            <Server size={18} color={colors.accentPrimary} style={styles.inputIcon} />
            <TextInput
              style={styles.textInput}
              value={serverUrl}
              onChangeText={setServerUrl}
              placeholder="http://13.212.164.108"
              placeholderTextColor={colors.textMuted}
              autoCapitalize="none"
              autoCorrect={false}
            />
          </View>

          {/* Email Input */}
          <Text style={styles.inputLabel}>EMAIL ADMINISTRATOR</Text>
          <View style={styles.inputRow}>
            <Mail size={18} color={colors.textSecondary} style={styles.inputIcon} />
            <TextInput
              style={styles.textInput}
              value={email}
              onChangeText={setEmail}
              placeholder="admin@routex.local"
              placeholderTextColor={colors.textMuted}
              autoCapitalize="none"
              keyboardType="email-address"
              autoCorrect={false}
            />
          </View>

          {/* Password Input */}
          <Text style={styles.inputLabel}>KATA SANDI</Text>
          <View style={styles.inputRow}>
            <Lock size={18} color={colors.textSecondary} style={styles.inputIcon} />
            <TextInput
              style={styles.textInput}
              value={password}
              onChangeText={setPassword}
              placeholder="Kata sandi admin"
              placeholderTextColor={colors.textMuted}
              secureTextEntry
              autoCapitalize="none"
            />
          </View>

          {/* Tombol Masuk Utama */}
          <TouchableOpacity
            style={[styles.loginBtn, loading && styles.loginBtnDisabled]}
            onPress={() => handleLogin()}
            disabled={loading}
            activeOpacity={0.8}
          >
            {loading ? (
              <ActivityIndicator color={colors.bgBase} />
            ) : (
              <>
                <Text style={styles.loginBtnText}>Hubungkan & Masuk Live</Text>
                <ArrowRight size={18} color={colors.bgBase} />
              </>
            )}
          </TouchableOpacity>

          {/* Tombol 1-Klik Masuk Cepat */}
          <TouchableOpacity
            style={styles.quickLoginBtn}
            onPress={() => handleLogin('admin@routex.local', 'admin')}
            disabled={loading}
            activeOpacity={0.7}
          >
            <Zap size={16} color={colors.accentLime} />
            <Text style={styles.quickLoginText}>
              Masuk Instan (Default Admin 1-Klik)
            </Text>
          </TouchableOpacity>
        </View>

        {/* Footer Security Badges */}
        <View style={styles.footerNote}>
          <Shield size={14} color={colors.textMuted} />
          <Text style={styles.footerText}>
            Enkripsi Sesi Token • Native Go Gateway Engine v1.2.0
          </Text>
        </View>
      </ScrollView>
    </KeyboardAvoidingView>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.bgBase,
  },
  scrollContent: {
    padding: 20,
    justifyContent: 'center',
    minHeight: '100%',
  },
  brandContainer: {
    alignItems: 'center',
    marginBottom: 24,
  },
  logoBadge: {
    width: 64,
    height: 64,
    borderRadius: 18,
    backgroundColor: colors.accentPrimary,
    justifyContent: 'center',
    alignItems: 'center',
    marginBottom: 14,
    elevation: 6,
    shadowColor: colors.accentPrimary,
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.3,
    shadowRadius: 8,
  },
  logoText: {
    color: colors.bgBase,
    fontSize: 28,
    fontWeight: '900',
    letterSpacing: -1,
  },
  appTitle: {
    color: colors.textPrimary,
    fontSize: 24,
    fontWeight: '800',
    letterSpacing: -0.5,
    marginBottom: 4,
  },
  appSubtitle: {
    color: colors.accentPrimary,
    fontSize: 11,
    fontWeight: '800',
    letterSpacing: 1.2,
  },
  syncCard: {
    backgroundColor: colors.bgCard,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    padding: 14,
    marginBottom: 20,
  },
  syncHeaderRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    marginBottom: 6,
  },
  syncTitle: {
    color: colors.textPrimary,
    fontSize: 13,
    fontWeight: '700',
  },
  syncDesc: {
    color: colors.textMuted,
    fontSize: 12,
    lineHeight: 18,
  },
  formCard: {
    backgroundColor: colors.bgCard,
    borderRadius: 18,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    padding: 20,
    marginBottom: 20,
  },
  inputLabel: {
    color: colors.textMuted,
    fontSize: 11,
    fontWeight: '700',
    letterSpacing: 0.6,
    marginBottom: 8,
    marginTop: 10,
  },
  inputRow: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: colors.bgElevated,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    paddingHorizontal: 12,
    marginBottom: 8,
  },
  inputIcon: {
    marginRight: 10,
  },
  textInput: {
    flex: 1,
    color: colors.textPrimary,
    fontSize: 14,
    paddingVertical: 12,
  },
  loginBtn: {
    backgroundColor: colors.accentPrimary,
    borderRadius: 12,
    paddingVertical: 14,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 8,
    marginTop: 20,
  },
  loginBtnDisabled: {
    opacity: 0.7,
  },
  loginBtnText: {
    color: colors.bgBase,
    fontSize: 15,
    fontWeight: '800',
  },
  quickLoginBtn: {
    backgroundColor: 'rgba(163, 230, 53, 0.1)',
    borderWidth: 1,
    borderColor: 'rgba(163, 230, 53, 0.3)',
    borderRadius: 12,
    paddingVertical: 12,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 8,
    marginTop: 12,
  },
  quickLoginText: {
    color: colors.accentLime,
    fontSize: 13,
    fontWeight: '700',
  },
  footerNote: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 6,
    marginTop: 8,
  },
  footerText: {
    color: colors.textMuted,
    fontSize: 11,
  },
});
