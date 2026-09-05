// Package responsecache menyediakan caching respons inferensi cerdas berbasis Redis
// untuk memotong latensi hingga < 2ms dan menghemat biaya token untuk prompt berulang.
package responsecache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// DefaultTTL bawaan untuk entri cache respons (1 jam).
const DefaultTTL = 1 * time.Hour

// Entry adalah satu respons inferensi yang disimpan di Redis.
type Entry struct {
	Model        string          `json:"model"`
	Raw          json.RawMessage `json:"raw"`
	Usage        providers.Usage `json:"usage"`
	StreamChunks []string        `json:"stream_chunks,omitempty"`
	CachedAt     time.Time       `json:"cached_at"`
}

// Stats merangkum metrik performa response cache.
type Stats struct {
	Enabled      bool  `json:"enabled"`
	TTLSeconds   int64 `json:"ttl_seconds"`
	Hits         int64 `json:"hits"`
	Misses       int64 `json:"misses"`
	TotalEntries int64 `json:"total_entries"`
}

// Engine mengelola pembacaan, penulisan, dan pembersihan cache respons.
type Engine struct {
	redis  *cache.Redis
	logger *slog.Logger

	mu      sync.RWMutex
	enabled bool
	ttl     time.Duration
}

// NewEngine membuat Engine baru. Bila redis nil, caching otomatis tidak aktif.
func NewEngine(r *cache.Redis, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		redis:   r,
		logger:  logger,
		enabled: true,
		ttl:     DefaultTTL,
	}
}

// SetSettings memperbarui setelan runtime response cache.
func (e *Engine) SetSettings(enabled bool, ttl time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enabled = enabled
	if ttl > 0 {
		e.ttl = ttl
	}
}

// IsEnabled memeriksa apakah caching sedang aktif.
func (e *Engine) IsEnabled() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.enabled && e.redis != nil
}

// TTL mengembalikan durasi simpan cache saat ini.
func (e *Engine) TTL() time.Duration {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ttl
}

// ComputeKey menghitung signature SHA-256 unik dari permintaan chat completion.
func (e *Engine) ComputeKey(req *providers.ChatRequest) string {
	if req == nil {
		return ""
	}

	h := sha256.New()
	_, _ = h.Write([]byte(req.Model))
	_, _ = h.Write([]byte{0})
	if req.Temperature != nil {
		_, _ = h.Write([]byte(strconv.FormatFloat(*req.Temperature, 'f', 4, 64)))
	}
	_, _ = h.Write([]byte{0})

	if req.MaxTokens != nil {
		_, _ = h.Write([]byte(strconv.Itoa(*req.MaxTokens)))
	}
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strconv.FormatBool(req.Stream)))
	_, _ = h.Write([]byte{0})

	for i := range req.Messages {
		m := &req.Messages[i]
		_, _ = h.Write([]byte(m.Role))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(m.Text()))
		_, _ = h.Write([]byte{0})
	}

	if len(req.Tools) > 0 {
		toolsBytes, _ := json.Marshal(req.Tools)
		_, _ = h.Write(toolsBytes)
	}

	return hex.EncodeToString(h.Sum(nil))
}

// Get mencari respons yang pernah disimpan berdasarkan kunci.
func (e *Engine) Get(ctx context.Context, key string) (*Entry, bool, error) {
	if !e.IsEnabled() || key == "" {
		return nil, false, nil
	}

	rKey := cache.Key(cache.NamespaceResponseCache, "entry", key)
	val, err := e.redis.Client().Get(ctx, rKey).Bytes()
	if errors.Is(err, redis.Nil) {
		e.recordStat(ctx, "misses")
		return nil, false, nil
	}
	if err != nil {
		e.logger.WarnContext(ctx, "gagal membaca response cache", "key", key, "error", err)
		return nil, false, err
	}

	var entry Entry
	if err := json.Unmarshal(val, &entry); err != nil {
		return nil, false, err
	}

	e.recordStat(ctx, "hits")
	return &entry, true, nil
}

// Set menyimpan respons inferensi ke Redis dengan TTL aktif.
func (e *Engine) Set(ctx context.Context, key string, entry *Entry) error {
	if !e.IsEnabled() || key == "" || entry == nil {
		return nil
	}

	rKey := cache.Key(cache.NamespaceResponseCache, "entry", key)
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("serialize cache entry: %w", err)
	}

	ttl := e.TTL()
	if err := e.redis.Client().Set(ctx, rKey, data, ttl).Err(); err != nil {
		e.logger.WarnContext(ctx, "gagal menulis response cache", "key", key, "error", err)
		return err
	}
	return nil
}

// Flush membersihkan seluruh entri cache respons di Redis tanpa menyentuh namespace lain.
func (e *Engine) Flush(ctx context.Context) (int64, error) {
	if e.redis == nil {
		return 0, nil
	}

	pattern := cache.Key(cache.NamespaceResponseCache, "entry", "*")
	client := e.redis.Client()

	var deletedCount int64
	var cursor uint64

	for {
		keys, nextCursor, err := client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return deletedCount, fmt.Errorf("scan response cache: %w", err)
		}

		if len(keys) > 0 {
			n, err := client.Del(ctx, keys...).Result()
			if err != nil {
				return deletedCount, fmt.Errorf("del response cache: %w", err)
			}
			deletedCount += n
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return deletedCount, nil
}

// Stats menghitung metrik statistik penggunaan cache respons.
func (e *Engine) Stats(ctx context.Context) (Stats, error) {
	s := Stats{
		Enabled:    e.IsEnabled(),
		TTLSeconds: int64(e.TTL().Seconds()),
	}
	if e.redis == nil {
		return s, nil
	}

	client := e.redis.Client()
	hitsKey := cache.Key(cache.NamespaceResponseCache, "stats", "hits")
	missesKey := cache.Key(cache.NamespaceResponseCache, "stats", "misses")

	hits, _ := client.Get(ctx, hitsKey).Int64()
	misses, _ := client.Get(ctx, missesKey).Int64()
	s.Hits = hits
	s.Misses = misses

	pattern := cache.Key(cache.NamespaceResponseCache, "entry", "*")
	var count int64
	var cursor uint64
	for {
		keys, next, err := client.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			break
		}
		count += int64(len(keys))
		cursor = next
		if cursor == 0 {
			break
		}
	}
	s.TotalEntries = count

	return s, nil
}

func (e *Engine) recordStat(ctx context.Context, metric string) {
	if e.redis == nil {
		return
	}
	key := cache.Key(cache.NamespaceResponseCache, "stats", metric)
	_ = e.redis.Client().Incr(ctx, key).Err()
}
