import React from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TouchableOpacity,
} from 'react-native';
import {
  LucideIcon,
  Home,
  Cpu,
  GitFork,
  Terminal,
  Network,
  KeyRound,
  Users,
  PieChart,
  Webhook,
  Settings,
  Activity,
  LogOut,
  ChevronRight,
} from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';

export interface MenuDrawerProps {
  onClose: () => void;
  onSelectScreen: (screenName: string) => void;
  currentScreen?: string;
}

interface MenuItem {
  id: string;
  label: string;
  icon: LucideIcon;
  screenTarget?: string;
  badge?: string;
}

interface MenuGroup {
  title: string;
  items: MenuItem[];
}

const MENU_GROUPS: MenuGroup[] = [
  {
    title: 'Penyedia AI & Perutean',
    items: [
      { id: 'm-providers', label: 'Providers', icon: Cpu, screenTarget: 'providers', badge: '6 Aktif' },
      { id: 'm-routing', label: 'Routing & Failover', icon: GitFork, screenTarget: 'routing' },
      { id: 'm-cli', label: 'CLI Integrations', icon: Terminal, screenTarget: 'cli' },
      { id: 'm-egress', label: 'Egress Pools', icon: Network, screenTarget: 'egress', badge: '4 Pool' },
    ],
  },
  {
    title: 'Akses & Anggaran',
    items: [
      { id: 'm-keys', label: 'API Keys', icon: KeyRound, screenTarget: 'apikeys', badge: '4' },
      { id: 'm-users', label: 'Users', icon: Users, screenTarget: 'users' },
      { id: 'm-budgets', label: 'Budgets & Limits', icon: PieChart, screenTarget: 'budgets', badge: '$1.2K' },
    ],
  },
  {
    title: 'Sistem & Pengaturan',
    items: [
      { id: 'm-webhooks', label: 'Webhooks', icon: Webhook, screenTarget: 'webhooks' },
      { id: 'm-settings', label: 'Settings', icon: Settings, screenTarget: 'settings' },
      { id: 'm-diagnostics', label: 'Diagnostics', icon: Activity, screenTarget: 'diagnostics', badge: '99.9%' },
    ],
  },
];

