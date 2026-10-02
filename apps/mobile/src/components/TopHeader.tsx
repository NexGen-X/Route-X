import React from 'react';
import { View, Text, StyleSheet, TouchableOpacity } from 'react-native';
import { Bell, ArrowLeft, Plus, Search, SlidersHorizontal, X } from 'lucide-react-native';
import { colors } from '../theme/colors';

export interface TopHeaderProps {
  mode?: 'brand' | 'subscreen' | 'drawer';
  title?: string;
  onBack?: () => void;
  onClose?: () => void;
  onAdd?: () => void;
  onSearch?: () => void;
  onSettings?: () => void;
  onNotificationPress?: () => void;
  onAvatarPress?: () => void;
  hasUnreadNotifications?: boolean;
}

export const TopHeader: React.FC<TopHeaderProps> = ({
  mode = 'brand',
  title = 'Route-X',
  onBack,
  onClose,
  onAdd,
  onSearch,
  onSettings,
  onNotificationPress,
  onAvatarPress,
  hasUnreadNotifications = true,
}) => {
  if (mode === 'drawer') {
    return (
      <View style={styles.container}>
        <View style={styles.brandRow}>
          <View style={styles.logoBadge}>
            <Text style={styles.logoText}>RX</Text>
          </View>
          <View>
            <Text style={styles.brandTitle}>Route-X</Text>
            <Text style={styles.brandSubtitle}>AI GATEWAY & RUNTIME</Text>
          </View>
        </View>
        {onClose && (
          <TouchableOpacity
            style={styles.iconButton}
            onPress={onClose}
            accessibilityLabel="Tutup Menu"
            accessibilityRole="button"
          >
            <X size={22} color={colors.textPrimary} />
          </TouchableOpacity>
        )}
      </View>
    );
  }

  if (mode === 'subscreen') {
    return (
      <View style={styles.container}>
        <View style={styles.subLeftRow}>
          {onBack && (
            <TouchableOpacity
              style={styles.backButton}
              onPress={onBack}
              accessibilityLabel="Kembali"
              accessibilityRole="button"
            >
              <ArrowLeft size={20} color={colors.textPrimary} />
            </TouchableOpacity>
          )}
          <Text style={styles.screenTitle}>{title}</Text>
        </View>

        <View style={styles.actionsRow}>
          {onSearch && (
            <TouchableOpacity
              style={styles.iconButton}
              onPress={onSearch}
              accessibilityLabel="Cari"
              accessibilityRole="button"
            >
              <Search size={20} color={colors.textSecondary} />
            </TouchableOpacity>
          )}
          {onSettings && (
            <TouchableOpacity
              style={styles.iconButton}
              onPress={onSettings}
              accessibilityLabel="Pengaturan"
              accessibilityRole="button"
            >
              <SlidersHorizontal size={20} color={colors.textSecondary} />
            </TouchableOpacity>
          )}
          {onAdd && (
            <TouchableOpacity
              style={styles.addButton}
              onPress={onAdd}
              accessibilityLabel="Tambah Baru"
              accessibilityRole="button"
            >
              <Plus size={18} color="#0A0B0D" />
            </TouchableOpacity>
          )}
        </View>
      </View>
    );
  }

  // Default mode: Brand (Home)
  return (
    <View style={styles.container}>
      <View style={styles.brandRow}>
        <View style={styles.logoBadge}>
          <Text style={styles.logoText}>RX</Text>
        </View>
        <View>
          <Text style={styles.brandTitle}>Route-X</Text>
          <Text style={styles.brandSubtitle}>AI GATEWAY & RUNTIME</Text>
        </View>
      </View>

      <View style={styles.rightCluster}>
        <TouchableOpacity
          style={styles.iconButton}
          onPress={onNotificationPress}
          accessibilityLabel="Notifikasi"
          accessibilityRole="button"
        >
          <Bell size={20} color={colors.textSecondary} />
          {hasUnreadNotifications && <View style={styles.notificationDot} />}
        </TouchableOpacity>

        <TouchableOpacity
          style={styles.avatarButton}
          onPress={onAvatarPress}
          accessibilityLabel="Profil Administrator"
          accessibilityRole="button"
        >
          <View style={styles.avatarCircle}>
            <Text style={styles.avatarText}>A</Text>
          </View>
        </TouchableOpacity>
      </View>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    height: 60,
    backgroundColor: colors.bgCanvas,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 16,
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
  },
  brandRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
  },
  subLeftRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
  },
  backButton: {
    width: 36,
    height: 36,
    borderRadius: 8,
    backgroundColor: colors.bgSurface,
    justifyContent: 'center',
    alignItems: 'center',
    borderWidth: 1,
    borderColor: colors.border,
  },
  screenTitle: {
    fontSize: 18,
    fontWeight: '700',
    color: colors.textPrimary,
    letterSpacing: -0.2,
  },
  logoBadge: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: colors.accentPrimary,
    justifyContent: 'center',
    alignItems: 'center',
    shadowColor: colors.accentPrimary,
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.4,
    shadowRadius: 6,
    elevation: 4,
  },
  logoText: {
    color: '#0A0B0D',
    fontSize: 15,
    fontWeight: '900',
    letterSpacing: -0.5,
  },
  brandTitle: {
    fontSize: 16,
    fontWeight: '800',
    color: colors.textPrimary,
    lineHeight: 18,
  },
  brandSubtitle: {
    fontSize: 9,
    fontWeight: '700',
    color: colors.accentPrimary,
    letterSpacing: 0.8,
  },
  rightCluster: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 10,
  },
  actionsRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
  },
  iconButton: {
    width: 36,
    height: 36,
    borderRadius: 8,
    backgroundColor: colors.bgSurface,
    borderWidth: 1,
    borderColor: colors.border,
    justifyContent: 'center',
    alignItems: 'center',
    position: 'relative',
  },
  addButton: {
    width: 36,
    height: 36,
    borderRadius: 8,
    backgroundColor: colors.accentPrimary,
    justifyContent: 'center',
    alignItems: 'center',
  },
  notificationDot: {
    position: 'absolute',
    top: 7,
    right: 7,
    width: 8,
    height: 8,
    borderRadius: 4,
    backgroundColor: colors.accentPrimary,
    borderWidth: 1.5,
    borderColor: colors.bgSurface,
  },
  avatarButton: {
    borderRadius: 18,
  },
  avatarCircle: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: colors.accentGreenSubtle,
    borderWidth: 1.5,
    borderColor: colors.accentPrimary,
    justifyContent: 'center',
    alignItems: 'center',
  },
  avatarText: {
    color: colors.accentPrimary,
    fontSize: 14,
    fontWeight: '700',
  },
});
