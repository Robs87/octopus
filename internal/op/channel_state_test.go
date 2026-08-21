package op

import (
	"math"
	"sync"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

func seedChannelKeyCacheForTest(t *testing.T, channel model.Channel) {
	t.Helper()
	channelCache.Clear()
	channelKeyCache.Clear()
	channelKeyCacheNeedUpdateLock.Lock()
	channelKeyCacheNeedUpdate = make(map[int]struct{})
	channelKeyCacheNeedUpdateLock.Unlock()
	channelCache.Set(channel.ID, channel)
	for _, key := range channel.Keys {
		channelKeyCache.Set(key.ID, key)
	}
	t.Cleanup(func() {
		channelCache.Clear()
		channelKeyCache.Clear()
		channelKeyCacheNeedUpdateLock.Lock()
		channelKeyCacheNeedUpdate = make(map[int]struct{})
		channelKeyCacheNeedUpdateLock.Unlock()
	})
}

func TestChannelKeyUpdatePreservesConcurrentDifferentKeys(t *testing.T) {
	channel := model.Channel{
		ID: 1,
		Keys: []model.ChannelKey{
			{ID: 11, ChannelID: 1, Enabled: true, ChannelKey: "key-11"},
			{ID: 12, ChannelID: 1, Enabled: true, ChannelKey: "key-12"},
		},
	}
	seedChannelKeyCacheForTest(t, channel)

	var wg sync.WaitGroup
	for _, key := range channel.Keys {
		key := key
		wg.Add(1)
		go func() {
			defer wg.Done()
			key.StatusCode = 200 + key.ID
			if err := ChannelKeyUpdate(key); err != nil {
				t.Errorf("ChannelKeyUpdate() error = %v", err)
			}
		}()
	}
	wg.Wait()

	got, ok := channelCache.Get(channel.ID)
	if !ok {
		t.Fatal("channel missing from cache")
	}
	for _, want := range channel.Keys {
		var found model.ChannelKey
		for _, key := range got.Keys {
			if key.ID == want.ID {
				found = key
				break
			}
		}
		if found.StatusCode != 200+want.ID {
			t.Fatalf("key %d status = %d, want %d; snapshot=%+v", want.ID, found.StatusCode, 200+want.ID, got.Keys)
		}
	}
}

func TestChannelKeyRecordUsageAccumulatesConcurrentCosts(t *testing.T) {
	channel := model.Channel{
		ID:   2,
		Keys: []model.ChannelKey{{ID: 21, ChannelID: 2, Enabled: true, ChannelKey: "key-21"}},
	}
	seedChannelKeyCacheForTest(t, channel)

	const calls = 100
	const cost = 0.125
	var wg sync.WaitGroup
	for i := 0; i < calls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := ChannelKeyRecordUsage(model.ChannelKey{
				ID:         21,
				ChannelID:  2,
				StatusCode: 200,
			}, cost); err != nil {
				t.Errorf("ChannelKeyRecordUsage() error = %v", err)
			}
		}()
	}
	wg.Wait()

	got, ok := channelKeyCache.Get(21)
	if !ok {
		t.Fatal("key missing from cache")
	}
	want := float64(calls) * cost
	if math.Abs(got.TotalCost-want) > 1e-9 {
		t.Fatalf("TotalCost = %.12f, want %.12f", got.TotalCost, want)
	}
}
