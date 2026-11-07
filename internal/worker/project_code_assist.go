package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// ResolveCodeAssistProject menyelesaikan project Code Assist untuk
// kredensial yang sedang dipakai (B-157), menyimpannya pada sesi OAuth, dan
// mengembalikannya agar factory bisa meneruskannya ke adapter Antigravity.
//
// Urutan pemeriksaannya disengaja murah-ke-mahal:
//   - kredensial tanpa sesi OAuth (API key biasa) → "" tanpa error: memang
//     tidak ada project untuk mereka;
//   - sesi yang project-nya sudah tersimpan → nilai tersimpan, tanpa
//     panggilan keluar apa pun;
//   - sesi tanpa project → SATU panggilan loadCodeAssist dengan access token
//     kredensial aktif; hasilnya disimpan agar konstruksi adapter berikutnya
//     tidak memanggil lagi (backfill malas — sesi yang login sebelum fitur
//     ini ada sembuh sendiri tanpa login ulang).
//
// Kegagalan loadCodeAssist diteruskan sebagai error agar pemanggil mencatat
// dan melanjutkan TANPA project (perilaku generik lama); fungsi ini tidak
// pernah mengarang project. Egress pool yang dipatok sesi dihormati persis
// seperti jalur refresh: gagal memuatnya adalah error, bukan alasan jatuh ke
// koneksi langsung.
func (p *PenyegarOAuth) ResolveCodeAssistProject(ctx context.Context, providerID, credentialID string, accessToken security.Secret) (string, error) {
	if p.oauthRepo == nil {
		return "", nil
	}
	meta, err := p.oauthRepo.GetSessionByCredentialID(ctx, credentialID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("memuat sesi oauth untuk resolusi project code assist: %w", err)
	}
	if !meta.Enabled {
		return "", nil
	}
	if meta.CodeAssistProject != nil && *meta.CodeAssistProject != "" {
		return *meta.CodeAssistProject, nil
	}
	if p.client == nil {
		return "", nil
	}

	var proxyURL security.Secret
	if meta.EgressPoolID != nil && *meta.EgressPoolID != "" {
		if p.deps.Egress == nil {
			return "", errors.New("sesi memakai egress pool tetapi sumber egress tidak tersedia")
		}
		pURL, err := p.deps.Egress.ProxyURL(ctx, *meta.EgressPoolID)
		if err != nil {
			return "", fmt.Errorf("gagal memuat URL egress pool untuk loadCodeAssist: %w", err)
		}
		proxyURL = pURL
	}

	project, err := p.client.LoadCodeAssistProject(ctx, accessToken.Reveal(), proxyURL)
	if err != nil {
		return "", err
	}
	if project == "" {
		p.logger.InfoContext(ctx, "loadCodeAssist tidak menawarkan project code assist; perilaku generik tanpa project berlanjut",
			"session_id", meta.ID, "provider_id", providerID, "email", meta.AccountEmail)
		return "", nil
	}

	if err := p.oauthRepo.UpdateCodeAssistProject(ctx, meta.ID, project); err != nil {
		// Penyimpanan gagal tidak membatalkan nilai yang baru saja
		// diperoleh: permintaan ini tetap memakai project-nya, dan
		// konstruksi berikutnya akan mencoba menyelesaikan lagi.
		p.logger.WarnContext(ctx, "gagal menyimpan project code assist pada sesi oauth",
			"session_id", meta.ID, "provider_id", providerID, "error", err)
	} else {
		p.logger.InfoContext(ctx, "project code assist terselesaikan dan disimpan pada sesi oauth",
			"session_id", meta.ID, "provider_id", providerID, "email", meta.AccountEmail)
		// B-438: project yang baru terisi mengubah konteks layan
		// sesi, tetapi project tidak masuk kunci response cache —
		// entri yang tersaji dalam keadaan tanpa project wajib
		// disiram lewat callback pemilik proses (siraman cache
		// respons di cmd), sama seperti jalur exchange yang sudah
		// menyiram lewat handler-nya.
		if p.sesudahBackfillProject != nil {
			p.sesudahBackfillProject(ctx)
		}
	}
	return project, nil
}
