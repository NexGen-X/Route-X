import { describe, it, expect } from 'vitest';
import { getErrorMessage } from './error';
import { ApiError } from '../api/client';

describe('getErrorMessage utility', () => {
  describe('Input bertipe Error standar & turunan', () => {
    it('mengembalikan pesan dari instance Error biasa', () => {
      const err = new Error('Koneksi ke database gagal');
      expect(getErrorMessage(err)).toBe('Koneksi ke database gagal');
    });

    it('mengembalikan pesan dari instance ApiError Route-X', () => {
      const apiErr = new ApiError(400, 'invalid_param', 'Nama webhook wajib diisi', 'name');
      expect(getErrorMessage(apiErr)).toBe('Nama webhook wajib diisi');
    });

    it('mengembalikan fallback jika pesan Error kosong atau hanya spasi', () => {
      expect(getErrorMessage(new Error(''))).toBe('Terjadi kesalahan sistem');
      expect(getErrorMessage(new Error('   '))).toBe('Terjadi kesalahan sistem');
    });

    it('mengekstrak data error dari properti respons gaya Axios pada objek Error', () => {
      const axiosErr = new Error('Request failed with status code 500');
      (axiosErr as unknown as { response: { data: { message: string } } }).response = {
        data: { message: 'Kapasitas Redis upstream penuh' },
      };
      expect(getErrorMessage(axiosErr)).toBe('Kapasitas Redis upstream penuh');
    });

    it('mengekstrak string data respons jika respons Axios berupa string mentah', () => {
      const axiosErr = new Error('Bad Gateway');
      (axiosErr as unknown as { response: { data: string } }).response = {
        data: '502 Bad Gateway Nginx',
      };
      expect(getErrorMessage(axiosErr)).toBe('502 Bad Gateway Nginx');
    });

    it('mengekstrak statusText jika data respons kosong', () => {
      const axiosErr = new Error('Gateway Timeout');
      (axiosErr as unknown as { response: { data: {}; statusText: string } }).response = {
        data: {},
        statusText: '504 Gateway Timeout',
      };
      expect(getErrorMessage(axiosErr)).toBe('504 Gateway Timeout');
    });
  });

  describe('Input bertipe String & JSON stringified', () => {
    it('mengembalikan string mentah yang sudah di-trim', () => {
      expect(getErrorMessage('  Gagal memvalidasi token JWT  ')).toBe('Gagal memvalidasi token JWT');
    });

    it('mengembalikan fallback jika string kosong atau hanya spasi', () => {
      expect(getErrorMessage('')).toBe('Terjadi kesalahan sistem');
      expect(getErrorMessage('   \n\t  ')).toBe('Terjadi kesalahan sistem');
    });

    it('mampu mem-parse string JSON dan mengekstrak properti message', () => {
      const jsonStr = JSON.stringify({ message: 'Kuota inferensi bulanan terlampaui' });
      expect(getErrorMessage(jsonStr)).toBe('Kuota inferensi bulanan terlampaui');
    });

    it('mampu mem-parse string JSON dengan properti error bersarang', () => {
      const jsonStr = JSON.stringify({ error: { message: 'Model claude-3-opus tidak tersedia' } });
      expect(getErrorMessage(jsonStr)).toBe('Model claude-3-opus tidak tersedia');
    });

    it('mengembalikan string apa adanya jika bukan JSON valid meskipun berawalan kurung kurawal', () => {
      const nonJson = '{ this is not valid json }';
      expect(getErrorMessage(nonJson)).toBe('{ this is not valid json }');
    });
  });

  describe('Input bertipe Objek (Plain JSON, message, code, param)', () => {
    it('mengekstrak properti message', () => {
      expect(getErrorMessage({ message: 'Token telah dicabut' })).toBe('Token telah dicabut');
    });

    it('mengekstrak properti error berupa string', () => {
      expect(getErrorMessage({ error: 'Akses ditolak' })).toBe('Akses ditolak');
    });

    it('mengekstrak properti error bersarang { error: { message } }', () => {
      expect(getErrorMessage({ error: { message: 'Kunci API tidak valid' } })).toBe('Kunci API tidak valid');
    });

    it('mengekstrak properti detail (pola FastAPI / REST API)', () => {
      expect(getErrorMessage({ detail: 'Entitas tidak ditemukan di sistem' })).toBe('Entitas tidak ditemukan di sistem');
    });

    it('mengekstrak properti code dan param bersamaan', () => {
      expect(getErrorMessage({ code: 'rate_limited', param: 'rpm' })).toBe('rate_limited (rpm)');
    });

    it('mengekstrak code saja jika param tidak ada', () => {
      expect(getErrorMessage({ code: 'unauthorized_access' })).toBe('unauthorized_access');
    });

    it('mengekstrak param saja jika code tidak ada', () => {
      expect(getErrorMessage({ param: 'provider_id' })).toBe('provider_id');
    });

    it('mengekstrak dari struktur respons Axios pada plain object', () => {
      const plainAxios = {
        response: {
          data: { error: 'Sesi kedaluwarsa, silakan login ulang' },
        },
      };
      expect(getErrorMessage(plainAxios)).toBe('Sesi kedaluwarsa, silakan login ulang');
    });

    it('mengembalikan representasi JSON string jika objek tidak memiliki kunci standar namun ada data', () => {
      const customObj = { custom_field: 'nilai error tak terduga' };
      expect(getErrorMessage(customObj)).toBe(JSON.stringify(customObj));
    });

    it('mengembalikan fallback jika objek kosong {}', () => {
      expect(getErrorMessage({})).toBe('Terjadi kesalahan sistem');
    });
  });

  describe('Input Null, Undefined, Primitif Lain', () => {
    it('mengembalikan fallback jika input null', () => {
      expect(getErrorMessage(null)).toBe('Terjadi kesalahan sistem');
    });

    it('mengembalikan fallback jika input undefined', () => {
      expect(getErrorMessage(undefined)).toBe('Terjadi kesalahan sistem');
    });

    it('mengembalikan representasi string untuk number dan boolean', () => {
      expect(getErrorMessage(404)).toBe('404');
      expect(getErrorMessage(500)).toBe('500');
      expect(getErrorMessage(false)).toBe('false');
    });

    it('menggunakan fallback custom jika disediakan', () => {
      const customFallback = 'Pesan galat kustom Route-X';
      expect(getErrorMessage(null, customFallback)).toBe(customFallback);
      expect(getErrorMessage(undefined, customFallback)).toBe(customFallback);
      expect(getErrorMessage({}, customFallback)).toBe(customFallback);
    });
  });

  describe('Input Array', () => {
    it('menggabungkan pesan-pesan dari array of strings', () => {
      expect(getErrorMessage(['Error satu', 'Error dua'])).toBe('Error satu, Error dua');
    });

    it('mengekstrak pesan dari array of error objects', () => {
      const errors = [{ message: 'Validasi gagal pada email' }, { message: 'Kata sandi terlalu pendek' }];
      expect(getErrorMessage(errors)).toBe('Validasi gagal pada email, Kata sandi terlalu pendek');
    });

    it('mengembalikan fallback jika array kosong', () => {
      expect(getErrorMessage([])).toBe('Terjadi kesalahan sistem');
    });
  });

  describe('Struktur Objek Sirkular (Circular Reference)', () => {
    it('tidak melempar TypeError dan mengembalikan pesan jika ada message di objek sirkular', () => {
      interface CircularWithMsg {
        message: string;
        self?: CircularWithMsg;
      }
      const circularObj: CircularWithMsg = { message: 'Kegagalan sirkular terdeteksi' };
      circularObj.self = circularObj;

      expect(() => getErrorMessage(circularObj)).not.toThrow();
      expect(getErrorMessage(circularObj)).toBe('Kegagalan sirkular terdeteksi');
    });

    it('tidak melempar TypeError dan mengembalikan fallback jika objek sirkular tidak memiliki pesan', () => {
      interface CircularEmpty {
        self?: CircularEmpty;
      }
      const circularEmpty: CircularEmpty = {};
      circularEmpty.self = circularEmpty;

      expect(() => getErrorMessage(circularEmpty)).not.toThrow();
      expect(getErrorMessage(circularEmpty)).toBe('Terjadi kesalahan sistem');
    });
  });
});
