package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/pkg/cachettl"
	"github.com/kurodakayn/mpp-backend/internal/pkg/redisdegrade"
)

const prefix = "mpp:dashboard:projects:list:v2"
const hashTag = "{dashboard:projects-list}"
const generationKey = "mpp:dashboard:projects:list-generation:v2:" + hashTag
const pattern = prefix + ":" + hashTag + ":*"
const degradedGeneration = "degraded"
const refreshTimeout = 15 * time.Second
const invalidateTimeout = 2 * time.Second

type Config struct {
	Client redis.UniversalClient
	TTL    time.Duration
	Group  *singleflight.Group
	Guard  *redisdegrade.Guard
}

type Cache struct {
	client redis.UniversalClient
	ttl    time.Duration
	group  *singleflight.Group
	guard  *redisdegrade.Guard
}

type Params struct {
	Generation   string `json:"generation"`
	Cursor       string `json:"cursor,omitempty"`
	Page         int    `json:"page"`
	Limit        int    `json:"limit"`
	Status       string `json:"status,omitempty"`
	FilterUserID string `json:"filter_user_id,omitempty"`
	Platform     string `json:"platform,omitempty"`
	ScopeUserID  string `json:"scope_user_id,omitempty"`
	WorkspaceID  string `json:"workspace_id,omitempty"`
	ActorUserID  string `json:"actor_user_id,omitempty"`
}

type Payload struct {
	Items      []dto.ProjectListItem `json:"items"`
	Cursor     string                `json:"cursor,omitempty"`
	NextCursor string                `json:"next_cursor,omitempty"`
	HasMore    bool                  `json:"has_more"`
	Page       int                   `json:"page"`
	Limit      int                   `json:"limit"`
	Total      int64                 `json:"total"`
	TotalPages int                   `json:"total_pages"`
}

type Compute func(context.Context) (*dto.PaginationResponse, error)

func New(config Config) *Cache {
	return &Cache{
		client: config.Client,
		ttl:    config.TTL,
		group:  config.Group,
		guard:  config.Guard,
	}
}

func (c *Cache) Get(ctx context.Context, params Params, compute Compute) (*dto.PaginationResponse, error) {
	if compute == nil {
		return nil, errors.New("project list cache compute is nil")
	}
	ctx = requestContext(ctx)
	if !c.ready() {
		return compute(ctx)
	}

	generation, err := c.generation(ctx)
	if err != nil {
		params.Generation = degradedGeneration
		return c.computeSingleflight(ctx, cacheKey(params), compute)
	}

	params.Generation = generation
	key := cacheKey(params)
	if resp, hit, err := c.cached(ctx, key, params.Cursor, params.Page, params.Limit); hit {
		return resp, nil
	} else if err != nil {
		return compute(ctx)
	}

	return c.refreshSingleflight(ctx, key, params, compute)
}

func (c *Cache) CanUse(ctx context.Context) bool {
	if !c.ready() {
		return false
	}
	stickyUntil, sticky := dbrouter.StickyWriterUntil(requestContext(ctx))
	return !sticky || !stickyUntil.After(time.Now())
}

func (c *Cache) Invalidate(ctx context.Context) {
	if c == nil || c.client == nil {
		return
	}
	ctx, cancel := invalidationContext(requestContext(ctx))
	defer cancel()
	_ = redisdegrade.DoWork(c.guard, "cache_invalidate", func() error {
		return c.client.Incr(ctx, generationKey).Err()
	})
	c.deleteKeys(ctx)
}

func (c *Cache) ready() bool {
	return c != nil && c.client != nil && c.ttl > 0
}

