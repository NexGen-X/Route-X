package gateway

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestPenjagaAuthTidakMembocorkanSecret adalah test pengunci perilaku
// untuk trio redaksi pada providerPenjagaAuth/arusPenjagaAuth (temuan
// turunan B-210, Fase 8): kedua pembungkus memegang ActiveCredential
// ber-Secret di field tak diekspor, jadi mencetaknya lewat fmt atau slog
// tidak boleh pernah menampilkan nilai rahasianya — hanya identitas
// non-rahasia. Penegakan sistemiknya adalah TestRedaksiFieldTakDiekspor
// di internal/security; test ini membuktikan redaksinya sungguh bekerja,
// bukan sekadar metodenya ada.
func TestPenjagaAuthTidakMembocorkanSecret(t *testing.T) {
	const token = "TOKEN-RAHASIA-PENJAGA-B210"
	penjaga := providerPenjagaAuth{
		target: ProviderTarget{ID: "prov-1", Name: "deepmind-uji", Kind: "google"},
		cred:   &upstream.ActiveCredential{ID: "cred-redaksi", Secret: security.Secret(token)},
	}
	arus := arusPenjagaAuth{penjaga: &penjaga}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	logger.Info("uji redaksi", "penjaga", penjaga, "arus", arus)

	keluaran := map[string]string{
		"%v penjaga":  fmt.Sprintf("%v", penjaga),
		"%#v penjaga": fmt.Sprintf("%#v", penjaga),
		"%v *penjaga": fmt.Sprintf("%v", &penjaga),
		"%v arus":     fmt.Sprintf("%v", arus),
		"%#v arus":    fmt.Sprintf("%#v", arus),
		"slog":        buf.String(),
	}
	for nama, s := range keluaran {
		if strings.Contains(s, token) {
			t.Errorf("%s membocorkan secret: %s", nama, s)
		}
	}
	// Identitas non-rahasia tetap terbaca — redaksi bukan penghapusan total.
	if !strings.Contains(penjaga.String(), "deepmind-uji") || !strings.Contains(penjaga.String(), "cred-redaksi") {
		t.Errorf("String penjaga kehilangan identitas non-rahasia: %s", penjaga.String())
	}
}
