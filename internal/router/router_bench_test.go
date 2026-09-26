package router

import (
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// benchKandidat membuat RouteCandidate terisi untuk kebutuhan benchmark hot-path.
func benchKandidat(nama string, prioritas, bobot int, ubah ...func(*upstream.RouteCandidate)) *upstream.RouteCandidate {
	c := &upstream.RouteCandidate{
		ProviderModelID:   "pm-" + nama,
		UpstreamModelName: "model-" + nama,
		SupportsStreaming: true,
		SupportsTools:     true,
		Priority:          prioritas,
		Weight:            bobot,
		ProviderID:        "prov-" + nama,
		ProviderName:      nama,
		Kind:              upstream.KindOpenAI,
	}
	for _, f := range ubah {
		f(c)
	}
	return c
}

type benchLatencySource struct {
	latencies map[string]time.Duration
}

func (b *benchLatencySource) LatencyP95(pmID string) (time.Duration, bool) {
	d, ok := b.latencies[pmID]
	return d, ok
}

type benchCostSource struct {
	costs map[string]upstream.USD
}

func (b *benchCostSource) Cost(pmID string, _ TokenEstimate) (upstream.USD, bool) {
	c, ok := b.costs[pmID]
	return c, ok
}

// BenchmarkRule_Matches mengukur efisiensi pencocokan kriteria aturan routing
// (ModelID, APIKeyID, Capabilities) pada hot-path routing.
func BenchmarkRule_Matches(b *testing.B) {
	rule := &Rule{
		ID:                "rule-gpt4-vision",
		Name:              "gpt-4-vision-priority",
		MatchModelID:      "m-gpt-4o",
		MatchAPIKeyID:     "key-prod-enterprise-01",
		MatchCapabilities: []string{upstream.CapVision, upstream.CapTools},
		VirtualAlias:      "gpt-4o-auto",
	}

	req := Request{
		ModelID:      "m-gpt-4o",
		ModelName:    "gpt-4o-auto",
		APIKeyID:     "key-prod-enterprise-01",
		Capabilities: []string{upstream.CapVision, upstream.CapTools, upstream.CapText},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = rule.Matches(req)
	}
}

// BenchmarkSelectCandidate_Priority mengukur kecepatan seleksi kandidat
// berbasis prioritas berjenjang (Priority ascending -> Weight descending -> ProviderName).
func BenchmarkSelectCandidate_Priority(b *testing.B) {
	s := NewSelector()
	rule := &Rule{Strategy: StrategyPriority}
	cands := []*upstream.RouteCandidate{
		benchKandidat("azure-eastus", 1, 50),
		benchKandidat("openai-direct", 2, 80),
		benchKandidat("aws-bedrock", 1, 40),
		benchKandidat("gcp-vertex", 3, 20),
		benchKandidat("azure-westeurope", 2, 70),
	}
	req := Request{ModelID: "m-gpt-4o"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.Order(req, rule, cands)
	}
}

// BenchmarkSelectCandidate_Weighted mengukur performa seleksi weighted routing
// dengan algoritma Efraimidis-Spirakis dan FNV-1a deterministic jitter tie-breaker.
func BenchmarkSelectCandidate_Weighted(b *testing.B) {
	s := NewSelector()
	rule := &Rule{Strategy: StrategyWeighted}
	cands := []*upstream.RouteCandidate{
		benchKandidat("provider-alpha", 1, 60),
		benchKandidat("provider-bravo", 1, 30),
		benchKandidat("provider-charlie", 1, 10),
		benchKandidat("provider-delta", 1, 25),
		benchKandidat("provider-echo", 1, 15),
	}
	req := Request{ModelID: "m-claude-3-5-sonnet"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.Order(req, rule, cands)
	}
}

// BenchmarkSelectCandidate_RoundRobin mengukur throughput perputaran kandidat
// secara atomik in-memory berbasis rotasiKey.
func BenchmarkSelectCandidate_RoundRobin(b *testing.B) {
	s := NewSelector()
	rule := &Rule{ID: "rule-rr-prod", Strategy: StrategyRoundRobin}
	cands := []*upstream.RouteCandidate{
		benchKandidat("node-01", 1, 10),
		benchKandidat("node-02", 1, 10),
		benchKandidat("node-03", 1, 10),
		benchKandidat("node-04", 1, 10),
	}
	req := Request{ModelID: "m-llama-3-70b"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.Order(req, rule, cands)
	}
}

// BenchmarkSelectCandidate_LowestLatency mengukur efisiensi pemilihan kandidat
// berdasarkan P95 latensi terukur dengan fallback kandidat tak terukur di posisi belakang.
func BenchmarkSelectCandidate_LowestLatency(b *testing.B) {
	latSrc := &benchLatencySource{
		latencies: map[string]time.Duration{
			"pm-azure-eastus":     180 * time.Millisecond,
			"pm-openai-direct":    240 * time.Millisecond,
			"pm-aws-bedrock":      145 * time.Millisecond,
			"pm-azure-westeurope": 310 * time.Millisecond,
		},
	}
	s := NewSelector(WithLatencySource(latSrc))
	rule := &Rule{Strategy: StrategyLowestLatency}
	cands := []*upstream.RouteCandidate{
		benchKandidat("azure-eastus", 1, 10),
		benchKandidat("openai-direct", 1, 10),
		benchKandidat("aws-bedrock", 1, 10),
		benchKandidat("azure-westeurope", 1, 10),
		benchKandidat("unmeasured-fallback", 1, 10),
	}
	req := Request{ModelID: "m-gpt-4o"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.Order(req, rule, cands)
	}
}

// BenchmarkSelectCandidate_LowestCost mengukur seleksi kandidat biaya terendah
// berdasarkan estimasi token input/output.
func BenchmarkSelectCandidate_LowestCost(b *testing.B) {
	costSrc := &benchCostSource{
		costs: map[string]upstream.USD{
			"pm-deepseek-v3":   upstream.USD(14000),
			"pm-qwen-2.5":      upstream.USD(20000),
			"pm-gpt-4o-mini":   upstream.USD(15000),
			"pm-gemini-1.5-fl": upstream.USD(7500),
		},
	}
	s := NewSelector(WithCostSource(costSrc))
	rule := &Rule{Strategy: StrategyLowestCost}
	cands := []*upstream.RouteCandidate{
		benchKandidat("deepseek-v3", 1, 10),
		benchKandidat("qwen-2.5", 1, 10),
		benchKandidat("gpt-4o-mini", 1, 10),
		benchKandidat("gemini-1.5-fl", 1, 10),
		benchKandidat("unpriced-fallback", 1, 10),
	}
	req := Request{
		ModelID: "m-cheap-chat",
		Tokens:  TokenEstimate{InputTokens: 1000, OutputTokens: 500},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.Order(req, rule, cands)
	}
}

// BenchmarkComboPipeline_Evaluate mengukur evaluasi failover cascading multi-model
// combo pipeline pada engine router.
func BenchmarkComboPipeline_Evaluate(b *testing.B) {
	engine := NewEngine(nil, nil, nil)
	rule := &Rule{
		ID:   "rule-combo-cascade",
		Name: "multi-model-cascade",
		Pipeline: &ComboPipeline{
			Strategy: StrategyPriority,
			Attempts: 6,
			Models:   []string{"primary-gpt4o", "secondary-claude", "tertiary-gemini"},
		},
	}
	cands := []*upstream.RouteCandidate{
		benchKandidat("azure-gpt4o", 1, 10, func(c *upstream.RouteCandidate) { c.UpstreamModelName = "primary-gpt4o" }),
		benchKandidat("openai-gpt4o", 2, 20, func(c *upstream.RouteCandidate) { c.UpstreamModelName = "primary-gpt4o" }),
		benchKandidat("aws-claude-sonnet", 1, 10, func(c *upstream.RouteCandidate) { c.UpstreamModelName = "secondary-claude" }),
		benchKandidat("gcp-gemini-pro", 1, 10, func(c *upstream.RouteCandidate) { c.UpstreamModelName = "tertiary-gemini" }),
	}
	req := Request{ModelID: "primary-gpt4o", ModelName: "multi-model-cascade"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = engine.OrderCandidates(req, rule.ID, rule.Pipeline.Strategy, cands)
	}
}

// BenchmarkDeterministicTieBreaker mengukur throughput komputasi FNV-1a 64-bit
// jitter tie-breaker pada jalur pemutus seri bobot identik.
func BenchmarkDeterministicTieBreaker(b *testing.B) {
	const salt = uint64(0xfeedfacecafebeef)
	const pmID = "pm-azure-openai-eastus2-gpt-4o"
	const pID = "prov-azure-enterprise-01"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = DeterministicTieBreaker(salt+uint64(i), pmID, pID)
	}
}
