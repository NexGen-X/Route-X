import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TouchableOpacity,
  Switch,
  TextInput,
  Alert,
} from 'react-native';
import { Shield, Sliders, HardDrive, Check } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';
import { api } from '../api/client';

export interface SettingsScreenProps {
  onBack: () => void;
}

export const SettingsScreen: React.FC<SettingsScreenProps> = ({ onBack }) => {
  const [port, setPort] = useState('8080');
  const [timeout, setTimeoutVal] = useState('60');
  const [maxBody, setMaxBody] = useState('10');
  const [rateLimit, setRateLimit] = useState('600');
  const [strictTls, setStrictTls] = useState(true);
  const [retention, setRetention] = useState('30');
  const [cacheResp, setCacheResp] = useState(true);

  useEffect(() => {
    const load = async () => {
      const data = await api.getSettings();
      setPort(data.gatewayPort.toString());
      setTimeoutVal(data.defaultTimeoutSec.toString());
      setMaxBody(data.maxRequestBodyMb.toString());
      setRateLimit(data.rateLimitPerMinute.toString());
      setStrictTls(data.enableStrictTls);
      setRetention(data.logRetentionDays.toString());
      setCacheResp(data.cacheResponses);
    };
    load();
  }, []);

  const handleSave = async () => {
    await api.updateSettings({
      gatewayPort: parseInt(port, 10) || 8080,
      defaultTimeoutSec: parseInt(timeout, 10) || 60,
      maxRequestBodyMb: parseInt(maxBody, 10) || 10,
      rateLimitPerMinute: parseInt(rateLimit, 10) || 600,
      enableStrictTls: strictTls,
      logRetentionDays: parseInt(retention, 10) || 30,
      cacheResponses: cacheResp,
    });
    Alert.alert('Pengaturan Disimpan', 'Konfigurasi runtime Route-X telah diperbarui secara realtime.');
  };

  return (
    <View style={styles.container}>
      <TopHeader
        mode="subscreen"
        title="Settings & Runtime"
        onBack={onBack}
      />

      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        {/* Section: Gateway Engine */}
        <View style={styles.sectionCard}>
          <View style={styles.sectionHeader}>
            <Sliders size={16} color={colors.accentPrimary} />
            <Text style={styles.sectionTitle}>Konfigurasi Port & Buffer Engine</Text>
          </View>

          <View style={styles.fieldRow}>
            <Text style={styles.fieldLabel}>Port Gateway Internal</Text>
            <TextInput
              style={styles.inputField}
              value={port}
              onChangeText={setPort}
              keyboardType="numeric"
            />
          </View>

          <View style={styles.divider} />

          <View style={styles.fieldRow}>
            <Text style={styles.fieldLabel}>Request Timeout (Detik)</Text>
            <TextInput
              style={styles.inputField}
              value={timeout}
              onChangeText={setTimeoutVal}
              keyboardType="numeric"
            />
          </View>

          <View style={styles.divider} />

          <View style={styles.fieldRow}>
            <Text style={styles.fieldLabel}>Maksimum Request Body (MB)</Text>
            <TextInput
              style={styles.inputField}
              value={maxBody}
              onChangeText={setMaxBody}
              keyboardType="numeric"
            />
          </View>
        </View>

        {/* Section: Security & Rate Limits */}
        <View style={styles.sectionCard}>
          <View style={styles.sectionHeader}>
            <Shield size={16} color={colors.accentPrimary} />
            <Text style={styles.sectionTitle}>Keamanan & Rate Limiting</Text>
          </View>

          <View style={styles.fieldRow}>
            <Text style={styles.fieldLabel}>Rate Limit Global (req/min)</Text>
            <TextInput
              style={styles.inputField}
              value={rateLimit}
              onChangeText={setRateLimit}
              keyboardType="numeric"
            />
          </View>

          <View style={styles.divider} />

          <View style={styles.switchRow}>
            <View style={styles.switchTextCol}>
              <Text style={styles.switchTitle}>Paksa Enkripsi TLS 1.3 Ketat</Text>
              <Text style={styles.switchDesc}>Tolak negosiasi cipher lawas untuk menjamin keamanan egress.</Text>
            </View>
            <Switch
              value={strictTls}
              onValueChange={setStrictTls}
              trackColor={{ false: colors.bgElevated, true: colors.accentPrimary }}
              thumbColor="#FFFFFF"
            />
          </View>
        </View>

        {/* Section: Cache & Log Retention */}
        <View style={styles.sectionCard}>
          <View style={styles.sectionHeader}>
            <HardDrive size={16} color={colors.accentPrimary} />
            <Text style={styles.sectionTitle}>Penyimpanan & Retensi Log</Text>
          </View>

          <View style={styles.fieldRow}>
            <Text style={styles.fieldLabel}>Retensi Log Audit (Hari)</Text>
            <TextInput
              style={styles.inputField}
              value={retention}
              onChangeText={setRetention}
              keyboardType="numeric"
            />
          </View>

          <View style={styles.divider} />

          <View style={styles.switchRow}>
            <View style={styles.switchTextCol}>
              <Text style={styles.switchTitle}>Caching Respons Model AI</Text>
              <Text style={styles.switchDesc}>Hemat biaya token untuk query berulang dengan Redis cache.</Text>
            </View>
            <Switch
              value={cacheResp}
              onValueChange={setCacheResp}
              trackColor={{ false: colors.bgElevated, true: colors.accentPrimary }}
              thumbColor="#FFFFFF"
            />
          </View>
        </View>

        {/* Save Button */}
        <TouchableOpacity
          style={styles.saveBtn}
          onPress={handleSave}
        >
          <Check size={18} color="#0A0B0D" />
          <Text style={styles.saveBtnText}>Simpan Pengaturan Gateway</Text>
        </TouchableOpacity>
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
  sectionCard: {
    backgroundColor: colors.bgSurface,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
    marginBottom: 16,
  },
  sectionHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    marginBottom: 14,
  },
  sectionTitle: {
    fontSize: 14,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  fieldRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    paddingVertical: 4,
  },
  fieldLabel: {
    fontSize: 13,
    color: colors.textSecondary,
    flex: 1,
  },
  inputField: {
    backgroundColor: colors.bgElevated,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 6,
    color: colors.textPrimary,
    fontFamily: 'monospace',
    fontSize: 13,
    width: 80,
    textAlign: 'center',
  },
  divider: {
    height: 1,
    backgroundColor: colors.border,
    marginVertical: 10,
  },
  switchRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    gap: 12,
  },
  switchTextCol: {
    flex: 1,
  },
  switchTitle: {
    fontSize: 13,
    fontWeight: '600',
    color: colors.textPrimary,
    marginBottom: 2,
  },
  switchDesc: {
    fontSize: 11,
    color: colors.textMuted,
    lineHeight: 16,
  },
  saveBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: colors.accentPrimary,
    borderRadius: 12,
    paddingVertical: 14,
    gap: 8,
    marginTop: 8,
  },
  saveBtnText: {
    color: '#0A0B0D',
    fontSize: 14,
    fontWeight: '700',
  },
});
