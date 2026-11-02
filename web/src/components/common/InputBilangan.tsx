import React, { useEffect, useRef, useState } from 'react';

interface InputBilanganProps {
  /** Nilai bilangan bulat yang dipegang induk. */
  value: number;
  /** Dipanggil HANYA dengan bilangan bulat sah di dalam rentang. */
  onChange: (nilai: number) => void;
  min?: number;
  max?: number;
  id?: string;
  ariaLabel?: string;
  className?: string;
  disabled?: boolean;
  placeholder?: string;
}

/**
 * Input bilangan bulat yang jujur (audit3 V-FF-02/V-FF-18 dan keluarga
 * parseInt(...) || bawaan).
 *
 * Pola lama `parseInt(teks) || 100` menelan ketikan operator secara diam-
 * diam: kolom tak pernah bisa dikosongkan (teks kosong langsung diganti
 * nilai bawaan), angka 0 yang sah diganti bawaan, dan teks tak sah berubah
 * menjadi angka lain tanpa penjelasan. Komponen ini memegang TEKS MENTAH
 * secara lokal: kolom boleh dikosongkan saat mengetik, nilai hanya diteruskan
 * ke induk bila bilangan bulat sah di dalam rentang, dan teks tak sah
 * menampilkan pesan galat tanpa mengubah state induk — jadi yang tersimpan
 * selalu persis yang operator maksud.
 */
export const InputBilangan: React.FC<InputBilanganProps> = ({
  value,
  onChange,
  min,
  max,
  id,
  ariaLabel,
  className = '',
  disabled,
  placeholder,
}) => {
  const [teks, setTeks] = useState<string>(() => String(value));
  const [galat, setGalat] = useState<string | null>(null);
  // Nilai terakhir yang disepakati dengan induk; dipakai membedakan
  // perubahan eksternal (drawer dibuka untuk entitas lain) dari gema
  // ketikan sendiri.
  const nilaiSepakatRef = useRef<number>(value);

  useEffect(() => {
    if (value !== nilaiSepakatRef.current) {
      nilaiSepakatRef.current = value;
      setTeks(String(value));
      setGalat(null);
    }
  }, [value]);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const mentah = e.target.value;
    setTeks(mentah);
    const t = mentah.trim();
    if (t === '') {
      setGalat('Kolom angka tidak boleh kosong.');
      return;
    }
    if (!/^-?\d+$/.test(t)) {
      setGalat('Masukkan bilangan bulat yang sah (tanpa desimal).');
      return;
    }
    const n = Number(t);
    if (min !== undefined && n < min) {
      setGalat(`Nilai minimal ${min}.`);
      return;
    }
    if (max !== undefined && n > max) {
      setGalat(`Nilai maksimal ${max}.`);
      return;
    }
    setGalat(null);
    nilaiSepakatRef.current = n;
    onChange(n);
  };

  return (
    <div>
      <input
        id={id}
        type="number"
        inputMode="numeric"
        aria-label={ariaLabel}
        aria-invalid={galat !== null}
        min={min}
        max={max}
        value={teks}
        onChange={handleChange}
        disabled={disabled}
        placeholder={placeholder}
        className={
          className ||
          'w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent'
        }
      />
      {galat && (
        <p role="alert" className="text-[11px] text-red-400 mt-1">
          {galat}
        </p>
      )}
    </div>
  );
};
