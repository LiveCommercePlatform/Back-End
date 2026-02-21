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

	fn()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn()
		}
	}
}

// -------- Views: product:{pid}:views --------
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

			delta, supported, err := tryGetDelInt64(ctx, rdb, k)
			if err != nil {
				continue
			}

			if !supported {
				delta, err = rdb.Get(ctx, k).Int64()
				if err == redis.Nil || delta == 0 {
					_ = rdb.Del(ctx, k).Err()
					continue
				}
				if err != nil {
					continue
				}
				_ = rdb.Del(ctx, k).Err()
			} else {
				if delta == 0 {
					continue
				}
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

// ---------- Scripts for atomic get+reset ----------

var getResetEngageDelta = redis.NewScript(`
local k = KEYS[1]
local l = tonumber(redis.call("HGET", k, "likes_count") or "0")
local d = tonumber(redis.call("HGET", k, "dislikes_count") or "0")
redis.call("HSET", k, "likes_count", 0, "dislikes_count", 0)
return {l, d}
`)

var getResetRatingDelta = redis.NewScript(`
local k = KEYS[1]
local c = tonumber(redis.call("HGET", k, "count") or "0")
local s = tonumber(redis.call("HGET", k, "sum") or "0")
redis.call("HSET", k, "count", 0, "sum", 0)
return {c, s}
`)

// -------- Engagement meta: product:{pid}:engagement:meta --------
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

			res, err := getResetEngageDelta.Run(ctx, rdb, []string{k}).Result()
			if err != nil {
				continue
			}

			arr, ok := res.([]interface{})
			if !ok || len(arr) != 2 {
				continue
			}

			var likesDelta, dislikesDelta int64
			fmt.Sscan(fmt.Sprint(arr[0]), &likesDelta)
			fmt.Sscan(fmt.Sprint(arr[1]), &dislikesDelta)

			if likesDelta == 0 && dislikesDelta == 0 {
				continue
			}

			_ = db.Model(&models.Product{}).
				Where("id = ?", pid).
				Updates(map[string]any{
					"like_count":    gorm.Expr("like_count + ?", likesDelta),
					"dislike_count": gorm.Expr("dislike_count + ?", dislikesDelta),
				}).Error
		}

		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}

// -------- Ratings meta: product:{pid}:ratings:meta --------
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

			res, err := getResetRatingDelta.Run(ctx, rdb, []string{k}).Result()
			if err != nil {
				continue
			}

			arr, ok := res.([]interface{})
			if !ok || len(arr) != 2 {
				continue
			}

			var countDelta, sumDelta int64
			fmt.Sscan(fmt.Sprint(arr[0]), &countDelta)
			fmt.Sscan(fmt.Sprint(arr[1]), &sumDelta)

			if countDelta == 0 && sumDelta == 0 {
				continue
			}

			_ = db.Model(&models.Product{}).
				Where("id = ?", pid).
				Updates(map[string]any{
					"rating_count": gorm.Expr("rating_count + ?", countDelta),
					"rating_sum":   gorm.Expr("rating_sum + ?", sumDelta),
					"rating_avg": gorm.Expr(
						"CASE WHEN (rating_count + ?) > 0 THEN (1.0 * (rating_sum + ?) / (rating_count + ?)) ELSE 0 END",
						countDelta, sumDelta, countDelta,
					),
				}).Error
		}

		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}

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

func tryGetDelInt64(ctx context.Context, rdb *redis.Client, key string) (val int64, supported bool, err error) {
	res, err := rdb.Do(ctx, "GETDEL", key).Result()
	if err == redis.Nil {
		return 0, true, nil
	}
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "unknown command") || strings.Contains(msg, "ERR unknown command") {
			return 0, false, nil
		}
		return 0, true, err
	}

	switch v := res.(type) {
	case string:
		n, e := strconv.ParseInt(v, 10, 64)
		return n, true, e
	case []byte:
		n, e := strconv.ParseInt(string(v), 10, 64)
		return n, true, e
	default:
		return 0, true, fmt.Errorf("unexpected GETDEL type: %T", res)
	}
}