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
  Layers,
  ShieldCheck,
  Sliders,
  FileText,
  Lock,
} from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';

export interface MenuDrawerProps {
  onClose: () => void;
  onSelectScreen: (screenName: string) => void;
  currentScreen?: string;
  onLogout?: () => void;
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
    title: 'Penyedia AI, Model & Perutean',
    items: [
      { id: 'm-providers', label: 'Providers Upstream', icon: Cpu, screenTarget: 'providers', badge: '6 Aktif' },
      { id: 'm-models', label: 'Katalog Model AI', icon: Layers, screenTarget: 'models', badge: '6 Model' },
      { id: 'm-routing', label: 'Routing & Failover', icon: GitFork, screenTarget: 'routing' },
      { id: 'm-rules', label: 'Aturan Routing Kustom', icon: Sliders, screenTarget: 'rules', badge: '3 Aturan' },
      { id: 'm-ratelimits', label: 'Batas Laju (Rate Limits)', icon: ShieldCheck, screenTarget: 'ratelimits', badge: '3 Tier' },
      { id: 'm-egress', label: 'Egress Pools', icon: Network, screenTarget: 'egress', badge: '4 Pool' },
      { id: 'm-cli', label: 'CLI Integrations', icon: Terminal, screenTarget: 'cli' },
    ],
  },
  {
    title: 'Akses, Pengguna & Anggaran',
    items: [
      { id: 'm-keys', label: 'API Keys Gateway', icon: KeyRound, screenTarget: 'apikeys', badge: '4' },
      { id: 'm-users', label: 'Users & Roles', icon: Users, screenTarget: 'users' },
      { id: 'm-budgets', label: 'Budgets & Limits', icon: PieChart, screenTarget: 'budgets', badge: '$1.2K' },
      { id: 'm-security', label: 'Profil & Keamanan', icon: Lock, screenTarget: 'security', badge: 'MFA' },
    ],
  },
  {
    title: 'Observabilitas & Sistem',
    items: [
      { id: 'm-dashboard', label: 'Dashboard Runtime', icon: Activity, screenTarget: 'dashboard' },
      { id: 'm-logs', label: 'Live Request Logs', icon: FileText, screenTarget: 'logs', badge: 'Live' },
      { id: 'm-webhooks', label: 'Webhook Alert', icon: Webhook, screenTarget: 'webhooks', badge: '3 Hooks' },
      { id: 'm-diagnostics', label: 'Diagnostics Vitals', icon: Activity, screenTarget: 'diagnostics', badge: '99.9%' },
      { id: 'm-settings', label: 'Gateway Settings', icon: Settings, screenTarget: 'settings' },
    ],
  },
];

export const MenuDrawer: React.FC<MenuDrawerProps> = ({
  onClose,
  onSelectScreen,
  currentScreen = 'home',
  onLogout,
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
            Beranda Gateway
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
        <TouchableOpacity
          style={styles.profileRow}
          onPress={() => {
            onSelectScreen('security');
            onClose();
          }}
          activeOpacity={0.8}
        >
          <View style={styles.avatarCircle}>
            <Text style={styles.avatarLetter}>A</Text>
          </View>
          <View style={styles.profileTextCol}>
            <Text style={styles.adminName}>Administrator</Text>
            <View style={styles.statusOnlineRow}>
              <View style={styles.onlineDot} />
              <Text style={styles.onlineText}>Route-X Enterprise Online</Text>
            </View>
          </View>
          <TouchableOpacity
            style={styles.logoutButton}
            accessibilityLabel="Keluar"
            accessibilityRole="button"
            onPress={() => {
              if (onLogout) {
                onLogout();
              } else {
                onSelectScreen('security');
                onClose();
              }
            }}
          >
            <LogOut size={18} color={colors.statusError} />
          </TouchableOpacity>
        </TouchableOpacity>
      </View>
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
    paddingBottom: 24,
  },
  homePill: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    backgroundColor: colors.bgCard,
    paddingVertical: 12,
    paddingHorizontal: 16,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    marginBottom: 20,
  },
  homePillActive: {
    borderColor: colors.accentPrimary,
    backgroundColor: 'rgba(56, 189, 248, 0.1)',
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
    fontSize: 11,
    fontWeight: '700',
    color: colors.textMuted,
    textTransform: 'uppercase',
    letterSpacing: 0.8,
    marginBottom: 8,
    marginLeft: 4,
  },
  groupCard: {
    backgroundColor: colors.bgCard,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    overflow: 'hidden',
  },
  menuItemRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingVertical: 13,
    paddingHorizontal: 14,
  },
  itemBorderBottom: {
    borderBottomWidth: 1,
    borderBottomColor: colors.borderSubtle,
  },
  itemRowActive: {
    backgroundColor: 'rgba(56, 189, 248, 0.08)',
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
    color: colors.textSecondary,
  },
  itemLabelActive: {
    color: colors.textPrimary,
    fontWeight: '700',
  },
  itemRight: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
  },
  badgeBox: {
    backgroundColor: colors.bgElevated,
    borderWidth: 1,
    borderColor: colors.borderSubtle,
    paddingHorizontal: 7,
    paddingVertical: 2,
    borderRadius: 6,
  },
  badgeText: {
    fontSize: 11,
    fontWeight: '700',
    color: colors.accentPrimary,
  },
  footerContainer: {
    padding: 16,
    borderTopWidth: 1,
    borderTopColor: colors.borderSubtle,
    backgroundColor: colors.bgCard,
  },
  profileRow: {
    flexDirection: 'row',
    alignItems: 'center',
  },
  avatarCircle: {
    width: 40,
    height: 40,
    borderRadius: 20,
    backgroundColor: colors.accentPrimary,
    justifyContent: 'center',
    alignItems: 'center',
    marginRight: 12,
  },
  avatarLetter: {
    fontSize: 16,
    fontWeight: '800',
    color: colors.bgBase,
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
    gap: 6,
    marginTop: 2,
  },
  onlineDot: {
    width: 7,
    height: 7,
    borderRadius: 3.5,
    backgroundColor: colors.accentLime,
  },
  onlineText: {
    fontSize: 11,
    color: colors.accentLime,
    fontWeight: '600',
  },
  logoutButton: {
    width: 36,
    height: 36,
    borderRadius: 8,
    backgroundColor: colors.bgElevated,
    justifyContent: 'center',
    alignItems: 'center',
  },
});
