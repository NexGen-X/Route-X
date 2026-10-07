import React, { useState } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TouchableOpacity,
  Alert,
} from 'react-native';
import { Terminal, Copy, Check, Code2, BookOpen } from 'lucide-react-native';
import { colors } from '../theme/colors';
import { TopHeader } from '../components/TopHeader';

export interface CliScreenProps {
  onBack: () => void;
}

export const CliScreen: React.FC<CliScreenProps> = ({ onBack }) => {
  const [activeTab, setActiveTab] = useState<'cli' | 'python' | 'curl'>('cli');
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const handleCopy = (id: string, text: string) => {
    setCopiedId(id);
    Alert.alert('Tersalin ke Clipboard', `Perintah telah disalin:\n\n${text}`);
    setTimeout(() => setCopiedId(null), 2500);
  };

  const cliSnippets = [
    {
      id: 'cmd-install',
      title: '1. Inisialisasi Route-X CLI',
      desc: 'Hubungkan mesin lokal Anda dengan gateway Route-X runtime.',
      cmd: 'npm install -g routex-cli\nroutex auth --host https://13.212.164.108',
    },
    {
      id: 'cmd-test',
      title: '2. Tes Perutean Model Cepat',
      desc: 'Verifikasi latensi dan responsivitas provider AI.',
      cmd: 'routex chat --model gpt-4o --prompt "Halo Route-X!"',
    },
    {
      id: 'cmd-status',
      title: '3. Pantau Metrik Terminal Realtime',
      desc: 'Tampilkan status throughput dan sisa kuota budget.',
      cmd: 'routex status --watch --interval 2s',
    },
  ];

  const pythonSnippet = `import openai

# Route-X Drop-In Replacement untuk OpenAI SDK
client = openai.OpenAI(
    base_url="http://13.212.164.108/v1",
    api_key="rk-live-••••••••••••94f2"
)

response = client.chat.completions.create(
    model="gpt-4o",  # Auto failover ke Claude/Gemini jika down!
    messages=[
        {"role": "system", "content": "You are Route-X Enterprise AI."},
        {"role": "user", "content": "Analyze system performance."}
    ]
)
print(response.choices[0].message.content)`;

  const curlSnippet = `curl -X POST http://13.212.164.108/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer rk-live-••••••••••••94f2" \\
  -d '{
    "model": "claude-3-5-sonnet",
    "messages": [{"role": "user", "content": "Ping Route-X Gateway"}],
    "stream": false
  }'`;

  return (
    <View style={styles.container}>
      <TopHeader
        mode="subscreen"
        title="CLI & SDK Integrations"
        onBack={onBack}
      />

      {/* Tabs Filter */}
      <View style={styles.tabRow}>
        <TouchableOpacity
          style={[styles.tabBtn, activeTab === 'cli' && styles.tabBtnActive]}
          onPress={() => setActiveTab('cli')}
        >
          <Terminal size={14} color={activeTab === 'cli' ? colors.accentPrimary : colors.textSecondary} />
          <Text style={[styles.tabText, activeTab === 'cli' && styles.tabTextActive]}>
            Route-X CLI
          </Text>
        </TouchableOpacity>

        <TouchableOpacity
          style={[styles.tabBtn, activeTab === 'python' && styles.tabBtnActive]}
          onPress={() => setActiveTab('python')}
        >
          <Code2 size={14} color={activeTab === 'python' ? colors.accentPrimary : colors.textSecondary} />
          <Text style={[styles.tabText, activeTab === 'python' && styles.tabTextActive]}>
            Python SDK
          </Text>
        </TouchableOpacity>

        <TouchableOpacity
          style={[styles.tabBtn, activeTab === 'curl' && styles.tabBtnActive]}
          onPress={() => setActiveTab('curl')}
        >
          <BookOpen size={14} color={activeTab === 'curl' ? colors.accentPrimary : colors.textSecondary} />
          <Text style={[styles.tabText, activeTab === 'curl' && styles.tabTextActive]}>
            cURL / REST
          </Text>
        </TouchableOpacity>
      </View>

      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        {activeTab === 'cli' && (
          <View style={styles.snippetList}>
            {cliSnippets.map((item) => (
              <View key={item.id} style={styles.codeCard}>
                <View style={styles.cardHeader}>
                  <Text style={styles.cardTitle}>{item.title}</Text>
                  <TouchableOpacity
                    style={styles.copyBtn}
                    onPress={() => handleCopy(item.id, item.cmd)}
                  >
                    {copiedId === item.id ? (
                      <Check size={14} color={colors.accentPrimary} />
                    ) : (
                      <Copy size={14} color={colors.textSecondary} />
                    )}
                  </TouchableOpacity>
                </View>
                <Text style={styles.cardDesc}>{item.desc}</Text>
                <View style={styles.terminalBox}>
                  <Text style={styles.terminalCode}>{item.cmd}</Text>
                </View>
              </View>
            ))}
          </View>
        )}

        {activeTab === 'python' && (
          <View style={styles.codeCard}>
            <View style={styles.cardHeader}>
              <Text style={styles.cardTitle}>OpenAI SDK Python Drop-In</Text>
              <TouchableOpacity
                style={styles.copyBtn}
                onPress={() => handleCopy('python-sdk', pythonSnippet)}
              >
                {copiedId === 'python-sdk' ? (
                  <Check size={14} color={colors.accentPrimary} />
                ) : (
                  <Copy size={14} color={colors.textSecondary} />
                )}
              </TouchableOpacity>
            </View>
            <Text style={styles.cardDesc}>
              Ganti `base_url` pada client OpenAI bawaan Anda tanpa perlu mengubah kode aplikasi internal.
            </Text>
            <View style={styles.terminalBox}>
              <Text style={styles.terminalCode}>{pythonSnippet}</Text>
            </View>
          </View>
        )}

        {activeTab === 'curl' && (
          <View style={styles.codeCard}>
            <View style={styles.cardHeader}>
              <Text style={styles.cardTitle}>cURL Terminal Request</Text>
              <TouchableOpacity
                style={styles.copyBtn}
                onPress={() => handleCopy('curl-cmd', curlSnippet)}
              >
                {copiedId === 'curl-cmd' ? (
                  <Check size={14} color={colors.accentPrimary} />
                ) : (
                  <Copy size={14} color={colors.textSecondary} />
                )}
              </TouchableOpacity>
            </View>
            <Text style={styles.cardDesc}>
              Kirim request langsung melalui shell terminal atau script backend HTTP standar.
            </Text>
            <View style={styles.terminalBox}>
              <Text style={styles.terminalCode}>{curlSnippet}</Text>
            </View>
          </View>
        )}
      </ScrollView>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.bgCanvas,
  },
  tabRow: {
    flexDirection: 'row',
    paddingHorizontal: 16,
    paddingVertical: 10,
    backgroundColor: colors.bgSurface,
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
    gap: 8,
  },
  tabBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    paddingHorizontal: 12,
    paddingVertical: 8,
    borderRadius: 8,
    backgroundColor: colors.bgElevated,
  },
  tabBtnActive: {
    backgroundColor: colors.accentGreenSubtle,
    borderWidth: 1,
    borderColor: colors.accentPrimary,
  },
  tabText: {
    fontSize: 12,
    fontWeight: '600',
    color: colors.textSecondary,
  },
  tabTextActive: {
    color: colors.accentPrimary,
  },
  scrollArea: {
    flex: 1,
  },
  scrollContent: {
    padding: 16,
    paddingBottom: 24,
  },
  snippetList: {
    gap: 16,
  },
  codeCard: {
    backgroundColor: colors.bgSurface,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
    marginBottom: 16,
  },
  cardHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 6,
  },
  cardTitle: {
    fontSize: 14,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  copyBtn: {
    width: 30,
    height: 30,
    borderRadius: 6,
    backgroundColor: colors.bgElevated,
    justifyContent: 'center',
    alignItems: 'center',
  },
  cardDesc: {
    fontSize: 12,
    color: colors.textMuted,
    lineHeight: 18,
    marginBottom: 12,
  },
  terminalBox: {
    backgroundColor: '#07080A',
    borderRadius: 8,
    padding: 12,
    borderWidth: 1,
    borderColor: colors.border,
  },
  terminalCode: {
    fontFamily: 'monospace',
    fontSize: 12,
    color: colors.accentPrimary,
    lineHeight: 18,
  },
});
