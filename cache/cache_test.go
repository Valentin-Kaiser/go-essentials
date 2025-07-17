package cache

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestUser represents a test user struct for testing serialization
type TestUser struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func TestMemoryCache_BasicOperations(t *testing.T) {
	cache := NewMemoryCache().
		WithMaxSize(100).
		WithDefaultTTL(time.Hour)

	defer cache.Close()

	ctx := context.Background()

	// Test Set and Get
	user := TestUser{ID: 1, Name: "John Doe", Email: "john@example.com"}
	err := cache.Set(ctx, "user:1", user, time.Minute)
	if err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	var retrievedUser TestUser
	found, err := cache.Get(ctx, "user:1", &retrievedUser)
	if err != nil {
		t.Fatalf("Failed to get value: %v", err)
	}

	if !found {
		t.Fatal("Expected to find value")
	}

	if retrievedUser.ID != user.ID || retrievedUser.Name != user.Name || retrievedUser.Email != user.Email {
		t.Errorf("Retrieved user doesn't match original: %+v != %+v", retrievedUser, user)
	}

	// Test Exists
	exists, err := cache.Exists(ctx, "user:1")
	if err != nil {
		t.Fatalf("Failed to check exists: %v", err)
	}
	if !exists {
		t.Error("Expected key to exist")
	}

	// Test Delete
	err = cache.Delete(ctx, "user:1")
	if err != nil {
		t.Fatalf("Failed to delete value: %v", err)
	}

	found, err = cache.Get(ctx, "user:1", &retrievedUser)
	if err != nil {
		t.Fatalf("Failed to get value after delete: %v", err)
	}
	if found {
		t.Error("Expected value to be deleted")
	}
}

func TestMemoryCache_TTL(t *testing.T) {
	cache := NewMemoryCache().
		WithDefaultTTL(time.Millisecond * 100)

	defer cache.Close()

	ctx := context.Background()

	// Set value with short TTL
	err := cache.Set(ctx, "test", "value", time.Millisecond*50)
	if err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	// Should exist immediately
	exists, err := cache.Exists(ctx, "test")
	if err != nil {
		t.Fatalf("Failed to check exists: %v", err)
	}
	if !exists {
		t.Error("Expected key to exist")
	}

	// Wait for expiration
	time.Sleep(time.Millisecond * 60)

	var value string
	found, err := cache.Get(ctx, "test", &value)
	if err != nil {
		t.Fatalf("Failed to get expired value: %v", err)
	}
	if found {
		t.Error("Expected value to be expired")
	}
}

func TestMemoryCache_LRUEviction(t *testing.T) {
	cache := NewMemoryCache().
		WithMaxSize(3).
		WithLRUEviction(true)

	defer cache.Close()

	ctx := context.Background()

	// Fill cache to capacity
	for i := 1; i <= 3; i++ {
		err := cache.Set(ctx, fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i), time.Hour)
		if err != nil {
			t.Fatalf("Failed to set value %d: %v", i, err)
		}
	}

	// Access key1 to make it recently used
	var value string
	_, err := cache.Get(ctx, "key1", &value)
	if err != nil {
		t.Fatalf("Failed to get key1: %v", err)
	}

	// Add one more item, should evict key2 (least recently used)
	err = cache.Set(ctx, "key4", "value4", time.Hour)
	if err != nil {
		t.Fatalf("Failed to set key4: %v", err)
	}

	// key2 should be evicted
	exists, err := cache.Exists(ctx, "key2")
	if err != nil {
		t.Fatalf("Failed to check exists for key2: %v", err)
	}
	if exists {
		t.Error("Expected key2 to be evicted")
	}

	// key1, key3, key4 should still exist
	for _, key := range []string{"key1", "key3", "key4"} {
		exists, err := cache.Exists(ctx, key)
		if err != nil {
			t.Fatalf("Failed to check exists for %s: %v", key, err)
		}
		if !exists {
			t.Errorf("Expected %s to exist", key)
		}
	}
}

