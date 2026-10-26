package models

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/proto"
	"github.com/gazebo-web/gz-go/v7"
	"google.golang.org/protobuf/proto"
)

// The model list that anonymous users get is cached in memcache (see
// isbasicModelListQuery). The cache keys include a generation number, which
// is stored under listCacheGenerationKey. ClearListCache moves to a new
// generation: the old entries are never read again, and memcached evicts
// them. Unlike DeleteAll, this leaves the rest of the memcached server alone.
// It's a variable so that tests can use their own generation.
var listCacheGenerationKey = "models_list_generation"

// listCacheExpiration is how long, in seconds, a cached list is kept. It
// bounds how long a stale list can be served. A list can be cached stale when
// it's read while a model is being changed, because the change clears the
// cache before its database transaction commits.
const listCacheExpiration = int32(10 * time.Minute / time.Second)

// listCacheGeneration returns the current generation of the model list cache.
func listCacheGeneration() (string, error) {
	item, err := globals.QueryCache.Get(listCacheGenerationKey)
	if errors.Is(err, memcache.ErrCacheMiss) {
		return newListCacheGeneration()
	}
	if err != nil {
		return "", err
	}
	return string(item.Value), nil
}

// newListCacheGeneration starts a generation when there's none, for example
// after memcached restarted or evicted the generation key. The number comes
// from the clock, so it doesn't reuse a generation whose entries could still
// be cached.
func newListCacheGeneration() (string, error) {
	gen := strconv.FormatInt(time.Now().UnixNano(), 10)
	err := globals.QueryCache.Add(&memcache.Item{Key: listCacheGenerationKey, Value: []byte(gen)})
	if errors.Is(err, memcache.ErrNotStored) {
		// Another request started a generation first.
		item, err := globals.QueryCache.Get(listCacheGenerationKey)
		if err != nil {
			return "", err
		}
		return string(item.Value), nil
	}
	if err != nil {
		return "", err
	}
	return gen, nil
}

// ClearListCache stops the cached model lists from being used. Call it after
// any change that shows in a public model's entry in the list.
func ClearListCache(ctx context.Context) {
	_, err := globals.QueryCache.Increment(listCacheGenerationKey, 1)
	// Without a generation, the next read starts a new one, which can't
	// match any cached entry.
	if err != nil && !errors.Is(err, memcache.ErrCacheMiss) {
		gz.LoggerFromContext(ctx).Error("Failed to clear the model list cache.", err)
	}
}

// listCacheKeys returns the keys of a page of the model list in a generation.
func listCacheKeys(gen string, p *gz.PaginationRequest) (modelsKey, paginationKey string) {
	suffix := gen
	if p.PageRequested {
		suffix += "_" + strconv.FormatInt(p.Page, 10)
	}
	return "models_list_models_" + suffix, "models_list_pagination_" + suffix
}

// getModelListCache returns a cached page of the model list, if there is one.
func getModelListCache(gen string, p *gz.PaginationRequest) (*fuel.Models, *gz.PaginationResult, bool) {
	modelsKey, paginationKey := listCacheKeys(gen, p)
	items, err := globals.QueryCache.GetMulti([]string{modelsKey, paginationKey})
	if err != nil || items[modelsKey] == nil || items[paginationKey] == nil {
		return nil, nil, false
	}

	var modelsResult fuel.Models
	var paginationResult gz.PaginationResult
	if err := proto.Unmarshal(items[modelsKey].Value, &modelsResult); err != nil {
		return nil, nil, false
	}
	if err := json.Unmarshal(items[paginationKey].Value, &paginationResult); err != nil {
		return nil, nil, false
	}
	return &modelsResult, &paginationResult, true
}

// setModelListCache caches a page of the model list.
func setModelListCache(ctx context.Context, gen string, p *gz.PaginationRequest,
	modelsResult *fuel.Models, paginationResult *gz.PaginationResult) {

	modelsBytes, err := proto.Marshal(modelsResult)
	if err != nil {
		gz.LoggerFromContext(ctx).Error("Error marshalling models result", err)
		return
	}
	paginationBytes, err := json.Marshal(paginationResult)
	if err != nil {
		gz.LoggerFromContext(ctx).Error("Error marshalling pagination result", err)
		return
	}

	modelsKey, paginationKey := listCacheKeys(gen, p)
	if err := globals.QueryCache.Set(&memcache.Item{Key: modelsKey, Value: modelsBytes,
		Expiration: listCacheExpiration}); err != nil {
		gz.LoggerFromContext(ctx).Error("Error caching model list result", err)
	}
	if err := globals.QueryCache.Set(&memcache.Item{Key: paginationKey, Value: paginationBytes,
		Expiration: listCacheExpiration}); err != nil {
		gz.LoggerFromContext(ctx).Error("Error caching model pagination result", err)
	}
}
