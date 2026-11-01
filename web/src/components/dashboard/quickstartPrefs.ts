// Preferensi tampilan Panduan Cepat di Dashboard (R1 Fase 2).
//
// Satu skema persistensi: kunci "routex.ui.quickstart" berisi JSON
// {dismissed, collapsed}. Nilai collapsed boleh null — artinya operator
// belum pernah mengaturnya manual, dan Dashboard bebas memakai bawaan
// cerdas berbasis status setup. Tiga kunci lama (routex-quickstart-
// dismissed, routex_dismiss_onboarding, routex_quickstart_collapsed)
// dibaca sekali sebagai migrasi, lalu dibersihkan saat penyimpanan
// berikutnya agar tidak ada lagi sumber kebenaran ganda.

export interface PrefQuickStart {
  dismissed: boolean;
  collapsed: boolean | null;
}

const KUNCI = 'routex.ui.quickstart';
const KUNCI_LAMA = [
  'routex-quickstart-dismissed',
  'routex_dismiss_onboarding',
  'routex_quickstart_collapsed',
] as const;

export function bacaPrefQuickStart(): PrefQuickStart {
  try {
    const mentah = localStorage.getItem(KUNCI);
    if (mentah) {
      const p = JSON.parse(mentah) as Partial<PrefQuickStart>;
      return {
        dismissed: p.dismissed === true,
        collapsed: typeof p.collapsed === 'boolean' ? p.collapsed : null,
      };
    }
    // Migrasi dari kunci lama.
    const lamaCollapsed = localStorage.getItem('routex_quickstart_collapsed');
    return {
      dismissed:
        localStorage.getItem('routex-quickstart-dismissed') === '1' ||
        localStorage.getItem('routex_dismiss_onboarding') === 'true',
      collapsed: lamaCollapsed === null ? null : lamaCollapsed === 'true',
    };
  } catch {
    return { dismissed: false, collapsed: null };
  }
}

export function simpanPrefQuickStart(pref: PrefQuickStart): void {
  try {
    localStorage.setItem(KUNCI, JSON.stringify(pref));
    for (const k of KUNCI_LAMA) localStorage.removeItem(k);
  } catch {
    // localStorage tidak tersedia (mode privat dsb.) — abaikan.
  }
}