func TestMemoryCache_MultiOperations(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()

	// Test SetMulti
	items := map[string]interface{}{
		"user:1": TestUser{ID: 1, Name: "Alice", Email: "alice@example.com"},
		"user:2": TestUser{ID: 2, Name: "Bob", Email: "bob@example.com"},
		"user:3": TestUser{ID: 3, Name: "Charlie", Email: "charlie@example.com"},
	}

	err := cache.SetMulti(ctx, items, time.Hour)
	if err != nil {
		t.Fatalf("Failed to set multiple items: %v", err)
	}

	// Test GetMulti
	keys := []string{"user:1", "user:2", "user:3", "user:4"} // user:4 doesn't exist
	results, err := cache.GetMulti(ctx, keys)
	if err != nil {
		t.Fatalf("Failed to get multiple items: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	// Test DeleteMulti
	deleteKeys := []string{"user:1", "user:2"}
	err = cache.DeleteMulti(ctx, deleteKeys)
	if err != nil {
		t.Fatalf("Failed to delete multiple items: %v", err)
	}

	// Verify deletions
	for _, key := range deleteKeys {
		exists, err := cache.Exists(ctx, key)
		if err != nil {
			t.Fatalf("Failed to check exists for %s: %v", key, err)
		}
		if exists {
			t.Errorf("Expected %s to be deleted", key)
		}
	}

	// user:3 should still exist
	exists, err := cache.Exists(ctx, "user:3")
	if err != nil {
		t.Fatalf("Failed to check exists for user:3: %v", err)
	}
	if !exists {
		t.Error("Expected user:3 to still exist")
	}
}

func TestMemoryCache_Stats(t *testing.T) {
	cache := NewMemoryCache().
		WithMaxSize(10)

	defer cache.Close()

	ctx := context.Background()

	// Initial stats
	stats := cache.GetStats()
	if stats.Hits != 0 || stats.Misses != 0 || stats.Sets != 0 {
		t.Errorf("Expected initial stats to be zero: %+v", stats)
	}

	// Set some values
	for i := 1; i <= 3; i++ {
		err := cache.Set(ctx, fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i), time.Hour)
		if err != nil {
			t.Fatalf("Failed to set key%d: %v", i, err)
		}
	}

	// Get existing value (hit)
	var value string
	found, err := cache.Get(ctx, "key1", &value)
	if err != nil {
		t.Fatalf("Failed to get key1: %v", err)
	}
	if !found {
		t.Error("Expected to find key1")
	}

	// Get non-existing value (miss)
	found, err = cache.Get(ctx, "nonexistent", &value)
	if err != nil {
		t.Fatalf("Failed to get nonexistent key: %v", err)
	}
	if found {
		t.Error("Expected not to find nonexistent key")
	}

	// Check stats
	stats = cache.GetStats()
	if stats.Sets != 3 {
		t.Errorf("Expected 3 sets, got %d", stats.Sets)
	}
	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
	if stats.Size != 3 {
		t.Errorf("Expected size 3, got %d", stats.Size)
	}

	expectedHitRatio := float64(1) / float64(2) // 1 hit out of 2 total operations
	if stats.HitRatio != expectedHitRatio {
		t.Errorf("Expected hit ratio %f, got %f", expectedHitRatio, stats.HitRatio)
	}
}

func TestMemoryCache_Events(t *testing.T) {
	eventsChan := make(chan Event, 10)

	cache := NewMemoryCache().
		WithEventHandler(func(event Event) {
			eventsChan <- event
		})

	defer cache.Close()

	ctx := context.Background()

	// Set value
	err := cache.Set(ctx, "test", "value", time.Hour)
	if err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	// Get value
	var value string
	_, err = cache.Get(ctx, "test", &value)
	if err != nil {
		t.Fatalf("Failed to get value: %v", err)
	}

	// Delete value
	err = cache.Delete(ctx, "test")
	if err != nil {
		t.Fatalf("Failed to delete value: %v", err)
	}

	// Check events (with timeout)
	timeout := time.After(time.Second)
	var events []Event

	for len(events) < 3 {
		select {
		case event := <-eventsChan:
			events = append(events, event)
		case <-timeout:
			t.Fatalf("Timeout waiting for events, got %d events", len(events))
		}
	}

	// Verify we got the expected events (order may vary due to goroutines)
	eventTypes := make(map[EventType]bool)
	for _, event := range events {
		if event.Key == "test" {
			eventTypes[event.Type] = true
		}
	}

	expectedTypes := []EventType{EventSet, EventGet, EventDelete}
	for _, expectedType := range expectedTypes {
		if !eventTypes[expectedType] {
			t.Errorf("Expected event type %s not found", expectedType)
		}
	}
}

func TestMemoryCache_Namespace(t *testing.T) {
	config := DefaultConfig()
	config.Namespace = "test"

	cache := NewMemoryCacheWithConfig(config)
	defer cache.Close()

	ctx := context.Background()

	err := cache.Set(ctx, "key", "value", time.Hour)
	if err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	// Check that the key is stored with namespace prefix
	keys := cache.GetKeys()
	if len(keys) != 1 {
		t.Fatalf("Expected 1 key, got %d", len(keys))
	}

	expectedKey := "test:key"
	if keys[0] != expectedKey {
		t.Errorf("Expected key to be '%s', got '%s'", expectedKey, keys[0])
	}

	// But we should be able to get it with the original key
	var value string
	found, err := cache.Get(ctx, "key", &value)
	if err != nil {
		t.Fatalf("Failed to get value: %v", err)
	}
	if !found {
		t.Error("Expected to find value")
	}
	if value != "value" {
		t.Errorf("Expected value to be 'value', got '%s'", value)
	}
}

func TestMemoryCache_Clear(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()

	// Set multiple values
	for i := 1; i <= 5; i++ {
		err := cache.Set(ctx, fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i), time.Hour)
		if err != nil {
			t.Fatalf("Failed to set key%d: %v", i, err)
		}
	}

	// Verify they exist
	if cache.GetSize() != 5 {
		t.Errorf("Expected size 5, got %d", cache.GetSize())
	}

	// Clear cache
	err := cache.Clear(ctx)
	if err != nil {
		t.Fatalf("Failed to clear cache: %v", err)
	}

	// Verify cache is empty
	if cache.GetSize() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", cache.GetSize())
	}

	// Verify no keys exist
	for i := 1; i <= 5; i++ {
		exists, err := cache.Exists(ctx, fmt.Sprintf("key%d", i))
		if err != nil {
			t.Fatalf("Failed to check exists for key%d: %v", i, err)
		}
		if exists {
			t.Errorf("Expected key%d to not exist after clear", i)
		}
	}
}

