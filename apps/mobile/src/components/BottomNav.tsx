import React from 'react';
import { View, Text, StyleSheet, TouchableOpacity } from 'react-native';
import { LucideIcon, Home, BarChart2, FileText, Menu } from 'lucide-react-native';
import { colors } from '../theme/colors';

export type TabKey = 'home' | 'dashboard' | 'logs' | 'more';

export interface BottomNavProps {
  activeTab: TabKey;
  onTabChange: (tab: TabKey) => void;
}

interface NavItem {
  key: TabKey;
  label: string;
  IconComponent: LucideIcon;
}

const NAV_ITEMS: NavItem[] = [
  { key: 'home', label: 'Beranda', IconComponent: Home },
  { key: 'dashboard', label: 'Dashboard', IconComponent: BarChart2 },
  { key: 'logs', label: 'Log', IconComponent: FileText },
  { key: 'more', label: 'Lainnya', IconComponent: Menu },
];

export const BottomNav: React.FC<BottomNavProps> = ({ activeTab, onTabChange }) => {
  return (
    <View style={styles.container}>
      {NAV_ITEMS.map((item) => {
        const isActive = activeTab === item.key;
        const color = isActive ? colors.accentPrimary : colors.textMuted;
        const Icon = item.IconComponent;

        return (
          <TouchableOpacity
            key={item.key}
            style={styles.tabButton}
            onPress={() => onTabChange(item.key)}
            accessibilityRole="tab"
            accessibilityState={{ selected: isActive }}
            accessibilityLabel={item.label}
          >
            {isActive && <View style={styles.activeBar} />}
            <Icon size={22} color={color} />
            <Text style={[styles.tabLabel, { color }]}>{item.label}</Text>
          </TouchableOpacity>
        );
      })}
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    height: 64,
    backgroundColor: colors.bgCanvas,
    borderTopWidth: 1,
    borderTopColor: colors.border,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-around',
    paddingBottom: 4,
  },
  tabButton: {
    flex: 1,
    height: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    position: 'relative',
    gap: 4,
  },
  activeBar: {
    position: 'absolute',
    top: 0,
    width: 32,
    height: 3,
    backgroundColor: colors.accentPrimary,
    borderRadius: 2,
  },
  tabLabel: {
    fontSize: 11,
    fontWeight: '600',
    letterSpacing: 0.1,
  },
});
