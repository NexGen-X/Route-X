import React, { useState } from 'react';
import {
  SafeAreaView,
  StatusBar,
  StyleSheet,
  View,
  Text,
  TouchableOpacity,
  Modal,
  Alert,
} from 'react-native';
import { registerRootComponent } from 'expo';
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

interface ErrorBoundaryState {
  hasError: boolean;
  error?: Error;
}

class AppErrorBoundary extends React.Component<{ children: React.ReactNode }, ErrorBoundaryState> {
  constructor(props: { children: React.ReactNode }) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: React.ErrorInfo) {
    console.error('Route-X App Error caught:', error, errorInfo);
  }

  render() {
    if (this.state.hasError) {
      return (
        <SafeAreaView style={styles.errorContainer}>
          <Text style={styles.errorTitle}>Route-X AI Gateway</Text>
          <Text style={styles.errorMessage}>
            {this.state.error?.message || 'Terjadi kesalahan saat memuat antarmuka.'}
          </Text>
          <TouchableOpacity
            style={styles.retryButton}
            onPress={() => this.setState({ hasError: false })}
          >
            <Text style={styles.retryButtonText}>Muat Ulang Antarmuka</Text>
          </TouchableOpacity>
        </SafeAreaView>
      );
    }
    return this.props.children;
  }
}

function MainApp(): React.JSX.Element {
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

export default function App(): React.JSX.Element {
  return (
    <AppErrorBoundary>
      <MainApp />
    </AppErrorBoundary>
  );
}

registerRootComponent(App);

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
  errorContainer: {
    flex: 1,
    backgroundColor: colors.bgCanvas,
    justifyContent: 'center',
    alignItems: 'center',
    padding: 24,
  },
  errorTitle: {
    fontSize: 20,
    fontWeight: '800',
    color: colors.accentPrimary,
    marginBottom: 12,
  },
  errorMessage: {
    fontSize: 14,
    color: colors.textSecondary,
    textAlign: 'center',
    marginBottom: 24,
    lineHeight: 20,
  },
  retryButton: {
    backgroundColor: colors.bgElevated,
    borderColor: colors.accentPrimary,
    borderWidth: 1,
    borderRadius: 8,
    paddingHorizontal: 20,
    paddingVertical: 12,
  },
  retryButtonText: {
    color: colors.accentPrimary,
    fontSize: 14,
    fontWeight: '700',
  },
});
