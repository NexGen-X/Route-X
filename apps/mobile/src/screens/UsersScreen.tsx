import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TextInput,
  TouchableOpacity,
  Modal,
  Alert,
} from 'react-native';
import { MoreHorizontal, Mail, Shield, UserPlus } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';
import { api } from '../api/client';
import { UserItem } from '../api/types';

export interface UsersScreenProps {
  onBack: () => void;
}

const ROLE_FILTERS = ['Semua', 'Admin', 'Operator', 'Developer', 'User'];

export const UsersScreen: React.FC<UsersScreenProps> = ({ onBack }) => {
  const [users, setUsers] = useState<UserItem[]>([]);
  const [selectedRole, setSelectedRole] = useState('Semua');
  const [searchQuery, setSearchQuery] = useState('');
  const [showSearch, setShowSearch] = useState(false);

  // Modal Tambah User
  const [modalVisible, setModalVisible] = useState(false);
  const [userName, setUserName] = useState('');
  const [userEmail, setUserEmail] = useState('');
  const [userRole, setUserRole] = useState<UserItem['role']>('Developer');

  useEffect(() => {
    loadUsers();
  }, []);

  const loadUsers = async () => {
    const list = await api.getUsers();
    setUsers([...list]);
  };

  const handleAddUser = async () => {
    if (!userName.trim() || !userEmail.trim()) {
      Alert.alert('Validasi Galat', 'Nama dan alamat email pengguna wajib diisi.');
      return;
    }
    await api.addUser(userName, userEmail, userRole);
    setModalVisible(false);
    setUserName('');
    setUserEmail('');
    Alert.alert('Sukses', `Pengguna "${userName}" (${userRole}) berhasil ditambahkan.`);
    loadUsers();
  };

  const handleUserMenu = (user: UserItem) => {
    Alert.alert(
      user.name,
      `Pilih tindakan untuk akun ${user.email}:`,
      [
        {
          text: 'Ubah Role Akses',
          onPress: () => {
            const nextRoles: Record<UserItem['role'], UserItem['role']> = {
              Admin: 'Operator',
              Operator: 'Developer',
              Developer: 'User',
              User: 'Admin',
            };
            user.role = nextRoles[user.role];
            setUsers([...users]);
            Alert.alert('Role Diperbarui', `Role ${user.name} diubah menjadi ${user.role}.`);
          },
        },
        {
          text: 'Hapus Pengguna',
          style: 'destructive',
          onPress: async () => {
            await api.deleteUser(user.id);
            loadUsers();
            Alert.alert('Pengguna Dihapus', `Akun ${user.name} telah dihapus.`);
          },
        },
        { text: 'Batal', style: 'cancel' },
      ]
    );
  };

  const filteredUsers = users.filter((u) => {
    const matchesRole = selectedRole === 'Semua' || u.role === selectedRole;
    const matchesSearch =
      searchQuery.trim() === '' ||
      u.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      u.email.toLowerCase().includes(searchQuery.toLowerCase());
    return matchesRole && matchesSearch;
  });

  const getRoleBadgeStyle = (role: string) => {
    switch (role) {
      case 'Admin':
        return { bg: colors.accentGreenSubtle, text: colors.accentPrimary };
      case 'Operator':
        return { bg: 'rgba(59, 130, 246, 0.12)', text: colors.avatarOperator };
      case 'Developer':
        return { bg: 'rgba(168, 85, 247, 0.12)', text: colors.avatarDev };
      default:
        return { bg: 'rgba(6, 182, 212, 0.12)', text: colors.avatarUser };
    }
  };

  return (
    <View style={styles.container}>
      <TopHeader
        mode="subscreen"
        title="Users & RBAC"
        onBack={onBack}
        onSearch={() => setShowSearch(!showSearch)}
        onAdd={() => setModalVisible(true)}
      />

      {showSearch && (
        <View style={styles.searchContainer}>
          <TextInput
            style={styles.searchInput}
            placeholder="Cari user berdasarkan nama atau email..."
            placeholderTextColor={colors.textMuted}
            value={searchQuery}
            onChangeText={setSearchQuery}
            autoFocus
          />
        </View>
      )}

      {/* Role Filters */}
      <View style={styles.filterSection}>
        <ScrollView
          horizontal
          showsHorizontalScrollIndicator={false}
          contentContainerStyle={styles.filterContent}
        >
          {ROLE_FILTERS.map((role) => {
            const isActive = selectedRole === role;
            return (
              <TouchableOpacity
                key={role}
                style={[styles.filterChip, isActive && styles.filterChipActive]}
                onPress={() => setSelectedRole(role)}
                activeOpacity={0.8}
              >
                <Text
                  style={[
                    styles.filterChipText,
                    isActive && styles.filterChipTextActive,
                  ]}
                >
                  {role}
                </Text>
              </TouchableOpacity>
            );
          })}
        </ScrollView>
      </View>

      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        <View style={styles.listHeaderRow}>
          <Text style={styles.listSubtitle}>
            {filteredUsers.length} Pengguna Terdaftar
          </Text>
          <TouchableOpacity
            style={styles.addUserTopBtn}
            onPress={() => setModalVisible(true)}
          >
            <UserPlus size={13} color={colors.accentPrimary} />
            <Text style={styles.addUserTopBtnText}>Tambah User</Text>
          </TouchableOpacity>
        </View>

        {filteredUsers.map((user) => {
          const roleStyle = getRoleBadgeStyle(user.role);

          return (
            <View key={user.id} style={styles.userCard}>
              <View style={styles.userCardLeft}>
                {/* Avatar Inisial Berwarna */}
                <View
                  style={[
                    styles.avatarCircle,
                    {
                      borderColor: user.avatarColor,
                      backgroundColor: `${user.avatarColor}1A`,
                    },
                  ]}
                >
                  <Text style={[styles.avatarInitial, { color: user.avatarColor }]}>
                    {user.initial}
                  </Text>
                </View>

                <View style={styles.infoCol}>
                  <View style={styles.nameRow}>
                    <Text style={styles.userName}>{user.name}</Text>
                    <View style={styles.activeBadge}>
                      <View style={styles.greenDot} />
                      <Text style={styles.activeText}>Aktif</Text>
                    </View>
                  </View>

                  <View style={styles.emailRow}>
                    <Mail size={12} color={colors.textMuted} />
                    <Text style={styles.userEmail}>{user.email}</Text>
                  </View>

                  <View style={styles.roleRow}>
                    <View
                      style={[
                        styles.roleBadge,
                        { backgroundColor: roleStyle.bg },
                      ]}
                    >
                      <Shield size={11} color={roleStyle.text} />
                      <Text
                        style={[styles.roleBadgeText, { color: roleStyle.text }]}
                      >
                        {user.role}
                      </Text>
                    </View>
                  </View>
                </View>
              </View>

              <TouchableOpacity
                style={styles.actionBtn}
                onPress={() => handleUserMenu(user)}
                accessibilityLabel="Detail User"
              >
                <MoreHorizontal size={18} color={colors.textSecondary} />
              </TouchableOpacity>
            </View>
          );
        })}
      </ScrollView>

      {/* Modal Tambah Pengguna Baru */}
      <Modal
        visible={modalVisible}
        transparent={true}
        animationType="slide"
        onRequestClose={() => setModalVisible(false)}
      >
        <View style={styles.modalOverlay}>
          <View style={styles.modalContent}>
            <View style={styles.modalHeader}>
              <Text style={styles.modalTitle}>Tambah Pengguna Baru</Text>
              <TouchableOpacity onPress={() => setModalVisible(false)}>
                <Text style={styles.closeBtn}>✕</Text>
              </TouchableOpacity>
            </View>

            <Text style={styles.inputLabel}>Nama Lengkap</Text>
            <TextInput
              style={styles.textInput}
              placeholder="Contoh: Rian Anggara"
              placeholderTextColor={colors.textMuted}
              value={userName}
              onChangeText={setUserName}
            />

            <Text style={styles.inputLabel}>Alamat Email</Text>
            <TextInput
              style={styles.textInput}
              placeholder="rian@routex.ai"
              placeholderTextColor={colors.textMuted}
              value={userEmail}
              onChangeText={setUserEmail}
              keyboardType="email-address"
              autoCapitalize="none"
            />

            <Text style={styles.inputLabel}>Peran (Role RBAC)</Text>
            <View style={styles.roleChipsRow}>
              {(['Admin', 'Operator', 'Developer', 'User'] as const).map((r) => (
                <TouchableOpacity
                  key={r}
                  style={[styles.roleChip, userRole === r && styles.roleChipActive]}
                  onPress={() => setUserRole(r)}
                >
                  <Text
                    style={[styles.roleChipText, userRole === r && styles.roleChipTextActive]}
                  >
                    {r}
                  </Text>
                </TouchableOpacity>
              ))}
            </View>

            <View style={styles.modalActionRow}>
              <TouchableOpacity
                style={styles.cancelBtn}
                onPress={() => setModalVisible(false)}
              >
                <Text style={styles.cancelBtnText}>Batal</Text>
              </TouchableOpacity>
              <TouchableOpacity
                style={styles.submitBtn}
                onPress={handleAddUser}
              >
                <Text style={styles.submitBtnText}>Simpan Pengguna</Text>
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
  searchContainer: {
    paddingHorizontal: 16,
    paddingVertical: 8,
    backgroundColor: colors.bgCanvas,
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
  },
  searchInput: {
    backgroundColor: colors.bgSurface,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 14,
    paddingVertical: 8,
    color: colors.textPrimary,
    fontSize: 14,
  },
  filterSection: {
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
    backgroundColor: colors.bgCanvas,
  },
  filterContent: {
    paddingHorizontal: 16,
    paddingVertical: 12,
    gap: 8,
  },
  filterChip: {
    backgroundColor: colors.bgSurface,
    paddingHorizontal: 16,
    paddingVertical: 8,
    borderRadius: 20,
    borderWidth: 1,
    borderColor: colors.border,
  },
  filterChipActive: {
    backgroundColor: colors.accentGreenSubtle,
    borderColor: colors.accentPrimary,
  },
  filterChipText: {
    fontSize: 13,
    fontWeight: '600',
    color: colors.textSecondary,
  },
  filterChipTextActive: {
    color: colors.accentPrimary,
    fontWeight: '700',
  },
  scrollArea: {
    flex: 1,
  },
  scrollContent: {
    padding: 16,
    paddingBottom: 24,
  },
  listHeaderRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 12,
  },
  listSubtitle: {
    fontSize: 13,
    fontWeight: '600',
    color: colors.textSecondary,
  },
  addUserTopBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
    backgroundColor: colors.accentGreenSubtle,
    paddingHorizontal: 8,
    paddingVertical: 4,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: colors.accentGreenBorder,
  },
  addUserTopBtnText: {
    fontSize: 11,
    fontWeight: '700',
    color: colors.accentPrimary,
  },
  userCard: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    backgroundColor: colors.bgSurface,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 14,
    marginBottom: 10,
  },
  userCardLeft: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    flex: 1,
  },
  avatarCircle: {
    width: 44,
    height: 44,
    borderRadius: 22,
    borderWidth: 1.5,
    justifyContent: 'center',
    alignItems: 'center',
  },
  avatarInitial: {
    fontSize: 16,
    fontWeight: '800',
  },
  infoCol: {
    flex: 1,
  },
  nameRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    marginBottom: 2,
  },
  userName: {
    fontSize: 15,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  activeBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
    backgroundColor: colors.accentGreenSubtle,
    paddingHorizontal: 6,
    paddingVertical: 1.5,
    borderRadius: 6,
  },
  greenDot: {
    width: 5,
    height: 5,
    borderRadius: 2.5,
    backgroundColor: colors.accentPrimary,
  },
  activeText: {
    fontSize: 10,
    fontWeight: '700',
    color: colors.accentPrimary,
  },
  emailRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
    marginBottom: 6,
  },
  userEmail: {
    fontSize: 12,
    color: colors.textSecondary,
  },
  roleRow: {
    flexDirection: 'row',
  },
  roleBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
    paddingHorizontal: 7,
    paddingVertical: 2,
    borderRadius: 6,
  },
  roleBadgeText: {
    fontSize: 11,
    fontWeight: '700',
  },
  actionBtn: {
    padding: 6,
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