func (c *Cache) computeSingleflight(ctx context.Context, key string, compute Compute) (*dto.PaginationResponse, error) {
	if c.group == nil {
		return compute(ctx)
	}

	resultCh := c.group.DoChan(key, func() (any, error) {
		refreshCtx, cancel := refreshContext(ctx)
		defer cancel()
		return compute(refreshCtx)
	})

	select {
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		if resp, ok := result.Val.(*dto.PaginationResponse); ok {
			return resp, nil
		}
		return compute(ctx)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Cache) refreshSingleflight(ctx context.Context, key string, params Params, compute Compute) (*dto.PaginationResponse, error) {
	if c.group == nil {
		refreshCtx, cancel := refreshContext(ctx)
		defer cancel()
		return c.refresh(refreshCtx, key, compute)
	}

	resultCh := c.group.DoChan(key, func() (any, error) {
		refreshCtx, cancel := refreshContext(ctx)
		defer cancel()
		if resp, hit, err := c.cached(refreshCtx, key, params.Cursor, params.Page, params.Limit); hit {
			return resp, nil
		} else if err != nil {
			return compute(refreshCtx)
		}

		return c.refresh(refreshCtx, key, compute)
	})

	select {
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		if resp, ok := result.Val.(*dto.PaginationResponse); ok {
			return resp, nil
		}
		return compute(ctx)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Cache) cached(ctx context.Context, key string, cursor string, page, limit int) (*dto.PaginationResponse, bool, error) {
	cached, err := redisdegrade.CallWork(c.guard, "cache_read", func() ([]byte, error) {
		return c.client.Get(ctx, key).Bytes()
	})
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if resp, ok := decodePayload(cached, cursor, page, limit); ok {
		return resp, true, nil
	}
	return nil, false, nil
}

func (c *Cache) refresh(ctx context.Context, key string, compute Compute) (*dto.PaginationResponse, error) {
	resp, err := compute(ctx)
	if err != nil {
		return nil, err
	}
	payload, ok := payloadFromResponse(resp)
	if !ok {
		return resp, nil
	}
	encoded, err := json.Marshal(payload)
	if err == nil {
		_ = redisdegrade.DoWork(c.guard, "cache_write", func() error {
			return c.client.Set(ctx, key, encoded, cachettl.Jitter(c.ttl, key)).Err()
		})
	}
	return resp, nil
}

func (c *Cache) generation(ctx context.Context) (string, error) {
	generation, err := redisdegrade.CallWork(c.guard, "cache_read", func() (string, error) {
		return c.client.Get(ctx, generationKey).Result()
	})
	if errors.Is(err, redis.Nil) {
		return "0", nil
	}
	return generation, err
}

func (c *Cache) deleteKeys(ctx context.Context) {
	var cursor uint64
	for {
		type scanResult struct {
			keys []string
			next uint64
		}
		result, err := redisdegrade.CallWork(c.guard, "cache_invalidate", func() (scanResult, error) {
			keys, next, err := c.client.Scan(ctx, cursor, pattern, 100).Result()
			return scanResult{keys: keys, next: next}, err
		})
		if err != nil {
			return
		}
		if len(result.keys) > 0 {
			for _, key := range result.keys {
				_ = redisdegrade.DoWork(c.guard, "cache_invalidate", func() error {
					return c.client.Del(ctx, key).Err()
				})
			}
		}
		if result.next == 0 {
			return
		}
		cursor = result.next
	}
}

func decodePayload(cached []byte, cursor string, page, limit int) (*dto.PaginationResponse, bool) {
	var payload Payload
	if err := json.Unmarshal(cached, &payload); err != nil {
		return nil, false
	}
	if !payloadValid(payload, cursor, page, limit) {
		return nil, false
	}
	return payloadToResponse(payload), true
}

func cacheKey(params Params) string {
	encoded, err := json.Marshal(params)
	if err != nil {
		return fmt.Sprintf("%s:%d:%d", prefix, params.Page, params.Limit)
	}
	sum := sha256.Sum256(encoded)
	return prefix + ":" + hashTag + ":" + hex.EncodeToString(sum[:])
}

func UUIDStringValue(value *uuid.UUID) string {
	if value == nil || *value == uuid.Nil {
		return ""
	}
	return value.String()
}

func payloadFromResponse(resp *dto.PaginationResponse) (Payload, bool) {
	items, ok := resp.Items.([]dto.ProjectListItem)
	if !ok {
		return Payload{}, false
	}
	return Payload{
		Items:      items,
		Cursor:     resp.Cursor,
		NextCursor: resp.NextCursor,
		HasMore:    resp.HasMore,
		Page:       resp.Page,
		Limit:      resp.Limit,
		Total:      resp.Total,
		TotalPages: resp.TotalPages,
	}, true
}

func payloadValid(payload Payload, cursor string, page, limit int) bool {
	if payload.Items == nil {
		return false
	}
	if payload.Cursor != cursor {
		return false
	}
	if payload.Page != page || payload.Limit != limit {
		return false
	}
	if payload.Total < 0 || payload.TotalPages < 0 {
		return false
	}
	return true
}

func payloadToResponse(payload Payload) *dto.PaginationResponse {
	return &dto.PaginationResponse{
		Items:      payload.Items,
		Cursor:     payload.Cursor,
		NextCursor: payload.NextCursor,
		HasMore:    payload.HasMore,
		Page:       payload.Page,
		Limit:      payload.Limit,
		Total:      payload.Total,
		TotalPages: payload.TotalPages,
	}
}

func refreshContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), refreshTimeout)
}

func invalidationContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), invalidateTimeout)
}

func requestContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
