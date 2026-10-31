package models

import (
	"context"
	"os"
	"testing"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/proto"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// useTestMemcache points the query cache at the memcached server in
// GZ_FUEL_MEMCACHED_ADDR, and skips the test when there's none. The test gets
// its own generation key, so that tests of other packages using the same
// server don't clear its cache.
func useTestMemcache(t *testing.T) {
	addr := os.Getenv("GZ_FUEL_MEMCACHED_ADDR")
	if addr == "" {
		t.Skip("GZ_FUEL_MEMCACHED_ADDR isn't set")
	}
	origCache, origKey := globals.QueryCache, listCacheGenerationKey
	t.Cleanup(func() { globals.QueryCache, listCacheGenerationKey = origCache, origKey })
	globals.QueryCache = memcache.New(addr)
	listCacheGenerationKey = "test_generation_" + gz.RandomString(8)
}

// cachePage caches a one-model page of the list in the current generation.
func cachePage(t *testing.T, p *gz.PaginationRequest, name string) {
	gen, err := listCacheGeneration()
	require.NoError(t, err)
	setModelListCache(context.Background(), gen, p,
		&fuel.Models{Models: []*fuel.Model{{Name: proto.String(name)}}},
		&gz.PaginationResult{Page: p.Page, PerPage: p.PerPage, PageFound: true})
}

// cachedPage returns the name of the model in the cached page, if there is one.
func cachedPage(t *testing.T, p *gz.PaginationRequest) (string, bool) {
	gen, err := listCacheGeneration()
	require.NoError(t, err)
	list, _, ok := getModelListCache(gen, p)
	if !ok {
		return "", false
	}
	require.Len(t, list.Models, 1)
	return *list.Models[0].Name, true
}

// TestClearListCache checks that clearing the model list cache hides the
// cached pages, and leaves other keys on the memcached server alone.
func TestClearListCache(t *testing.T) {
	useTestMemcache(t)
	ctx := context.Background()

	unpaged := &gz.PaginationRequest{PerPage: 20}
	page2 := &gz.PaginationRequest{PageRequested: true, Page: 2, PerPage: 20}
	cachePage(t, unpaged, "first")
	cachePage(t, page2, "second")
	other := &memcache.Item{Key: "not_the_model_list", Value: []byte("kept")}
	require.NoError(t, globals.QueryCache.Set(other))

	name, ok := cachedPage(t, unpaged)
	assert.True(t, ok)
	assert.Equal(t, "first", name)
	name, ok = cachedPage(t, page2)
	assert.True(t, ok)
	assert.Equal(t, "second", name)

	ClearListCache(ctx)
	_, ok = cachedPage(t, unpaged)
	assert.False(t, ok)
	_, ok = cachedPage(t, page2)
	assert.False(t, ok)

	item, err := globals.QueryCache.Get(other.Key)
	require.NoError(t, err)
	assert.Equal(t, "kept", string(item.Value))

	// Pages cached after clearing are used.
	cachePage(t, unpaged, "third")
	name, ok = cachedPage(t, unpaged)
	assert.True(t, ok)
	assert.Equal(t, "third", name)
}

// TestListCacheGenerationLost checks that cached pages aren't used once the
// generation key is lost, for example when memcached evicts it.
func TestListCacheGenerationLost(t *testing.T) {
	useTestMemcache(t)

	p := &gz.PaginationRequest{PerPage: 20}
	cachePage(t, p, "first")
	before, err := listCacheGeneration()
	require.NoError(t, err)

	require.NoError(t, globals.QueryCache.Delete(listCacheGenerationKey))
	// Clearing without a generation isn't an error, and doesn't start one.
	ClearListCache(context.Background())
	_, err = globals.QueryCache.Get(listCacheGenerationKey)
	assert.ErrorIs(t, err, memcache.ErrCacheMiss)

	after, err := listCacheGeneration()
	require.NoError(t, err)
	assert.NotEqual(t, before, after)
	_, ok := cachedPage(t, p)
	assert.False(t, ok)
}