func TestMemoryCache_TTLOperations(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx := context.Background()

	// Set value with TTL
	err := cache.Set(ctx, "test", "value", time.Hour)
	if err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	// Get TTL
	ttl, err := cache.GetTTL(ctx, "test")
	if err != nil {
		t.Fatalf("Failed to get TTL: %v", err)
	}

	if ttl <= 0 || ttl > time.Hour {
		t.Errorf("Expected TTL to be between 0 and 1 hour, got %v", ttl)
	}

	// Update TTL
	err = cache.SetTTL(ctx, "test", time.Minute)
	if err != nil {
		t.Fatalf("Failed to set TTL: %v", err)
	}

	// Verify updated TTL
	newTTL, err := cache.GetTTL(ctx, "test")
	if err != nil {
		t.Fatalf("Failed to get updated TTL: %v", err)
	}

	if newTTL <= 0 || newTTL > time.Minute {
		t.Errorf("Expected updated TTL to be between 0 and 1 minute, got %v", newTTL)
	}

	// Test TTL for non-existent key
	_, err = cache.GetTTL(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent key TTL")
	}
}

func TestJSONSerializer(t *testing.T) {
	serializer := &JSONSerializer{}

	user := TestUser{ID: 1, Name: "John", Email: "john@example.com"}

	// Test serialization
	data, err := serializer.Serialize(user)
	if err != nil {
		t.Fatalf("Failed to serialize: %v", err)
	}

	// Test deserialization
	var deserializedUser TestUser
	err = serializer.Deserialize(data, &deserializedUser)
	if err != nil {
		t.Fatalf("Failed to deserialize: %v", err)
	}

	if deserializedUser.ID != user.ID || deserializedUser.Name != user.Name || deserializedUser.Email != user.Email {
		t.Errorf("Deserialized user doesn't match original: %+v != %+v", deserializedUser, user)
	}
}

func TestNoOpSerializer(t *testing.T) {
	serializer := &NoOpSerializer{}

	// Test with []byte
	originalData := []byte("test data")

	data, err := serializer.Serialize(originalData)
	if err != nil {
		t.Fatalf("Failed to serialize []byte: %v", err)
	}

	var result []byte
	err = serializer.Deserialize(data, &result)
	if err != nil {
		t.Fatalf("Failed to deserialize to []byte: %v", err)
	}

	if string(result) != string(originalData) {
		t.Errorf("Expected %s, got %s", string(originalData), string(result))
	}

	// Test with string
	originalString := "test string"

	data, err = serializer.Serialize(originalString)
	if err != nil {
		t.Fatalf("Failed to serialize string: %v", err)
	}

	var resultString string
	err = serializer.Deserialize(data, &resultString)
	if err != nil {
		t.Fatalf("Failed to deserialize to string: %v", err)
	}

	if resultString != originalString {
		t.Errorf("Expected %s, got %s", originalString, resultString)
	}

	// Test with unsupported type
	_, err = serializer.Serialize(123)
	if err == nil {
		t.Error("Expected error for unsupported type")
	}
}
