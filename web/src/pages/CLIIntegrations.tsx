import React, { useState, useEffect, useRef, useMemo } from 'react';
import { Terminal, RefreshCw } from 'lucide-react';
import { api } from '../api/client';
import { Button } from '../components/common/Button';
import { PageHeader } from '../components/common/PageHeader';
import type { CLITool, CLIDetectedResponse, Model, RoutingRule, APIKey } from '../types';
import { useToast } from '../context/ToastContext';
import { copyTextToClipboard } from '../utils/clipboard';
import { QueryError } from '../components/common/QueryError';
import { PeringatanDaftarTerpotong } from '../components/common/PeringatanDaftarTerpotong';
import type { ToolConfig, DiagStatus } from '../components/cli/types';
import { extractDynamicComboOptions } from '../components/cli/types';
import { CLIEnvBanner } from '../components/cli/CLIEnvBanner';
import { CLIApiKeyToolbar } from '../components/cli/CLIApiKeyToolbar';
import { CLIFilterToolbar } from '../components/cli/CLIFilterToolbar';
import { statusKonfigurasiTool, verifikasiHasilApply } from '../components/cli/statusTool';
import { CLIToolCard } from '../components/cli/CLIToolCard';
import { CLIExportModal } from '../components/cli/CLIExportModal';

const CATEGORIES = [
  'all',
  'Coding Agent',
  'Terminal Assistant',
  'Local LLM',
  'Git Automation',
  'Workflow & Prompts',
  'DevOps & SRE',
];

