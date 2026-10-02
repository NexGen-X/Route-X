import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TextInput,
  TouchableOpacity,
  Alert,
} from 'react-native';
import { MoreHorizontal, Mail, Shield } from 'lucide-react-native';
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

  useEffect(() => {
    loadUsers();
  }, []);

  const loadUsers = async () => {
    const list = await api.getUsers();
    setUsers(list);
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
        title="Users"
        onBack={onBack}
        onSearch={() => setShowSearch(!showSearch)}
        onAdd={() =>
          Alert.alert('Undang User Baru', 'Kirim undangan akses RBAC ke email rekan tim.')
        }
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
                onPress={() =>
                  Alert.alert(
                    user.name,
                    `Role: ${user.role}\nEmail: ${user.email}\nStatus: Aktif`
                  )
                }
                accessibilityLabel="Detail User"
              >
                <MoreHorizontal size={18} color={colors.textSecondary} />
              </TouchableOpacity>
            </View>
          );
        })}
      </ScrollView>
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
    marginBottom: 12,
  },
  listSubtitle: {
    fontSize: 13,
    fontWeight: '600',
    color: colors.textSecondary,
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
});
