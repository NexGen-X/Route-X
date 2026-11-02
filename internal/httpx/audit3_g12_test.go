package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Grup 12 audit3 (V-FJ-06): penghitung in-flight mengukur request gateway
// yang sedang dilayani — naik selama handler berjalan, kembali sesudahnya.

func TestTrackGatewayInFlightGrup12(t *testing.T) {
	dasar := GatewayInFlight()
	var selama int64
	handler := TrackGatewayInFlight(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		selama = GatewayInFlight()
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/chat/completions", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}
	if selama != dasar+1 {
		t.Errorf("in-flight selama handler = %d, mau %d (dasar+1)", selama, dasar+1)
	}
	if got := GatewayInFlight(); got != dasar {
		t.Errorf("in-flight sesudah handler = %d, mau kembali ke dasar %d", got, dasar)
	}
}

func TestTrackGatewayInFlightPanikTetapTurunGrup12(t *testing.T) {
	dasar := GatewayInFlight()
	handler := TrackGatewayInFlight(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("ledakan uji")
	}))

	defer func() {
		if recover() == nil {
			t.Fatal("panik handler harus diteruskan, bukan ditelan middleware")
		}
		if got := GatewayInFlight(); got != dasar {
			t.Errorf("in-flight sesudah panik = %d, mau dasar %d", got, dasar)
		}
	}()
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/models", nil))
}