export const CLIIntegrations: React.FC = () => {
  const { toast } = useToast();
  const [data, setData] = useState<CLIDetectedResponse | null>(null);
  const [models, setModels] = useState<Model[]>([]);
  const [rules, setRules] = useState<RoutingRule[]>([]);
  const [registeredKeys, setRegisteredKeys] = useState<APIKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  // Sumber data pilihan (model/aturan/kunci) yang terpotong saat dimuat
  // (sisa V-FF-28): pilihan konfigurasi yang tidak terbaca utuh wajib
  // menampilkan peringatan, bukan tampil seolah lengkap.
  const [sumberTerpotong, setSumberTerpotong] = useState<string[]>([]);
  // Daftar pendamping yang GAGAL dimuat (B-225) — terpisah dari
  // penanda terpotong: gagal ≠ terpotong ≠ kosong betulan.
  const [sumberGagal, setSumberGagal] = useState<string[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedCategory, setSelectedCategory] = useState<string>('all');
  const [mainTab, setMainTab] = useState<'installed' | 'all'>('installed');
  // B-489: tujuan tool dari Command Palette (App menyimpannya di
  // sessionStorage sesaat sebelum navigasi ke halaman ini) —
  // dikonsumsi SATU kali: saring tampilan ke tool itu pada tab
  // "semua" supaya kartu konfigurasinya langsung terlihat.
  //
  // B-513: konsumsi tidak lagi hanya saat mount. Bila operator
  // SUDAH berada di halaman ini, navigasi palet tidak mengubah
  // path dan halaman tidak ter-mount ulang — pilihan dulu hilang
  // senyap dan entri storage tertinggal basi untuk kunjungan
  // berikutnya. Kini konsumsi yang sama berjalan juga pada event
  // 'routex:cli-tujuan' (ditembakkan App tepat sesudah menulis
  // storage) dan saat jendela kembali fokus.
  useEffect(() => {
    const konsumsiTujuan = () => {
      let tujuan: string | null = null;
      try {
        tujuan = sessionStorage.getItem('routex_cli_tujuan');
        if (tujuan) sessionStorage.removeItem('routex_cli_tujuan');
      } catch {
        tujuan = null;
      }
      if (tujuan) {
        setMainTab('all');
        setSelectedCategory('all');
        setSearchQuery(tujuan);
      }
    };
    konsumsiTujuan();
    window.addEventListener('routex:cli-tujuan', konsumsiTujuan);
    window.addEventListener('focus', konsumsiTujuan);
    return () => {
      window.removeEventListener('routex:cli-tujuan', konsumsiTujuan);
      window.removeEventListener('focus', konsumsiTujuan);
    };
  }, []);

  // Kunci API global pengguna untuk menghasilkan skrip terminal siap pakai
  const [userApiKey, setUserApiKey] = useState<string>(() => {
    try {
      const legacy = localStorage.getItem('routex_cli_apikey');
      if (legacy !== null) {
        sessionStorage.setItem('routex_cli_apikey', legacy);
        localStorage.removeItem('routex_cli_apikey');
      }
      return sessionStorage.getItem('routex_cli_apikey') || '';
    } catch {
      return '';
    }
  });
  const [showGlobalKey, setShowGlobalKey] = useState(false);

  // Status form lokal per tool { [toolId]: { mode, target, apiKey } }
  const [toolConfigs, setToolConfigs] = useState<Record<string, ToolConfig>>({});
  const [applyingTool, setApplyingTool] = useState<string | null>(null);
  const [applySuccess, setApplySuccess] = useState<Record<string, string>>({});
  // Peringatan verifikasi sesudah apply (Fase 2 R4): konfigurasi tersimpan
  // tetapi pindai ulang tidak cocok — tidak boleh tertutup pesan sukses.
  const [applyWarning, setApplyWarning] = useState<Record<string, string>>({});
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const feedbackTimersRef = useRef<ReturnType<typeof setTimeout>[]>([]);

  useEffect(() => {
    return () => {
      feedbackTimersRef.current.forEach((t) => clearTimeout(t));
      feedbackTimersRef.current = [];
    };
  }, []);

  // Live Protocol Diagnostics status: menguji langsung endpoint inferensi asli
  const [diagStatus, setDiagStatus] = useState<Record<string, DiagStatus>>({});

  // Cek apakah user membuka dashboard dari mesin lokal (localhost) atau remote cloud
  const isLocalGateway = useMemo(() => {
    if (typeof window === 'undefined') return false;
    const h = window.location.hostname.toLowerCase();
    return h === 'localhost' || h === '127.0.0.1' || h === '::1';
  }, []);

  // Pilihan konteks lingkungan: Server (host) vs Laptop (remote client)
  const [envContext, setEnvContext] = useState<'server' | 'remote'>(
    isLocalGateway ? 'server' : 'remote',
  );

  const publicBaseURL = useMemo(() => {
    if (typeof window !== 'undefined') {
      return window.location.origin;
    }
    return 'http://localhost:8080';
  }, []);

  // Temukan aturan combo dinamis dari database
  const dynamicComboOptions = useMemo(() => extractDynamicComboOptions(rules), [rules]);

  // Modal skrip ekspor global
  const [isExportModalOpen, setIsExportModalOpen] = useState(false);

  const loadData = async () => {
    try {
      setRefreshing(true);
      const [cliRes, modelsRes, rulesRes, apiKeysRes] = await Promise.allSettled([
        api.cli.detected(),
        api.models.listAll(),
        api.routing.listAll(),
        api.apiKeys.listAll(),
      ]);

      // B-545: keadaan kesegaran sumber PENDAMPING diperbarui dari
      // hasil sumber itu sendiri, INDEPENDEN dari nasib sumber
      // utama. Versi lama memproses hasil pendamping hanya di
      // cabang cli berhasil — kegagalan SERENTAK cli+pendamping
      // membuang hasil pendamping seluruhnya, sehingga banner
      // "sebagian data gagal dimuat" tidak pernah muncul dan
      // gerbang klaim (B-512/B-525, yang membaca sumberGagal)
      // menilai dari keadaan BASI muatan sebelumnya: sumber yang
      // baru saja gagal bisa terbaca "tidak gagal" dan klaim
      // definitif atas katalog hidup lagi. Kini tiap sumber
      // dicatat/dibersihkan pada muatan yang benar-benar
      // menyangkutnya, dua arah.
      const modelsList = modelsRes.status === 'fulfilled' ? modelsRes.value?.items || [] : [];
      const rulesList = rulesRes.status === 'fulfilled' ? rulesRes.value?.items || [] : [];
      const keysList = apiKeysRes.status === 'fulfilled' ? apiKeysRes.value?.items || [] : [];

      setModels(modelsList);
      setRules(rulesList);
      setRegisteredKeys(keysList);

      const terpotong: string[] = [];
      if (modelsRes.status === 'fulfilled' && modelsRes.value?.terpotong) terpotong.push('model');
      if (rulesRes.status === 'fulfilled' && rulesRes.value?.terpotong) terpotong.push('aturan routing');
      if (apiKeysRes.status === 'fulfilled' && apiKeysRes.value?.terpotong) terpotong.push('kunci API');
      setSumberTerpotong(terpotong);

      // Kegagalan memuat daftar pendamping harus TERLIHAT (B-225):
      // versi lama mengubahnya menjadi daftar kosong tanpa jejak,
      // sehingga pemilih target tampil kosong seolah memang tidak
      // ada model/aturan/kunci yang terdaftar.
      const gagal: string[] = [];
      if (modelsRes.status === 'rejected') gagal.push('model');
      if (rulesRes.status === 'rejected') gagal.push('aturan routing');
      if (apiKeysRes.status === 'rejected') gagal.push('kunci API');
      setSumberGagal(gagal);

      if (cliRes.status === 'fulfilled' && cliRes.value) {
        setData(cliRes.value);

        const comboOpts = extractDynamicComboOptions(rulesList);
        const initialConfigs: Record<string, ToolConfig> = {};
        cliRes.value.tools.forEach((t: CLITool) => {
          const mode = t.active_mode || 'model_only';
          const fallback =
            mode === 'routing'
              ? rulesList[0]?.name || ''
              : mode === 'combo'
              ? comboOpts[0]?.value || ''
              : modelsList[0]?.model_id || '';
          initialConfigs[t.id] = {
            mode,
            target: t.active_target || fallback,
            apiKey: '',
          };
        });
        // Gabung, jangan bangun ulang (audit F7): "Pindai Ulang" versi
        // lama mengganti seluruh toolConfigs dari data server sehingga
        // pilihan mode/target/API key per-tool yang BELUM diterapkan
        // operator hilang. Konfigurasi yang sudah ada dipertahankan;
        // initialConfigs hanya menjadi bawaan untuk tool baru.
        setToolConfigs((prev) => {
          const merged: Record<string, ToolConfig> = {};
          cliRes.value.tools.forEach((t: CLITool) => {
            merged[t.id] = prev[t.id] ?? initialConfigs[t.id];
          });
          return merged;
        });
        setLoadError(null);
      } else {
        const err = cliRes.status === 'rejected' ? cliRes.reason : new Error('Gagal memuat status CLI.');
        setLoadError(err instanceof Error ? err.message : String(err));
      }
    } catch (err: unknown) {
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  };

  useEffect(() => {
    void loadData();
  }, []);

  const handleModeChange = (toolId: string, mode: 'model_only' | 'routing' | 'combo') => {
    setToolConfigs((prev) => {
      const current = prev[toolId];
      let newTarget = current?.target || '';

      if (mode === 'model_only') {
        newTarget = models[0]?.model_id || '';
      } else if (mode === 'routing') {
        newTarget = rules[0]?.name || '';
      } else if (mode === 'combo') {
        newTarget = dynamicComboOptions[0]?.value || '';
      }

      return {
        ...prev,
        [toolId]: {
          ...current,
          mode,
          target: newTarget,
        },
      };
    });
  };

  const handleTargetChange = (toolId: string, target: string) => {
    setToolConfigs((prev) => ({
      ...prev,
      [toolId]: {
        ...prev[toolId],
        target,
      },
    }));
  };

  const handleApiKeyChange = (toolId: string, apiKey: string) => {
    setToolConfigs((prev) => ({
      ...prev,
      [toolId]: {
        ...prev[toolId],
        apiKey,
      },
    }));
  };

  const handleApply = async (toolId: string) => {
    const config = toolConfigs[toolId];
    if (!config) return;

    try {
      setApplyingTool(toolId);
      const effectiveKey = config.apiKey || userApiKey || undefined;
      const res = await api.cli.configure({
        tool_id: toolId,
        mode: config.mode,
        target: config.target,
        api_key: effectiveKey,
      });

      if (res.success) {
        // Verifikasi sesudah apply (Fase 2 R4): pindai ulang daftar tool
        // dan cocokkan keadaan tool ini dengan yang baru diterapkan.
        // Pesan sukses penuh hanya bila pindai ulang membuktikannya;
        // ketidakcocokan tampil sebagai peringatan berisi keadaan aktual.
        setApplyWarning((prev) => {
          const next = { ...prev };
          delete next[toolId];
          return next;
        });
        let terverifikasi = false;
        let pesanVerifikasi = '';
        try {
          const segar = await api.cli.detected();
          setData(segar);
          // B-553: pindaian ulang verifikasi adalah muatan SEGAR
          // sumber utama yang BERHASIL — banner basi sumber itu
          // (dari Pindai Ulang yang gagal sebelumnya) dibersihkan,
          // sama seperti loadData membersihkannya pada muatan
          // penuh yang berhasil. Kesegaran dicatat per sumber:
          // keberhasilan satu muatan membersihkan penanda sumber
          // itu sendiri, bukan menunggu muatan penuh berikutnya.
          setLoadError(null);
          const ver = verifikasiHasilApply(
            segar.tools.find((t) => t.id === toolId),
            config.mode,
            config.target,
          );
          terverifikasi = ver.ok;
          pesanVerifikasi = ver.pesan ?? '';
        } catch {
          pesanVerifikasi = 'pindaian ulang untuk verifikasi tidak tersedia';
          // Pindai ulang gagal: tampilkan hasil respons configure apa
          // adanya, tetapi tetap tandai belum terverifikasi di bawah.
          setData((prev) => {
            if (!prev) return prev;
            return {
              ...prev,
              tools: prev.tools.map((t) => (t.id === toolId ? res.tool : t)),
            };
          });
        }

        if (terverifikasi) {
          setApplySuccess((prev) => ({
            ...prev,
            [toolId]: 'Konfigurasi berhasil disimpan dan terverifikasi dari pindaian ulang.',
          }));
          feedbackTimersRef.current.push(
            setTimeout(() => {
              setApplySuccess((prev) => {
                const next = { ...prev };
                delete next[toolId];
                return next;
              });
            }, 4000),
          );
        } else {
          setApplyWarning((prev) => ({
            ...prev,
            [toolId]: `Konfigurasi tersimpan, tetapi belum terverifikasi: ${pesanVerifikasi}. Periksa keadaan tool sebelum mengandalkannya.`,
          }));
          toast.warn(
            `Konfigurasi ${toolId} tersimpan tetapi verifikasi gagal: ${pesanVerifikasi}`,
            'Verifikasi Konfigurasi',
          );
        }
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(`Gagal menerapkan konfigurasi: ${msg || 'Error tidak diketahui'}`, 'Gagal Konfigurasi');
    } finally {
      setApplyingTool(null);
    }
  };

  // Uji Sambungan Nyata (Live Protocol Diagnostics)
  const handleTestConnection = async (tool: CLITool) => {
    const toolId = tool.id;
    setDiagStatus((prev) => ({ ...prev, [toolId]: { testing: true } }));
    const start = performance.now();
    const cfg = toolConfigs[toolId];
    // B-227: target uji HARUS model nyata — dari konfigurasi tool,
    // target aktifnya, atau kepala katalog. Versi lama jatuh ke
    // literal 'Atria-Dawn-Preview' bila ketiganya kosong: model itu
    // TIDAK ada di katalog (sudah deprecated, penggantinya intern-s2),
    // sehingga uji "live" menembak model karangan dan hasilnya
    // menyesatkan. Tanpa target nyata, uji dibatalkan dengan pesan
    // eksplisit SEBELUM fetch apa pun diterbitkan.
    const targetModel = cfg?.target || tool.active_target || models[0]?.model_id;
    if (!targetModel) {
      // B-525: klaim "belum ada model terdaftar di katalog" hanya
      // sah bila daftar model benar-benar termuat dan kosong. Bila
      // sumber model GAGAL dimuat, keadaan katalog belum terukur —
      // pesan harus menyatakan kegagalan muat itu, bukan keadaan
      // katalog yang definitif (saudara gerbang B-512 di
      // CLIToolCard untuk permukaan diagnostik yang sama).
      const modelsGagal = sumberGagal.includes('model');
      setDiagStatus((prev) => ({
        ...prev,
        [toolId]: {
          testing: false,
          // B-592: uji dibatalkan SEBELUM fetch apa pun — tidak ada
          // latensi yang terukur, jadi `latency` dibiarkan undefined
          // agar chip `ms` tidak mengarang angka (dulu `latency: 0`).
          ok: false,
          message: modelsGagal
            ? 'Uji koneksi dibatalkan: daftar model gagal dimuat, jadi keadaan katalog belum dapat dipastikan. Muat ulang halaman lalu coba lagi.'
            : 'Uji koneksi dibatalkan: belum ada model terdaftar di katalog untuk diuji. Sinkronkan model dari provider terlebih dahulu.',
        },
      }));
      return;
    }
    const effectiveKey = cfg?.apiKey || userApiKey || '';

    // Tenggat uji koneksi (audit3 V-FF-30): fetch mentah versi lama tanpa
    // timeout — gateway yang menggantung membuat status "menguji..." tidak
    // pernah selesai. 20 detik cukup untuk ping max_tokens=5.
    const kontroler = new AbortController();
    const timerBatal = setTimeout(() => kontroler.abort(), 20_000);
    try {
      const headers: Record<string, string> = {
        'Content-Type': 'application/json',
      };
      if (effectiveKey) {
        headers['Authorization'] = `Bearer ${effectiveKey}`;
        headers['x-api-key'] = effectiveKey;
      }

      let res: Response;
      if (toolId === 'claude') {
        headers['anthropic-version'] = '2023-06-01';
        res = await fetch('/v1/messages', {
          method: 'POST',
          headers,
          signal: kontroler.signal,
          body: JSON.stringify({
            model: targetModel,
            max_tokens: 5,
            messages: [{ role: 'user', content: 'ping' }],
          }),
        });
      } else {
        res = await fetch('/v1/chat/completions', {
          method: 'POST',
          headers,
          signal: kontroler.signal,
          body: JSON.stringify({
            model: targetModel,
            max_tokens: 5,
            messages: [{ role: 'user', content: 'ping' }],
          }),
        });
      }

      const latency = Math.round(performance.now() - start);
      if (res.ok) {
        setDiagStatus((prev) => ({
          ...prev,
          [toolId]: {
            testing: false,
            latency,
            ok: true,
            message: `200 OK • Terhubung (${latency}ms)`,
          },
        }));
      } else {
        let errDetail = `HTTP ${res.status}`;
        try {
          const errJson = await res.json();
          if (errJson?.error?.message) errDetail = errJson.error.message;
          else if (errJson?.message) errDetail = errJson.message;
        } catch {}
        setDiagStatus((prev) => ({
          ...prev,
          [toolId]: {
            testing: false,
            latency,
            ok: false,
            message: errDetail,
          },
        }));
      }
    } catch (err: unknown) {
      setDiagStatus((prev) => ({
        ...prev,
        [toolId]: {
          testing: false,
          // B-592: cabang `catch` (galat jaringan/abort/timeout) tidak
          // pernah mengukur latensi — `latency` dibiarkan undefined
          // agar chip `ms` tidak mengarang `0ms` (dulu `latency: 0`).
          // Cabang HTTP non-ok di atas tetap memakai latensi round-trip
          // terukur yang nyata.
          ok: false,
          message:
            err instanceof Error && err.name === 'AbortError'
              ? 'Uji koneksi melewati batas waktu 20 detik — gateway tidak menjawab.'
              : err instanceof Error
                ? err.message
                : 'Koneksi gagal',
        },
      }));
    } finally {
      clearTimeout(timerBatal);
    }
  };

  const handleCopy = async (text: string, key: string) => {
    if (!text) {
      toast.error('Tidak ada teks untuk disalin.');
      return;
    }
    try {
      await copyTextToClipboard(text);
      setCopiedKey(key);
      toast.success('Disalin ke clipboard.');
      feedbackTimersRef.current.push(setTimeout(() => setCopiedKey(null), 2000));
    } catch (err: unknown) {
      toast.error('Gagal menyalin: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const installedCount = useMemo(
    () => (data?.tools || []).filter((t) => t.installed).length,
    [data],
  );
  const totalCount = useMemo(() => (data?.tools || []).length, [data]);

  const filteredTools = useMemo(() => {
    return (data?.tools || []).filter((tool) => {
      const q = searchQuery.toLowerCase();
      const matchesSearch =
        (tool.name || '').toLowerCase().includes(q) ||
        (tool.id || '').toLowerCase().includes(q) ||
        (tool.description || '').toLowerCase().includes(q);

      const matchesCategory =
        selectedCategory === 'all' ||
        (tool.category || '').toLowerCase() === selectedCategory.toLowerCase();

      const matchesTab = mainTab === 'all' || tool.installed;

      return matchesSearch && matchesCategory && matchesTab;
    });
  }, [data, searchQuery, selectedCategory, mainTab]);

  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-text-muted">
        <RefreshCw className="w-8 h-8 animate-spin text-accent-text mb-4" />
        <p className="text-sm font-mono">Memindai perkakas AI CLI di sistem host...</p>
      </div>
    );
  }

  if (loadError && !data) {
    return <QueryError message={loadError} onRetry={() => void loadData()} />;
  }

  return (
    <div className="space-y-6">
      {sumberTerpotong.length > 0 && (
        <PeringatanDaftarTerpotong sumber={sumberTerpotong.join(', ')} />
      )}
      {sumberGagal.length > 0 && (
        <div
          role="status"
          aria-live="polite"
          className="text-xs text-amber-300 border border-amber-500/30 bg-amber-500/10 rounded-xl p-3 flex items-center gap-2"
        >
          <span className="w-2 h-2 rounded-full bg-amber-400 shrink-0" aria-hidden="true" />
          <span>
            Sebagian data gagal dimuat: {sumberGagal.join(', ')}. Pemilih target untuk sumber tersebut tampil
            kosong karena datanya tidak termuat — bukan karena datanya tidak ada.
          </span>
        </div>
      )}
      {/* B-534: kegagalan sumber UTAMA (cli.detected) pada Pindai
          Ulang dulu tertelan total — loadError hanya dirender pada
          cabang !data di atas, jadi begitu data pertama ada, kartu,
          lencana, dan hitungan basi tampil definitif tanpa penanda
          (kelas B-518 di sumber utama halaman). Semantik
          keadaanMuat tetap: data lama menang untuk DITAMPILKAN,
          tetapi kegagalannya wajib TERLIHAT — banner ini menandai
          daftar sebagai hasil pindai sebelumnya. */}
      {loadError && data && (
        <div
          role="status"
          aria-live="polite"
          className="text-xs text-amber-300 border border-amber-500/30 bg-amber-500/10 rounded-xl p-3 flex items-center gap-2"
        >
          <span className="w-2 h-2 rounded-full bg-amber-400 shrink-0" aria-hidden="true" />
          <span>
            Pindai ulang gagal: {loadError}. Daftar di bawah adalah hasil pindai sebelumnya — bukan hasil pindai
            terkini.
          </span>
        </div>
      )}
      <PageHeader
        title="Integrasi CLI & Editor AI"
        actions={
          <Button
            variant="secondary"
            size="sm"
            onClick={() => void loadData()}
            disabled={refreshing}
            className="gap-1.5 h-8 text-xs"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${refreshing ? 'animate-spin' : ''}`} />
            Pindai Ulang
          </Button>
        }
      />

      {/* Top Banner: Pemisah Konteks Server Host vs Laptop Remote Client */}
      <CLIEnvBanner
        envContext={envContext}
        setEnvContext={setEnvContext}
        installedCount={installedCount}
        totalCount={totalCount}
        publicBaseURL={publicBaseURL}
        userApiKey={userApiKey}
        copiedKey={copiedKey}
        onCopy={handleCopy}
        onOpenExportModal={() => setIsExportModalOpen(true)}
        onRefresh={() => void loadData()}
        refreshing={refreshing}
      />

      {/* Global API Key Helper Toolbar dengan Pilihan Cepat */}
      <CLIApiKeyToolbar
        userApiKey={userApiKey}
        setUserApiKey={setUserApiKey}
        showGlobalKey={showGlobalKey}
        setShowGlobalKey={setShowGlobalKey}
        registeredKeys={registeredKeys}
      />

      {/* Tab Filter Hierarkis & Pencarian */}
      <CLIFilterToolbar
        mainTab={mainTab}
        setMainTab={setMainTab}
        installedCount={installedCount}
        totalCount={totalCount}
        searchQuery={searchQuery}
        setSearchQuery={setSearchQuery}
        selectedCategory={selectedCategory}
        setSelectedCategory={setSelectedCategory}
        categories={CATEGORIES}
        statusCounts={{
          terkonfigurasi: (data?.tools ?? []).filter(
            (t) => statusKonfigurasiTool(t) === 'terkonfigurasi-routex',
          ).length,
          belumDikonfigurasi: (data?.tools ?? []).filter(
            (t) => statusKonfigurasiTool(t) === 'belum-dikonfigurasi',
          ).length,
        }}
      />

      {/* Grid Kartu Perkakas CLI */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
        {filteredTools.map((tool) => (
          <CLIToolCard
            key={tool.id}
            tool={tool}
            config={
              toolConfigs[tool.id] || {
                mode: tool.active_mode || 'model_only',
                target: tool.active_target || models[0]?.model_id || '',
                apiKey: '',
              }
            }
            models={models}
            rules={rules}
            dynamicComboOptions={dynamicComboOptions}
            modelsGagal={sumberGagal.includes('model')}
            rulesGagal={sumberGagal.includes('aturan routing')}
            userApiKey={userApiKey}
            publicBaseURL={publicBaseURL}
            isApplying={applyingTool === tool.id}
            successMsg={applySuccess[tool.id]}
            warningMsg={applyWarning[tool.id]}
            diag={diagStatus[tool.id]}
            copiedKey={copiedKey}
            onModeChange={handleModeChange}
            onTargetChange={handleTargetChange}
            onApiKeyChange={handleApiKeyChange}
            onApply={handleApply}
            onTestConnection={handleTestConnection}
            onCopy={handleCopy}
          />
        ))}
      </div>

      {/* Tampilan Kosong Jika Tidak Ada Perkakas yang Cocok */}
      {filteredTools.length === 0 && (
        <div className="text-center py-16 bg-bg-surface rounded-box border border-border space-y-3">
          <Terminal className="w-10 h-10 text-text-muted mx-auto opacity-50" />
          <h4 className="text-base font-semibold text-white">
            {mainTab === 'installed'
              ? 'Belum ada perkakas CLI yang terdeteksi terpasang di host'
              : 'Tidak ada perkakas CLI yang cocok dengan pencarian'}
          </h4>
          <p className="text-xs text-text-muted max-w-md mx-auto">
            {mainTab === 'installed'
              ? 'Periksa tab "Katalog Lengkap" untuk melihat seluruh perkakas AI CLI yang didukung Route-X.'
              : 'Coba ubah kata kunci pencarian atau ganti filter kategori.'}
          </p>
          {mainTab === 'installed' && (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => setMainTab('all')}
              className="text-xs gap-1.5"
            >
              <Terminal className="w-3.5 h-3.5" />
              Buka Katalog Lengkap ({totalCount})
            </Button>
          )}
        </div>
      )}

      {/* Modal Skrip Ekspor Gabungan */}
      <CLIExportModal
        isOpen={isExportModalOpen}
        onClose={() => setIsExportModalOpen(false)}
        userApiKey={userApiKey}
        copiedKey={copiedKey}
        onCopy={handleCopy}
      />
    </div>
  );
};
