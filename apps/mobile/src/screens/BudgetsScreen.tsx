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
import { PieChart, DollarSign, AlertTriangle, ShieldCheck, Edit3 } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';
import { api } from '../api/client';
import { BudgetItem } from '../api/types';

export interface BudgetsScreenProps {
  onBack: () => void;
}

export const BudgetsScreen: React.FC<BudgetsScreenProps> = ({ onBack }) => {
  const [budgets, setBudgets] = useState<BudgetItem[]>([]);
  const [selectedBudget, setSelectedBudget] = useState<BudgetItem | null>(null);
  const [editLimitModal, setEditLimitModal] = useState(false);
  const [newLimitInput, setNewLimitInput] = useState('');

  const loadBudgets = async () => {
    const data = await api.getBudgets();
    setBudgets([...data]);
  };

  useEffect(() => {
    loadBudgets();
  }, []);

  const openEditModal = (budget: BudgetItem) => {
    setSelectedBudget(budget);
    setNewLimitInput(budget.allocatedMonthly.toString());
    setEditLimitModal(true);
  };

  const handleSaveBudget = async () => {
    if (!selectedBudget) return;
    const newLimit = parseFloat(newLimitInput);
    if (isNaN(newLimit) || newLimit <= 0) {
      Alert.alert('Validasi Galat', 'Batas anggaran harus berupa angka positif.');
      return;
    }
    await api.updateBudgetLimit(selectedBudget.id, newLimit);
    setEditLimitModal(false);
    Alert.alert('Sukses', `Batas anggaran untuk "${selectedBudget.name}" berhasil diubah menjadi $${newLimit.toLocaleString()}.`);
    loadBudgets();
  };

  const totalAllocated = budgets.reduce((acc, curr) => acc + curr.allocatedMonthly, 0);
  const totalSpent = budgets.reduce((acc, curr) => acc + curr.spentMonthly, 0);
  const totalPercent = totalAllocated > 0 ? ((totalSpent / totalAllocated) * 100).toFixed(1) : '0';

  return (
    <View style={styles.container}>
      <TopHeader
        mode="subscreen"
        title="Budgets & Limits"
        onBack={onBack}
      />

      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        {/* Total Summary Card */}
        <View style={styles.summaryCard}>
          <View style={styles.summaryTopRow}>
            <View>
              <Text style={styles.summaryLabel}>Total Penggunaan Anggaran Bulan Ini</Text>
              <Text style={styles.summaryAmount}>
                ${totalSpent.toLocaleString('en-US', { minimumFractionDigits: 2 })}
              </Text>
            </View>
            <View style={styles.iconCircle}>
              <DollarSign size={20} color={colors.accentPrimary} />
            </View>
          </View>

          {/* Progress Bar */}
          <View style={styles.progressBarTrack}>
            <View
              style={[
                styles.progressBarFill,
                { width: `${Math.min(parseFloat(totalPercent), 100)}%` },
              ]}
            />
          </View>

          <View style={styles.summaryFooterRow}>
            <Text style={styles.summaryFooterText}>
              Batas Maksimal: ${totalAllocated.toLocaleString()}
            </Text>
            <Text style={styles.summaryPercentText}>{totalPercent}% Terpakai</Text>
          </View>
        </View>

        {/* Section Title */}
        <View style={styles.sectionHeader}>
          <PieChart size={16} color={colors.accentPrimary} />
          <Text style={styles.sectionTitle}>Rincian Alokasi Per Departemen</Text>
        </View>

        {/* Budget Cards List */}
        <View style={styles.budgetList}>
          {budgets.map((item) => {
            const isWarning = item.percentage >= item.alertThreshold;
            return (
              <View key={item.id} style={styles.budgetCard}>
                <View style={styles.cardHeader}>
                  <View style={styles.cardTitleCol}>
                    <Text style={styles.budgetName}>{item.name}</Text>
                    <Text style={styles.budgetSubtitle}>
                      Peringatan aktif pada {item.alertThreshold}%
                    </Text>
                  </View>
                  <TouchableOpacity
                    style={styles.editBtn}
                    onPress={() => openEditModal(item)}
                  >
                    <Edit3 size={15} color={colors.accentPrimary} />
                  </TouchableOpacity>
                </View>

                {/* Progress Bar Individual */}
                <View style={styles.progressBarTrackSm}>
                  <View
                    style={[
                      styles.progressBarFillSm,
                      { width: `${Math.min(item.percentage, 100)}%` },
                      isWarning && styles.progressBarFillWarning,
                    ]}
                  />
                </View>

                <View style={styles.cardFooter}>
                  <View style={styles.spendInfo}>
                    <Text style={styles.spendText}>
                      ${item.spentMonthly.toFixed(2)} / ${item.allocatedMonthly.toFixed(2)}
                    </Text>
                  </View>
                  <View style={styles.badgeRow}>
                    {isWarning ? (
                      <View style={styles.warningBadge}>
                        <AlertTriangle size={12} color={colors.statusWarning} />
                        <Text style={styles.warningText}>{item.percentage}% AMBANG TINGGI</Text>
                      </View>
                    ) : (
                      <View style={styles.safeBadge}>
                        <ShieldCheck size={12} color={colors.accentPrimary} />
                        <Text style={styles.safeText}>{item.percentage}% NORMAL</Text>
                      </View>
                    )}
                  </View>
                </View>
              </View>
            );
          })}
        </View>
      </ScrollView>

      {/* Modal Edit Limit Budget */}
      <Modal
        visible={editLimitModal}
        transparent={true}
        animationType="slide"
        onRequestClose={() => setEditLimitModal(false)}
      >
        <View style={styles.modalOverlay}>
          <View style={styles.modalContent}>
            <View style={styles.modalHeader}>
              <Text style={styles.modalTitle}>Ubah Batas Anggaran Bulanan</Text>
              <TouchableOpacity onPress={() => setEditLimitModal(false)}>
                <Text style={styles.closeBtn}>✕</Text>
              </TouchableOpacity>
            </View>

            {selectedBudget && (
              <>
                <Text style={styles.selectedBudgetName}>{selectedBudget.name}</Text>
                <Text style={styles.inputLabel}>Batas Maksimal Anggaran Bulanan ($ USD)</Text>
                <TextInput
                  style={styles.textInput}
                  keyboardType="numeric"
                  placeholder="Contoh: 2500"
                  placeholderTextColor={colors.textMuted}
                  value={newLimitInput}
                  onChangeText={setNewLimitInput}
                />

                <View style={styles.modalActionRow}>
                  <TouchableOpacity
                    style={styles.cancelBtn}
                    onPress={() => setEditLimitModal(false)}
                  >
                    <Text style={styles.cancelBtnText}>Batal</Text>
                  </TouchableOpacity>
                  <TouchableOpacity
                    style={styles.submitBtn}
                    onPress={handleSaveBudget}
                  >
                    <Text style={styles.submitBtnText}>Simpan Batas</Text>
                  </TouchableOpacity>
                </View>
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
  scrollArea: {
    flex: 1,
  },
  scrollContent: {
    padding: 16,
    paddingBottom: 24,
  },
  summaryCard: {
    backgroundColor: colors.bgSurface,
    borderRadius: 16,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 18,
    marginBottom: 20,
  },
  summaryTopRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 16,
  },
  summaryLabel: {
    fontSize: 12,
    color: colors.textSecondary,
    marginBottom: 4,
    fontWeight: '500',
  },
  summaryAmount: {
    fontSize: 26,
    fontWeight: '800',
    color: colors.textPrimary,
    letterSpacing: -0.5,
  },
  iconCircle: {
    width: 42,
    height: 42,
    borderRadius: 21,
    backgroundColor: colors.accentGreenSubtle,
    borderWidth: 1,
    borderColor: colors.accentGreenBorder,
    justifyContent: 'center',
    alignItems: 'center',
  },
  progressBarTrack: {
    height: 8,
    borderRadius: 4,
    backgroundColor: colors.bgElevated,
    overflow: 'hidden',
    marginBottom: 12,
  },
  progressBarFill: {
    height: '100%',
    backgroundColor: colors.accentPrimary,
    borderRadius: 4,
  },
  summaryFooterRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  summaryFooterText: {
    fontSize: 12,
    color: colors.textMuted,
  },
  summaryPercentText: {
    fontSize: 12,
    fontWeight: '700',
    color: colors.accentPrimary,
  },
  sectionHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    marginBottom: 12,
  },
  sectionTitle: {
    fontSize: 15,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  budgetList: {
    gap: 12,
  },
  budgetCard: {
    backgroundColor: colors.bgSurface,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
  },
  cardHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'flex-start',
    marginBottom: 12,
  },
  cardTitleCol: {
    flex: 1,
  },
  budgetName: {
    fontSize: 15,
    fontWeight: '700',
    color: colors.textPrimary,
    marginBottom: 3,
  },
  budgetSubtitle: {
    fontSize: 11,
    color: colors.textMuted,
  },
  editBtn: {
    width: 32,
    height: 32,
    borderRadius: 8,
    backgroundColor: colors.bgElevated,
    borderWidth: 1,
    borderColor: colors.border,
    justifyContent: 'center',
    alignItems: 'center',
  },
  progressBarTrackSm: {
    height: 6,
    borderRadius: 3,
    backgroundColor: colors.bgElevated,
    overflow: 'hidden',
    marginBottom: 12,
  },
  progressBarFillSm: {
    height: '100%',
    backgroundColor: colors.accentPrimary,
    borderRadius: 3,
  },
  progressBarFillWarning: {
    backgroundColor: colors.statusWarning,
  },
  cardFooter: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  spendInfo: {
    flex: 1,
  },
  spendText: {
    fontSize: 13,
    fontWeight: '700',
    color: colors.textPrimary,
    fontFamily: 'monospace',
  },
  badgeRow: {
    flexDirection: 'row',
  },
  safeBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: colors.accentGreenSubtle,
    paddingHorizontal: 8,
    paddingVertical: 3,
    borderRadius: 6,
    gap: 4,
  },
  safeText: {
    fontSize: 10,
    fontWeight: '800',
    color: colors.accentPrimary,
  },
  warningBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: 'rgba(234, 179, 8, 0.12)',
    paddingHorizontal: 8,
    paddingVertical: 3,
    borderRadius: 6,
    gap: 4,
  },
  warningText: {
    fontSize: 10,
    fontWeight: '800',
    color: colors.statusWarning,
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
    marginBottom: 12,
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
  selectedBudgetName: {
    fontSize: 14,
    color: colors.accentPrimary,
    fontWeight: '600',
    marginBottom: 12,
  },
  inputLabel: {
    fontSize: 12,
    fontWeight: '600',
    color: colors.textSecondary,
    marginBottom: 6,
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
    fontFamily: 'monospace',
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
