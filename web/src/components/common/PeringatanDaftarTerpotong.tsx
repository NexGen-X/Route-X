import React from 'react';
import { InfoBanner } from './InfoBanner';

// PeringatanDaftarTerpotong — penanda visual daftar yang tidak terbaca
// utuh (sisa audit3 V-FF-28). `fetchAllPages` di api/client berhenti di
// batas aman (25 halaman / 5.000 item) dan menandai hasilnya `terpotong`;
// permukaan daftar yang memakai hasil itu wajib menampilkan penanda ini
// agar operator tidak mengira daftar yang tampil sudah lengkap.
export interface PeringatanDaftarTerpotongProps {
  // Frasa sumber daftar, mis. "provider" atau "aturan routing, model, dan provider".
  sumber: string;
  className?: string;
}

export const PeringatanDaftarTerpotong: React.FC<PeringatanDaftarTerpotongProps> = ({
  sumber,
  className = '',
}) => (
  <InfoBanner
    variant="warning"
    className={className}
    summary={`Daftar ${sumber} mungkin tidak lengkap — sebagian data tidak terbaca karena jumlahnya melebihi batas aman pemuatan. Data yang tampil tetap valid.`}
  />
);
