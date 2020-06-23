package reverse_dns

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSimpleReverseDNSLookup(t *testing.T) {
	d := NewReverseDNSCache(60*time.Second, 1*time.Second, -1)
	d.Resolver = &localResolver{}
	answer := d.Lookup("127.0.0.1")
	require.Equal(t, []string{"localhost"}, answer)
	d.blockAllWorkers()

	// do another request with no workers available.
	// it should read from cache instantly.
	answer = d.Lookup("127.0.0.1")
	require.Equal(t, []string{"localhost"}, answer)

	require.Len(t, d.cache, 1)
	require.Len(t, d.expireList, 1)
	d.cleanup()
	require.Len(t, d.expireList, 1) // ttl hasn't hit yet.

	stats := d.Stats()

	require.EqualValues(t, 0, stats.CacheExpire)
	require.EqualValues(t, 1, stats.CacheMiss)
	require.EqualValues(t, 1, stats.CacheHit)
	require.EqualValues(t, 1, stats.RequestsFilled)
	require.EqualValues(t, 0, stats.RequestsAbandoned)
}

func TestParallelReverseDNSLookup(t *testing.T) {
	d := NewReverseDNSCache(1*time.Second, 1*time.Second, -1)
	d.Resolver = &localResolver{}
	var answer1 []string
	var answer2 []string
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		answer1 = d.Lookup("127.0.0.1")
		wg.Done()
	}()
	go func() {
		answer2 = d.Lookup("127.0.0.1")
		wg.Done()
	}()

	wg.Wait()

	t.Log(answer1)
	t.Log(answer2)

	require.Equal(t, []string{"localhost"}, answer1)
	require.Equal(t, []string{"localhost"}, answer2)

	require.Len(t, d.cache, 1)

	stats := d.Stats()

	require.EqualValues(t, 1, stats.CacheMiss)
	require.EqualValues(t, 1, stats.CacheHit)
}

func TestUnavailableDNSServerRespectsTimeout(t *testing.T) {
	d := NewReverseDNSCache(0, 1, -1)
	d.Resolver = &timeoutResolver{}

	result := d.Lookup("192.153.33.3")

	require.Equal(t, []string{}, result)
}

func TestCleanupHappens(t *testing.T) {
	ttl := 100 * time.Millisecond
	d := NewReverseDNSCache(ttl, 1*time.Second, -1)
	d.Resolver = &localResolver{}
	_ = d.Lookup("127.0.0.1")

	require.Len(t, d.cache, 1)

	time.Sleep(ttl) // wait for cache entry to expire.
	d.cleanup()
	require.Len(t, d.expireList, 0)

	stats := d.Stats()

	require.EqualValues(t, 1, stats.CacheExpire)
	require.EqualValues(t, 1, stats.CacheMiss)
	require.EqualValues(t, 0, stats.CacheHit)
}

func TestCachePassthrough(t *testing.T) {
	d := NewReverseDNSCache(0, 1*time.Second, -1)
	d.Resolver = &localResolver{}
	_ = d.Lookup("127.0.0.1")

	require.Len(t, d.cache, 0)

	stats := d.Stats()
	require.EqualValues(t, 1, stats.CacheMiss)
	require.EqualValues(t, 0, stats.CacheHit)
}

func TestLookupTimeout(t *testing.T) {
	d := NewReverseDNSCache(10*time.Second, 10*time.Second, -1)
	d.Resolver = &timeoutResolver{}
	d.Lookup("127.0.0.1")
	require.EqualValues(t, 1, d.Stats().RequestsAbandoned)
}

type timeoutResolver struct{}

func (r *timeoutResolver) LookupAddr(ctx context.Context, addr string) (names []string, err error) {
	return nil, errors.New("timeout")
}

type localResolver struct{}

func (r *localResolver) LookupAddr(ctx context.Context, addr string) (names []string, err error) {
	return []string{"localhost"}, nil
}
