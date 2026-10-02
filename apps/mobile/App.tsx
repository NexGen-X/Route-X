import React, { useState } from 'react';
import {
  SafeAreaView,
  StatusBar,
  StyleSheet,
  View,
  Modal,
  Alert,
} from 'react-native';
import { colors } from './src/theme/colors';
import { TopHeader } from './src/components/TopHeader';
import { BottomNav, TabKey } from './src/components/BottomNav';
import { HomeScreen } from './src/screens/HomeScreen';
import { MenuDrawer } from './src/screens/MenuDrawer';
import { ProvidersScreen } from './src/screens/ProvidersScreen';
import { RoutingScreen } from './src/screens/RoutingScreen';
import { ApiKeysScreen } from './src/screens/ApiKeysScreen';
import { UsersScreen } from './src/screens/UsersScreen';
import { DashboardScreen } from './src/screens/DashboardScreen';
import { LogsScreen } from './src/screens/LogsScreen';

export type ScreenId =
  | 'home'
  | 'providers'
  | 'routing'
  | 'apikeys'
  | 'users'
  | 'dashboard'
  | 'logs';

export default function App(): React.JSX.Element {
  const [currentScreen, setCurrentScreen] = useState<ScreenId>('home');
  const [activeTab, setActiveTab] = useState<TabKey>('home');
  const [drawerVisible, setDrawerVisible] = useState(false);

  const handleTabChange = (tab: TabKey) => {
    setActiveTab(tab);
    if (tab === 'more') {
      setDrawerVisible(true);
    } else if (tab === 'home') {
      setCurrentScreen('home');
    } else if (tab === 'dashboard') {
      setCurrentScreen('dashboard');
    } else if (tab === 'logs') {
      setCurrentScreen('logs');
    }
  };

  const handleNavigate = (screenName: string) => {
    switch (screenName) {
      case 'providers':
        setCurrentScreen('providers');
        break;
      case 'routing':
        setCurrentScreen('routing');
        break;
      case 'apikeys':
        setCurrentScreen('apikeys');
        break;
      case 'users':
        setCurrentScreen('users');
        break;
      case 'dashboard':
        setCurrentScreen('dashboard');
        setActiveTab('dashboard');
        break;
      case 'logs':
        setCurrentScreen('logs');
        setActiveTab('logs');
        break;
      case 'home':
      default:
        setCurrentScreen('home');
        setActiveTab('home');
        break;
    }
  };

  const handleBackToHome = () => {
    setCurrentScreen('home');
    setActiveTab('home');
  };

  const renderActiveScreen = () => {
    switch (currentScreen) {
      case 'providers':
        return <ProvidersScreen onBack={handleBackToHome} />;
      case 'routing':
        return <RoutingScreen onBack={handleBackToHome} />;
      case 'apikeys':
        return <ApiKeysScreen onBack={handleBackToHome} />;
      case 'users':
        return <UsersScreen onBack={handleBackToHome} />;
      case 'dashboard':
        return <DashboardScreen onBack={handleBackToHome} />;
      case 'logs':
        return <LogsScreen onBack={handleBackToHome} />;
      case 'home':
      default:
        return (
          <View style={styles.screenWrapper}>
            <TopHeader
              mode="brand"
              onNotificationPress={() =>
                Alert.alert('Notifikasi Route-X', 'Tidak ada anomali atau pesan error aktif.')
              }
              onAvatarPress={() => setDrawerVisible(true)}
            />
            <HomeScreen onNavigateToScreen={handleNavigate} />
          </View>
        );
    }
  };

  return (
    <SafeAreaView style={styles.safeArea}>
      <StatusBar
        barStyle="light-content"
        backgroundColor={colors.bgCanvas}
      />

      <View style={styles.container}>
        {renderActiveScreen()}

        <BottomNav
          activeTab={activeTab}
          onTabChange={handleTabChange}
        />
      </View>

      {/* Menu Drawer Modal (Layar 2) */}
      <Modal
        visible={drawerVisible}
        animationType="slide"
        transparent={false}
        onRequestClose={() => setDrawerVisible(false)}
      >
        <SafeAreaView style={styles.safeArea}>
          <MenuDrawer
            currentScreen={currentScreen}
            onClose={() => setDrawerVisible(false)}
            onSelectScreen={(screenId) => {
              handleNavigate(screenId);
              setDrawerVisible(false);
            }}
          />
        </SafeAreaView>
      </Modal>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safeArea: {
    flex: 1,
    backgroundColor: colors.bgCanvas,
  },
  container: {
    flex: 1,
    backgroundColor: colors.bgCanvas,
  },
  screenWrapper: {
    flex: 1,
  },
});