export const MenuDrawer: React.FC<MenuDrawerProps> = ({
  onClose,
  onSelectScreen,
  currentScreen = 'home',
}) => {
  return (
    <View style={styles.container}>
      <TopHeader mode="drawer" onClose={onClose} />

      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        {/* Beranda Pill Active */}
        <TouchableOpacity
          style={[
            styles.homePill,
            currentScreen === 'home' && styles.homePillActive,
          ]}
          onPress={() => {
            onSelectScreen('home');
            onClose();
          }}
          activeOpacity={0.8}
        >
          <Home
            size={18}
            color={currentScreen === 'home' ? colors.accentPrimary : colors.textSecondary}
          />
          <Text
            style={[
              styles.homePillText,
              currentScreen === 'home' && styles.homePillTextActive,
            ]}
          >
            Beranda
          </Text>
        </TouchableOpacity>

        {/* Grup Menu */}
        {MENU_GROUPS.map((group) => (
          <View key={group.title} style={styles.groupContainer}>
            <Text style={styles.groupTitle}>{group.title}</Text>
            <View style={styles.groupCard}>
              {group.items.map((item, idx) => {
                const Icon = item.icon;
                const isSelected = currentScreen === item.screenTarget;
                const isLast = idx === group.items.length - 1;

                return (
                  <TouchableOpacity
                    key={item.id}
                    style={[
                      styles.menuItemRow,
                      !isLast && styles.itemBorderBottom,
                      isSelected && styles.itemRowActive,
                    ]}
                    onPress={() => {
                      if (item.screenTarget) {
                        onSelectScreen(item.screenTarget);
                        onClose();
                      }
                    }}
                    activeOpacity={0.7}
                  >
                    <View style={styles.itemLeft}>
                      <View style={styles.iconCircle}>
                        <Icon
                          size={17}
                          color={isSelected ? colors.accentPrimary : colors.textSecondary}
                        />
                      </View>
                      <Text
                        style={[
                          styles.itemLabel,
                          isSelected && styles.itemLabelActive,
                        ]}
                      >
                        {item.label}
                      </Text>
                    </View>

                    <View style={styles.itemRight}>
                      {item.badge && (
                        <View style={styles.badgeBox}>
                          <Text style={styles.badgeText}>{item.badge}</Text>
                        </View>
                      )}
                      <ChevronRight size={16} color={colors.textMuted} />
                    </View>
                  </TouchableOpacity>
                );
              })}
            </View>
          </View>
        ))}
      </ScrollView>

      {/* Profil Footer & Status Online */}
      <View style={styles.footerContainer}>
        <View style={styles.profileRow}>
          <View style={styles.avatarCircle}>
            <Text style={styles.avatarLetter}>A</Text>
          </View>
          <View style={styles.profileTextCol}>
            <Text style={styles.adminName}>Administrator</Text>
            <View style={styles.statusOnlineRow}>
              <View style={styles.onlineDot} />
              <Text style={styles.onlineText}>Route-X Online</Text>
            </View>
          </View>
          <TouchableOpacity
            style={styles.logoutButton}
            accessibilityLabel="Keluar"
            accessibilityRole="button"
          >
            <LogOut size={18} color={colors.statusError} />
          </TouchableOpacity>
        </View>
      </View>
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
  homePill: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    backgroundColor: colors.bgSurface,
    paddingVertical: 12,
    paddingHorizontal: 16,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    marginBottom: 20,
  },
  homePillActive: {
    borderColor: colors.accentPrimary,
    backgroundColor: colors.accentGreenSubtle,
  },
  homePillText: {
    fontSize: 15,
    fontWeight: '700',
    color: colors.textSecondary,
  },
  homePillTextActive: {
    color: colors.accentPrimary,
  },
  groupContainer: {
    marginBottom: 20,
  },
  groupTitle: {
    fontSize: 12,
    fontWeight: '700',
    color: colors.textMuted,
    textTransform: 'uppercase',
    letterSpacing: 0.6,
    marginBottom: 8,
    marginLeft: 4,
  },
  groupCard: {
    backgroundColor: colors.bgSurface,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.border,
    overflow: 'hidden',
  },
  menuItemRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingVertical: 12,
    paddingHorizontal: 14,
  },
  itemBorderBottom: {
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
  },
  itemRowActive: {
    backgroundColor: colors.accentGreenSubtle,
  },
  itemLeft: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
  },
  iconCircle: {
    width: 32,
    height: 32,
    borderRadius: 8,
    backgroundColor: colors.bgElevated,
    justifyContent: 'center',
    alignItems: 'center',
  },
  itemLabel: {
    fontSize: 14,
    fontWeight: '600',
    color: colors.textPrimary,
  },
  itemLabelActive: {
    color: colors.accentPrimary,
  },
  itemRight: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
  },
  badgeBox: {
    backgroundColor: colors.bgElevated,
    paddingHorizontal: 7,
    paddingVertical: 2,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: colors.border,
  },
  badgeText: {
    fontSize: 11,
    fontWeight: '700',
    color: colors.accentPrimary,
  },
  footerContainer: {
    padding: 16,
    borderTopWidth: 1,
    borderTopColor: colors.border,
    backgroundColor: colors.bgSurface,
  },
  profileRow: {
    flexDirection: 'row',
    alignItems: 'center',
  },
  avatarCircle: {
    width: 40,
    height: 40,
    borderRadius: 20,
    backgroundColor: colors.accentGreenSubtle,
    borderWidth: 1.5,
    borderColor: colors.accentPrimary,
    justifyContent: 'center',
    alignItems: 'center',
    marginRight: 12,
  },
  avatarLetter: {
    fontSize: 16,
    fontWeight: '800',
    color: colors.accentPrimary,
  },
  profileTextCol: {
    flex: 1,
  },
  adminName: {
    fontSize: 14,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  statusOnlineRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
    marginTop: 2,
  },
  onlineDot: {
    width: 6,
    height: 6,
    borderRadius: 3,
    backgroundColor: colors.statusOnline,
  },
  onlineText: {
    fontSize: 12,
    color: colors.statusOnline,
    fontWeight: '600',
  },
  logoutButton: {
    width: 38,
    height: 38,
    borderRadius: 10,
    backgroundColor: colors.bgElevated,
    borderWidth: 1,
    borderColor: colors.border,
    justifyContent: 'center',
    alignItems: 'center',
  },
});
