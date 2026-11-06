import { useEffect, useState } from 'react';
import { api } from '../api/client';

// Status keterhubungan gateway untuk chrome aplikasi (Header &
// Sidebar). Status ini DITURUNKAN dari panggilan nyata ke endpoint
// overview sistem — bukan konstanta: versi lama memasang titik hijau
// "Gateway Online"/"Live" secara hardcoded sehingga chrome aplikasi
// mengklaim sehat bahkan saat API observability/overview sedang
// gagal (B-224).
//
// Satu polling bersama dipakai semua pemakai hook (store tingkat
// modul): Header dan Sidebar me-mount bersamaan di setiap halaman,
// dan menembak endpoint yang sama dua kali adalah pemborosan yang
// tidak memberi informasi tambahan.

export type StatusGateway = 'memeriksa' | 'online' | 'offline';

const INTERVAL_PERIKSA_MS = 30_000;

let statusTerakhir: StatusGateway = 'memeriksa';
const pendengar = new Set<(s: StatusGateway) => void>();
let timer: ReturnType<typeof setInterval> | null = null;
let sedangMemuat = false;

function siarkan(s: StatusGateway) {
  statusTerakhir = s;
  pendengar.forEach((fn) => fn(s));
}

async function periksa() {
  if (sedangMemuat) return;
  sedangMemuat = true;
  try {
    await api.system.overview();
    siarkan('online');
  } catch {
    siarkan('offline');
  } finally {
    sedangMemuat = false;
  }
}

function mulai() {
  if (timer !== null) return;
  void periksa();
  timer = setInterval(() => {
    void periksa();
  }, INTERVAL_PERIKSA_MS);
}

function berhentiBilaSepi() {
  if (pendengar.size === 0 && timer !== null) {
    clearInterval(timer);
    timer = null;
  }
}

export function useGatewayStatus(): StatusGateway {
  const [status, setStatus] = useState<StatusGateway>(statusTerakhir);
  useEffect(() => {
    pendengar.add(setStatus);
    mulai();
    setStatus(statusTerakhir);
    return () => {
      pendengar.delete(setStatus);
      berhentiBilaSepi();
    };
  }, []);
  return status;
}
