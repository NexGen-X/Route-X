// Tumpukan overlay bersama (audit3 V-FF-20).
//
// Modal dan Drawer dulu masing-masing memasang listener Escape pada window:
// satu tekanan Escape menutup SEMUA lapisan sekaligus (drawer yang terbuka
// di atas modal ikut tertutup), dan tidak ada cara bagi lapisan bawah untuk
// tahu bahwa ia bukan yang teratas. Modul ini menyimpan urutan lapisan yang
// terbuka; hanya lapisan TERATAS yang boleh menanggapi Escape. Select juga
// membaca tumpukan yang sama sebelum menutup dirinya, dan menghentikan
// rambatan Escape agar tidak mencapai listener lapisan di bawahnya.

const tumpukan: symbol[] = [];

/** Penanda satu lapisan: masuk/keluar tumpukan + cek posisi teratas. */
export function buatPenandaOverlay(): { id: symbol; masuk: () => void; keluar: () => void; teratas: () => boolean } {
  const id = Symbol('overlay');
  return {
    id,
    masuk: () => {
      if (!tumpukan.includes(id)) tumpukan.push(id);
    },
    keluar: () => {
      const idx = tumpukan.indexOf(id);
      if (idx >= 0) tumpukan.splice(idx, 1);
    },
    teratas: () => tumpukan[tumpukan.length - 1] === id,
  };
}
