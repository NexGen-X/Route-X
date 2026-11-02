package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test regresi Batch C2: bagian dokumen kanonik diteruskan adapter sebagai
// block document native, utuh dan pada posisinya.

func TestDokumenDiteruskanSebagaiBlockNative(t *testing.T) {
	p, _ := newScriptedServer(t, textResponse)
	req := simpleRequest()
	req.Messages = []providers.Message{{
		Role: providers.RoleUser,
		Parts: []providers.ContentPart{
			{Type: providers.PartTypeText, Text: "ringkas dokumen ini"},
			{Type: providers.PartTypeDocument, Document: &providers.Document{
				MediaType: "application/pdf", Data: "JVBERi0xLjQ=",
			}},
		},
	}}

	body, err := p.buildMessagesRequest(req, false)
	if err != nil {
		t.Fatalf("buildMessagesRequest: %v", err)
	}
	var wire struct {
		Messages []struct {
			Content []struct {
				Type   string `json:"type"`
				Text   string `json:"text,omitempty"`
				Source *struct {
					Type      string `json:"type"`
					MediaType string `json:"media_type"`
					Data      string `json:"data"`
				} `json:"source,omitempty"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatalf("body bukan JSON: %v", err)
	}
	if len(wire.Messages) != 1 || len(wire.Messages[0].Content) != 2 {
		t.Fatalf("messages = %s, mau 1 pesan 2 block", body)
	}
	blok := wire.Messages[0].Content
	if blok[0].Type != "text" || blok[0].Text != "ringkas dokumen ini" {
		t.Errorf("block[0] = %+v, mau teks pendamping lebih dulu", blok[0])
	}
	if blok[1].Type != "document" || blok[1].Source == nil {
		t.Fatalf("block[1] = %+v, mau block document bersumber", blok[1])
	}
	if blok[1].Source.Type != "base64" || blok[1].Source.MediaType != "application/pdf" ||
		blok[1].Source.Data != "JVBERi0xLjQ=" {
		t.Errorf("source dokumen = %+v, mau base64 application/pdf dengan data utuh", blok[1].Source)
	}
}
