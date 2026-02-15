package syncer

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"livecommerce/internal/cache"
	"livecommerce/internal/database"
	"livecommerce/internal/models"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Config struct {
	ViewsEvery      time.Duration
	EngagementEvery time.Duration
	RatingsEvery    time.Duration
	BatchSize       int64
}

func DefaultConfig() Config {
	return Config{
		ViewsEvery:      1 * time.Minute,
		EngagementEvery: 5 * time.Minute,
		RatingsEvery:    10 * time.Minute,
		BatchSize:       200,
	}
}

func StartRedisSync(ctx context.Context, cfg Config) {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 200
	}
	go runTicker(ctx, cfg.ViewsEvery, func() { _ = SyncViewsOnce(ctx, cfg.BatchSize) })
	go runTicker(ctx, cfg.EngagementEvery, func() { _ = SyncEngagementOnce(ctx, cfg.BatchSize) })
	go runTicker(ctx, cfg.RatingsEvery, func() { _ = SyncRatingsOnce(ctx, cfg.BatchSize) })
}

func runTicker(ctx context.Context, every time.Duration, fn func()) {
	if every <= 0 {
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()

	fn() // run once at startup

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn()
		}
	}
}

// ---------------- Views ----------------
// Redis key: product:{pid}:views (string counter)
// DB column: products.view_count += delta
func SyncViewsOnce(ctx context.Context, batch int64) error {
	rdb := cache.Client
	db := database.DB

	var cursor uint64
	for {
		keys, next, err := rdb.Scan(ctx, cursor, "product:*:views", batch).Result()
		if err != nil {
			return err
		}

		for _, k := range keys {
			pid, ok := parseProductIDFromKey(k)
			if !ok {
				continue
			}

			// ✅ Redis server v8.4.0 supports GETDEL => atomic read+delete
			delta, err := getDelInt64(ctx, rdb, k)
			if err != nil || delta <= 0 {
				continue
			}

			_ = db.Model(&models.Product{}).
				Where("id = ?", pid).
				UpdateColumn("view_count", gorm.Expr("view_count + ?", delta)).Error
		}

		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}

// ---------------- Engagement meta ----------------
// Redis key: product:{pid}:engagement:meta (hash: likes_count, dislikes_count)
// DB columns: like_count, dislike_count
func SyncEngagementOnce(ctx context.Context, batch int64) error {
	rdb := cache.Client
	db := database.DB

	var cursor uint64
	for {
		keys, next, err := rdb.Scan(ctx, cursor, "product:*:engagement:meta", batch).Result()
		if err != nil {
			return err
		}

		for _, k := range keys {
			pid, ok := parseProductIDFromKey(k)
			if !ok {
				continue
			}

			meta, err := rdb.HMGet(ctx, k, "likes_count", "dislikes_count").Result()
			if err != nil {
				continue
			}

			var likes, dislikes int64
			if len(meta) >= 2 {
				if meta[0] != nil {
					fmt.Sscan(fmt.Sprint(meta[0]), &likes)
				}
				if meta[1] != nil {
					fmt.Sscan(fmt.Sprint(meta[1]), &dislikes)
				}
			}

			_ = db.Model(&models.Product{}).
				Where("id = ?", pid).
				Updates(map[string]any{
					"like_count":    likes,
					"dislike_count": dislikes,
				}).Error
		}

		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}

// ---------------- Ratings meta ----------------
// Redis key: product:{pid}:ratings:meta (hash: count, sum, avg)
// DB columns: rating_count, rating_sum, rating_avg
func SyncRatingsOnce(ctx context.Context, batch int64) error {
	rdb := cache.Client
	db := database.DB

	var cursor uint64
	for {
		keys, next, err := rdb.Scan(ctx, cursor, "product:*:ratings:meta", batch).Result()
		if err != nil {
			return err
		}

		for _, k := range keys {
			pid, ok := parseProductIDFromKey(k)
			if !ok {
				continue
			}

			meta, err := rdb.HMGet(ctx, k, "count", "sum", "avg").Result()
			if err != nil {
				continue
			}

			var count, sum int64
			var avg float64
			if len(meta) >= 3 {
				if meta[0] != nil {
					fmt.Sscan(fmt.Sprint(meta[0]), &count)
				}
				if meta[1] != nil {
					fmt.Sscan(fmt.Sprint(meta[1]), &sum)
				}
				if meta[2] != nil {
					fmt.Sscan(fmt.Sprint(meta[2]), &avg)
				}
			}

			_ = db.Model(&models.Product{}).
				Where("id = ?", pid).
				Updates(map[string]any{
					"rating_count": count,
					"rating_sum":   sum,
					"rating_avg":   avg,
				}).Error
		}

		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}

// ---------------- Helpers ----------------

// product:{pid}:...
func parseProductIDFromKey(k string) (uuid.UUID, bool) {
	if !strings.HasPrefix(k, "product:") {
		return uuid.Nil, false
	}
	rest := strings.TrimPrefix(k, "product:")
	parts := strings.Split(rest, ":")
	if len(parts) < 2 {
		return uuid.Nil, false
	}
	pid, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, false
	}
	return pid, true
}

// Atomic read+delete: GETDEL key
func getDelInt64(ctx context.Context, rdb *redis.Client, key string) (int64, error) {
	res, err := rdb.Do(ctx, "GETDEL", key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	switch v := res.(type) {
	case string:
		return strconv.ParseInt(v, 10, 64)
	case []byte:
		return strconv.ParseInt(string(v), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected GETDEL type: %T", res)
	}
}