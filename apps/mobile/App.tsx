import React, { useState, useEffect } from 'react';
import {
  SafeAreaView,
  StatusBar,
  StyleSheet,
  View,
  Text,
  TouchableOpacity,
  Modal,
  Alert,
  ActivityIndicator,
} from 'react-native';
import { registerRootComponent } from 'expo';
import { colors } from './src/theme/colors';
import { api } from './src/api/client';
import { LoginScreen } from './src/screens/LoginScreen';
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
import { EgressScreen } from './src/screens/EgressScreen';
import { BudgetsScreen } from './src/screens/BudgetsScreen';
import { CliScreen } from './src/screens/CliScreen';
import { DiagnosticsScreen } from './src/screens/DiagnosticsScreen';
import { SettingsScreen } from './src/screens/SettingsScreen';
import { WebhooksScreen } from './src/screens/WebhooksScreen';
import { ModelsScreen } from './src/screens/ModelsScreen';
import { RateLimitsScreen } from './src/screens/RateLimitsScreen';
import { RoutingRulesScreen } from './src/screens/RoutingRulesScreen';
import { ProfileSecurityScreen } from './src/screens/ProfileSecurityScreen';

export type ScreenId =
  | 'home'
  | 'providers'
  | 'routing'
  | 'apikeys'
  | 'users'
  | 'dashboard'
  | 'logs'
  | 'egress'
  | 'budgets'
  | 'cli'
  | 'diagnostics'
  | 'settings'
  | 'webhooks'
  | 'models'
  | 'ratelimits'
  | 'rules'
  | 'security';

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
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [checkingAuth, setCheckingAuth] = useState(true);

  useEffect(() => {
    // Otomatis verifikasi dan login default admin saat startup untuk integrasi langsung
    api.login('admin@routex.local', 'admin')
      .then((res) => {
        if (res.success) {
          setIsAuthenticated(true);
        }
      })
      .catch(() => {})
      .finally(() => {
        setCheckingAuth(false);
      });
  }, []);

  const handleLogout = async () => {
    await api.logout();
    setIsAuthenticated(false);
    setDrawerVisible(false);
    setCurrentScreen('home');
  };

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
      case 'egress':
        setCurrentScreen('egress');
        break;
      case 'budgets':
        setCurrentScreen('budgets');
        break;
      case 'cli':
        setCurrentScreen('cli');
        break;
      case 'diagnostics':
        setCurrentScreen('diagnostics');
        break;
      case 'settings':
        setCurrentScreen('settings');
        break;
      case 'webhooks':
        setCurrentScreen('webhooks');
        break;
      case 'models':
        setCurrentScreen('models');
        break;
      case 'ratelimits':
        setCurrentScreen('ratelimits');
        break;
      case 'rules':
        setCurrentScreen('rules');
        break;
      case 'security':
        setCurrentScreen('security');
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
      case 'egress':
        return <EgressScreen onBack={handleBackToHome} />;
      case 'budgets':
        return <BudgetsScreen onBack={handleBackToHome} />;
      case 'cli':
        return <CliScreen onBack={handleBackToHome} />;
      case 'diagnostics':
        return <DiagnosticsScreen onBack={handleBackToHome} />;
      case 'settings':
        return <SettingsScreen onBack={handleBackToHome} />;
      case 'webhooks':
        return <WebhooksScreen onBack={handleBackToHome} />;
      case 'models':
        return <ModelsScreen onBack={handleBackToHome} />;
      case 'ratelimits':
        return <RateLimitsScreen onBack={handleBackToHome} />;
      case 'rules':
        return <RoutingRulesScreen onBack={handleBackToHome} />;
      case 'security':
        return <ProfileSecurityScreen onBack={handleBackToHome} />;
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

  if (checkingAuth) {
    return (
      <SafeAreaView style={styles.loadingContainer}>
        <ActivityIndicator size="large" color={colors.accentPrimary} />
        <Text style={styles.loadingText}>Menghubungkan ke Route-X Gateway...</Text>
      </SafeAreaView>
    );
  }

  if (!isAuthenticated) {
    return (
      <SafeAreaView style={styles.safeArea}>
        <StatusBar barStyle="light-content" backgroundColor={colors.bgBase} />
        <LoginScreen onLoginSuccess={() => setIsAuthenticated(true)} />
      </SafeAreaView>
    );
  }

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

      {/* Menu Drawer Modal */}
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
            onLogout={handleLogout}
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
  loadingContainer: {
    flex: 1,
    backgroundColor: colors.bgBase,
    justifyContent: 'center',
    alignItems: 'center',
    gap: 16,
  },
  loadingText: {
    color: colors.textSecondary,
    fontSize: 14,
    fontWeight: '600',
  },
});
