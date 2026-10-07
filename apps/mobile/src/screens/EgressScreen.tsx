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
import { Network, ShieldCheck, Globe, Activity } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';
import { api } from '../api/client';
import { EgressPoolItem } from '../api/types';

export interface EgressScreenProps {
  onBack: () => void;
}

export const EgressScreen: React.FC<EgressScreenProps> = ({ onBack }) => {
  const [pools, setPools] = useState<EgressPoolItem[]>([]);
  const [modalVisible, setModalVisible] = useState(false);
  const [newPoolName, setNewPoolName] = useState('');
  const [newPoolRegion, setNewPoolRegion] = useState('ap-southeast-1 (Singapore)');
  const [newPoolIps, setNewPoolIps] = useState('4');

  const loadPools = async () => {
    const data = await api.getEgressPools();
    setPools([...data]);
  };

  useEffect(() => {
    loadPools();
  }, []);

  const handleToggle = async (poolId: string, currentStatus: boolean) => {
    await api.toggleEgressPool(poolId, currentStatus);
    loadPools();
  };

  const handleAddPool = async () => {
    if (!newPoolName.trim()) {
      Alert.alert('Validasi Galat', 'Nama pool tidak boleh kosong.');
      return;
    }
    await api.addEgressPool({
      name: newPoolName,
      description: 'Dedicated IP pool untuk koneksi egress aman Route-X.',
      region: newPoolRegion,
      activeIps: parseInt(newPoolIps, 10) || 2,
      totalIps: parseInt(newPoolIps, 10) || 2,
      status: 'healthy',
      tlsVersion: 'TLS 1.3 Strict',
      priority: pools.length + 1,
      active: true,
    });
    setModalVisible(false);
    setNewPoolName('');
    Alert.alert('Sukses', `Egress pool "${newPoolName}" berhasil ditambahkan.`);
    loadPools();
  };

  return (
    <View style={styles.container}>
      <TopHeader
        mode="subscreen"
        title="Egress IP Pools"
        onBack={onBack}
        onAdd={() => setModalVisible(true)}
      />

      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        {/* Banner Info */}
        <View style={styles.infoBanner}>
          <Network size={20} color={colors.accentPrimary} />
          <View style={styles.infoTextCol}>
            <Text style={styles.infoTitle}>Egress NAT & Multi-Region Gateway</Text>
            <Text style={styles.infoDesc}>
              Kelola IP statis keluar (outbound) dan terowongan TLS 1.3 untuk menghindari pemblokiran rate limit IP oleh provider AI.
            </Text>
          </View>
        </View>

        {/* Daftar Egress Pools */}
        <View style={styles.poolList}>
          {pools.map((item) => (
            <View key={item.id} style={styles.poolCard}>
              <View style={styles.cardHeaderRow}>
                <View style={styles.titleCol}>
                  <Text style={styles.poolName}>{item.name}</Text>
                  <View style={styles.regionRow}>
                    <Globe size={13} color={colors.textSecondary} />
                    <Text style={styles.regionText}>{item.region}</Text>
                  </View>
                </View>

                <Switch
                  value={item.active}
                  onValueChange={() => handleToggle(item.id, item.active)}
                  trackColor={{ false: colors.bgElevated, true: colors.accentPrimary }}
                  thumbColor="#FFFFFF"
                />
              </View>

              <Text style={styles.poolDesc}>{item.description}</Text>

              <View style={styles.cardFooterRow}>
                <View style={styles.badgeCluster}>
                  <View
                    style={[
                      styles.statusBadge,
                      item.status === 'healthy' ? styles.badgeHealthy : styles.badgeDegraded,
                    ]}
                  >
                    <View
                      style={[
                        styles.statusDot,
                        item.status === 'healthy' ? styles.dotHealthy : styles.dotDegraded,
                      ]}
                    />
                    <Text
                      style={[
                        styles.statusBadgeText,
                        item.status === 'healthy' ? styles.textHealthy : styles.textDegraded,
                      ]}
                    >
                      {item.status.toUpperCase()}
                    </Text>
                  </View>

                  <View style={styles.tlsBadge}>
                    <ShieldCheck size={12} color={colors.accentLime} />
                    <Text style={styles.tlsBadgeText}>{item.tlsVersion}</Text>
                  </View>
                </View>

                <View style={styles.ipCounter}>
                  <Activity size={13} color={colors.accentPrimary} />
                  <Text style={styles.ipCounterText}>
                    {item.activeIps}/{item.totalIps} IP Aktif
                  </Text>
                </View>
              </View>
            </View>
          ))}
        </View>
      </ScrollView>

      {/* Modal Tambah Egress Pool */}
      <Modal
        visible={modalVisible}
        transparent={true}
        animationType="slide"
        onRequestClose={() => setModalVisible(false)}
      >
        <View style={styles.modalOverlay}>
          <View style={styles.modalContent}>
            <View style={styles.modalHeader}>
              <Text style={styles.modalTitle}>Tambah Egress Pool Baru</Text>
              <TouchableOpacity onPress={() => setModalVisible(false)}>
                <Text style={styles.closeBtn}>✕</Text>
              </TouchableOpacity>
            </View>

            <Text style={styles.inputLabel}>Nama Pool</Text>
            <TextInput
              style={styles.textInput}
              placeholder="Contoh: US-West GPU Direct"
              placeholderTextColor={colors.textMuted}
              value={newPoolName}
              onChangeText={setNewPoolName}
            />

            <Text style={styles.inputLabel}>Region Kluster Cloud</Text>
            <TextInput
              style={styles.textInput}
              placeholder="ap-southeast-1 (Singapore)"
              placeholderTextColor={colors.textMuted}
              value={newPoolRegion}
              onChangeText={setNewPoolRegion}
            />

            <Text style={styles.inputLabel}>Alokasi IP Statis (Jumlah)</Text>
            <TextInput
              style={styles.textInput}
              keyboardType="numeric"
              placeholder="4"
              placeholderTextColor={colors.textMuted}
              value={newPoolIps}
              onChangeText={setNewPoolIps}
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
                onPress={handleAddPool}
              >
                <Text style={styles.submitBtnText}>Simpan Pool</Text>
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
  infoTextCol: {
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
  poolList: {
    gap: 12,
  },
  poolCard: {
    backgroundColor: colors.bgSurface,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
  },
  cardHeaderRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 8,
  },
  titleCol: {
    flex: 1,
  },
  poolName: {
    fontSize: 15,
    fontWeight: '700',
    color: colors.textPrimary,
    marginBottom: 3,
  },
  regionRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
  },
  regionText: {
    fontSize: 12,
    color: colors.textSecondary,
  },
  poolDesc: {
    fontSize: 13,
    color: colors.textMuted,
    lineHeight: 18,
    marginBottom: 14,
  },
  cardFooterRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    borderTopWidth: 1,
    borderTopColor: colors.border,
    paddingTop: 12,
  },
  badgeCluster: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
  },
  statusBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 8,
    paddingVertical: 3,
    borderRadius: 6,
    gap: 5,
  },
  badgeHealthy: {
    backgroundColor: colors.accentGreenSubtle,
  },
  badgeDegraded: {
    backgroundColor: 'rgba(234, 179, 8, 0.12)',
  },
  statusDot: {
    width: 6,
    height: 6,
    borderRadius: 3,
  },
  dotHealthy: {
    backgroundColor: colors.accentPrimary,
  },
  dotDegraded: {
    backgroundColor: colors.statusWarning,
  },
  statusBadgeText: {
    fontSize: 10,
    fontWeight: '800',
  },
  textHealthy: {
    color: colors.accentPrimary,
  },
  textDegraded: {
    color: colors.statusWarning,
  },
  tlsBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: colors.bgElevated,
    paddingHorizontal: 8,
    paddingVertical: 3,
    borderRadius: 6,
    gap: 4,
  },
  tlsBadgeText: {
    fontSize: 11,
    color: colors.accentLime,
    fontWeight: '600',
  },
  ipCounter: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
  },
  ipCounterText: {
    fontSize: 12,
    fontWeight: '700',
    color: colors.accentPrimary,
    fontFamily: 'monospace',
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
