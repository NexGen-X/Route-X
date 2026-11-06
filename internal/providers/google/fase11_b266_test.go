package google

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// FASE 11 B-266 (PB-21): petunjuk jeda resmi Google untuk 429 ada di BODY
// (google.rpc.RetryInfo.retryDelay), tetapi adapter hanya membaca header
// Retry-After — dan jalur error yang menumpang di body 200 tidak membaca
// petunjuk apa pun. Kedua jalur kini menghormatinya; header yang sah tetap
// didahulukan pada jalur respons HTTP.

const body429DenganRetryInfo = `{"error":{"code":429,"message":"kuota habis","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"RATE_LIMIT_EXCEEDED"},{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"7s"}]}}`

func TestB266RetryDelayDariBodySaja(t *testing.T) {
	p := &Provider{name: "uji-b266"}
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(body429DenganRetryInfo)),
	}
	err := p.errorFromResponse(resp)
	if err.RetryAfter != 7*time.Second {
		t.Fatalf("RetryAfter = %v, mau 7s dari RetryInfo body", err.RetryAfter)
	}
}

func TestB266HeaderMenangAtasBody(t *testing.T) {
	p := &Provider{name: "uji-b266"}
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": []string{"3"}},
		Body:       io.NopCloser(strings.NewReader(body429DenganRetryInfo)),
	}
	err := p.errorFromResponse(resp)
	if err.RetryAfter != 3*time.Second {
		t.Fatalf("RetryAfter = %v, mau 3s dari header (header didahulukan)", err.RetryAfter)
	}
}

func TestB266ErrorMenumpangDiBody200(t *testing.T) {
	p := &Provider{name: "uji-b266"}
	raw := `{"error":{"code":429,"message":"kuota habis","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"7s"}]}}`
	var u generateResponse
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		t.Fatalf("unmarshal respons: %v", err)
	}
	if u.Error == nil {
		t.Fatal("fixture: error menumpang tidak terbaca")
	}
	err := p.checkBlocked(&u)
	if err == nil {
		t.Fatal("checkBlocked harus mengembalikan error")
	}
	pe, ok := err.(*providers.Error)
	if !ok {
		t.Fatalf("tipe error %T, mau *providers.Error", err)
	}
	if pe.RetryAfter != 7*time.Second {
		t.Fatalf("RetryAfter = %v, mau 7s dari details error menumpang", pe.RetryAfter)
	}
}
