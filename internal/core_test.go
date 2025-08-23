package internal

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func TestGet(t *testing.T) {
	const key = "read-key"
	const value = "read-value"

	var val interface{}
	var err error

	defer delete(store.m, key)

	// Read a non-thing
	val, err = Get(key) //nolint:ineffassign
	if err == nil {
		t.Error("expected an error: ", err)
	}
	if !errors.Is(err, ErrorNoSuchKey) {
		t.Error("unexpected error:", err)
	}

	store.m[key] = value

	val, err = Get(key)
	if err != nil {
		t.Error("unexpected error:", err)
	}

	if val != value {
		t.Error("val/value mismatch")
	}
}

func TestPut(t *testing.T) {
	const key = "create-key"
	const value = "create-value"

	var val interface{}
	var contains bool

	defer delete(store.m, key)

	// Sanity check
	_, contains = store.m[key]
	if contains {
		t.Error("key/value already exists")
	}

	// err should be nil
	err := Put(key, value)
	if err != nil {
		t.Error(err)
	}

	val, contains = store.m[key]
	if !contains {
		t.Error("create failed")
	}

	if val != value {
		t.Error("val/value mismatch")
	}
}

func TestDelete(t *testing.T) {
	const key = "delete-key"
	const value = "delete-value"

	var contains bool

	defer delete(store.m, key)

	store.m[key] = value

	_, contains = store.m[key]
	if !contains {
		t.Error("key/value doesn't exist")
	}

	if err := Delete(key); err != nil {
		t.Error("Delete returns an error: ", err)
	}

	_, contains = store.m[key]
	if contains {
		t.Error("Delete failed")
	}
}

func TestPutAndGet(t *testing.T) {
	// Clear the store before testing
	store.Lock()
	store.m = make(map[string]string)
	store.Unlock()

	tests := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		{
			name:    "basic put and get",
			key:     "test-key",
			value:   "test-value",
			wantErr: false,
		},
		{
			name:    "empty value",
			key:     "empty",
			value:   "",
			wantErr: false,
		},
		{
			name:    "special characters",
			key:     "special!@#$",
			value:   "value!@#$",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Put
			if err := Put(tt.key, tt.value); (err != nil) != tt.wantErr {
				t.Errorf("Put() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Test Get
			got, err := Get(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.value {
				t.Errorf("Get() got = %v, want %v", got, tt.value)
			}
		})
	}
}

func TestGetNonExistentKey(t *testing.T) {
	store.Lock()
	store.m = make(map[string]string)
	store.Unlock()

	_, err := Get("non-existent-key")
	if err != ErrorNoSuchKey {
		t.Errorf("Get() error = %v, want %v", err, ErrorNoSuchKey)
	}
}

func BenchmarkGet(b *testing.B) {
	const key = "read-key"
	const value = "read-value"
	store.m[key] = value
	var err error

	for i := 0; i < b.N; i++ {
		if _, err = Get(key); err != nil {
			b.Error("Get returns an error: ", err)
		}
	}
}

func BenchmarkGet_BigInputs(b *testing.B) {
	keys := []string{"", "bar", "eye", "foo"}
	values := []string{"empty", "beer", "glasses", "bar"}
	var err error

	for i, key := range keys {
		store.m[key] = values[i]
	}

	for i := 0; i < b.N; i++ {
		for _, key := range keys {
			if _, err = Get(key); err != nil {
				b.Error("Get returns an error: ", err)
			}
		}
	}
}

func FuzzGet(f *testing.F) {
	var val string
	var err error
	f.Add("kayak")
	f.Fuzz(func(t *testing.T, str string) {
		if err = Put("fuzz", str); err != nil {
			t.Error("Get returns an error: ", err)
		}
		val, err = Get("fuzz")
		if err != nil {
			t.Error("Get returns an error: ", err)
		}
		if val != str {
			t.Fail()
		}
	})
}

// TestConcurrentOperations tests concurrent access to the store
func TestConcurrentOperations(t *testing.T) {
	// Clear store before test
	store.Lock()
	store.m = make(map[string]string)
	store.Unlock()

	const (
		numGoroutines = 100
		numOperations = 10
	)

	var wg sync.WaitGroup
	errorChan := make(chan error, numGoroutines*numOperations)

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key_%d_%d", id, j)
				value := fmt.Sprintf("value_%d_%d", id, j)
				if err := Put(key, value); err != nil {
					errorChan <- fmt.Errorf("Put failed: %w", err)
				}
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key_%d_%d", id, j)
				// Wait a bit for writes to happen
				time.Sleep(time.Microsecond)
				value, err := Get(key)
				if err != nil && err != ErrorNoSuchKey {
					errorChan <- fmt.Errorf("Get failed: %w", err)
				} else if err == nil {
					expectedValue := fmt.Sprintf("value_%d_%d", id, j)
					if value != expectedValue {
						errorChan <- fmt.Errorf("Value mismatch for key %s: expected %s, got %s", key, expectedValue, value)
					}
				}
			}
		}(i)
	}

	wg.Wait()
	close(errorChan)

	// Check for errors
	for err := range errorChan {
		t.Error(err)
	}
}

// TestLargeDataHandling tests behavior with large keys and values
func TestLargeDataHandling(t *testing.T) {
	// Clear store before test
	store.Lock()
	store.m = make(map[string]string)
	store.Unlock()

	tests := []struct {
		name      string
		keySize   int
		valueSize int
	}{
		{"large key", 10000, 100},
		{"large value", 100, 1000000},
		{"both large", 5000, 500000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := strings.Repeat("k", tt.keySize)
			value := strings.Repeat("v", tt.valueSize)

			// Test Put
			if err := Put(key, value); err != nil {
				t.Errorf("Put failed: %v", err)
			}

			// Test Get
			gotValue, err := Get(key)
			if err != nil {
				t.Errorf("Get failed: %v", err)
			}
			if gotValue != value {
				t.Errorf("Value mismatch: expected length %d, got length %d", len(value), len(gotValue))
			}

			// Test Delete
			if err := Delete(key); err != nil {
				t.Errorf("Delete failed: %v", err)
			}

			// Verify deletion
			_, err = Get(key)
			if err != ErrorNoSuchKey {
				t.Errorf("Expected ErrorNoSuchKey after delete, got %v", err)
			}
		})
	}
}

// TestSpecialCharacters tests UTF-8 and special character support
func TestSpecialCharacters(t *testing.T) {
	// Clear store before test
	store.Lock()
	store.m = make(map[string]string)
	store.Unlock()

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"UTF-8 Chinese", "中文键", "中文值"},
		{"UTF-8 Japanese", "日本語キー", "日本語の値"},
		{"UTF-8 Emoji", "🔑key", "💎value"},
		{"Special chars", "key!@#$%^&*()_+", "value[]{}|;':,.<>?"},
		{"Newlines", "key\nwith\nnewlines", "value\nwith\nmultiple\nlines"},
		{"Tabs and spaces", "key\t with\tspaces", "value\t with\ttabs"},
		{"Unicode points", "key\u0001\u001F\u007F", "value\u0080\u009F\u00FF"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate UTF-8
			if !utf8.ValidString(tt.key) {
				t.Skipf("Invalid UTF-8 key: %q", tt.key)
			}
			if !utf8.ValidString(tt.value) {
				t.Skipf("Invalid UTF-8 value: %q", tt.value)
			}

			// Test Put
			if err := Put(tt.key, tt.value); err != nil {
				t.Errorf("Put failed: %v", err)
			}

			// Test Get
			gotValue, err := Get(tt.key)
			if err != nil {
				t.Errorf("Get failed: %v", err)
			}
			if gotValue != tt.value {
				t.Errorf("Value mismatch: expected %q, got %q", tt.value, gotValue)
			}

			// Clean up
			if err := Delete(tt.key); err != nil {
				t.Errorf("Delete failed: %v", err)
			}
		})
	}
}

// TestMemoryPressure tests behavior under memory constraints
func TestMemoryPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory pressure test in short mode")
	}

	// Clear store before test
	store.Lock()
	store.m = make(map[string]string)
	store.Unlock()

	// Get initial memory stats
	var m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// Fill store with data
	const numEntries = 100000
	for i := 0; i < numEntries; i++ {
		key := fmt.Sprintf("memory_test_key_%d", i)
		value := fmt.Sprintf("memory_test_value_%d_with_some_additional_data", i)
		if err := Put(key, value); err != nil {
			t.Errorf("Put failed at entry %d: %v", i, err)
		}
	}

	// Get memory stats after filling
	var m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m2)

	// Verify all data is still accessible
	for i := 0; i < 1000; i++ { // Check a sample
		key := fmt.Sprintf("memory_test_key_%d", i)
		expectedValue := fmt.Sprintf("memory_test_value_%d_with_some_additional_data", i)
		gotValue, err := Get(key)
		if err != nil {
			t.Errorf("Get failed for entry %d: %v", i, err)
		}
		if gotValue != expectedValue {
			t.Errorf("Value mismatch for entry %d", i)
		}
	}

	// Clean up half the entries
	for i := 0; i < numEntries/2; i++ {
		key := fmt.Sprintf("memory_test_key_%d", i)
		if err := Delete(key); err != nil {
			t.Errorf("Delete failed for entry %d: %v", i, err)
		}
	}

	// Get memory stats after cleanup
	var m3 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m3)

	t.Logf("Memory usage: initial=%d, after_fill=%d, after_cleanup=%d",
		m1.Alloc, m2.Alloc, m3.Alloc)

	// Verify remaining data is still accessible
	for i := numEntries / 2; i < numEntries/2+100; i++ {
		key := fmt.Sprintf("memory_test_key_%d", i)
		expectedValue := fmt.Sprintf("memory_test_value_%d_with_some_additional_data", i)
		gotValue, err := Get(key)
		if err != nil {
			t.Errorf("Get failed for remaining entry %d: %v", i, err)
		}
		if gotValue != expectedValue {
			t.Errorf("Value mismatch for remaining entry %d", i)
		}
	}
}

// TestErrorConditions tests comprehensive error scenarios
func TestErrorConditions(t *testing.T) {
	// Clear store before test
	store.Lock()
	store.m = make(map[string]string)
	store.Unlock()

	tests := []struct {
		name          string
		key           string
		expectedError error
		setupFunc     func()
	}{
		{
			name:          "get non-existent key",
			key:           "non-existent",
			expectedError: ErrorNoSuchKey,
			setupFunc:     func() {},
		},
		{
			name:          "get after delete",
			key:           "deleted-key",
			expectedError: ErrorNoSuchKey,
			setupFunc: func() {
				Put("deleted-key", "some-value")
				Delete("deleted-key")
			},
		},
		{
			name:          "empty key",
			key:           "",
			expectedError: ErrorNoSuchKey,
			setupFunc:     func() {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupFunc()

			_, err := Get(tt.key)
			if !errors.Is(err, tt.expectedError) {
				t.Errorf("Expected error %v, got %v", tt.expectedError, err)
			}
		})
	}
}

// TestThreadSafety validates thread safety under concurrent load
func TestThreadSafety(t *testing.T) {
	// Clear store before test
	store.Lock()
	store.m = make(map[string]string)
	store.Unlock()

	const (
		numReaders   = 50
		numWriters   = 10
		numDeletes   = 5
		testDuration = 100 * time.Millisecond
	)

	var wg sync.WaitGroup
	done := make(chan struct{})
	errorChan := make(chan error, 1000)

	// Initialize some data
	for i := 0; i < 100; i++ {
		Put(fmt.Sprintf("initial_%d", i), fmt.Sprintf("value_%d", i))
	}

	// Start readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			readerCount := 0
			for {
				select {
				case <-done:
					t.Logf("Reader %d performed %d reads", id, readerCount)
					return
				default:
					key := fmt.Sprintf("initial_%d", id%100)
					_, err := Get(key)
					if err != nil && err != ErrorNoSuchKey {
						select {
						case errorChan <- fmt.Errorf("Reader %d error: %w", id, err):
						default:
						}
						return
					}
					readerCount++
					time.Sleep(time.Microsecond)
				}
			}
		}(i)
	}

	// Start writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			writerCount := 0
			for {
				select {
				case <-done:
					t.Logf("Writer %d performed %d writes", id, writerCount)
					return
				default:
					key := fmt.Sprintf("writer_%d_%d", id, writerCount)
					value := fmt.Sprintf("value_%d_%d", id, writerCount)
					if err := Put(key, value); err != nil {
						select {
						case errorChan <- fmt.Errorf("Writer %d error: %w", id, err):
						default:
						}
						return
					}
					writerCount++
					time.Sleep(time.Microsecond * 10)
				}
			}
		}(i)
	}

	// Start deleters
	for i := 0; i < numDeletes; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			deleteCount := 0
			for {
				select {
				case <-done:
					t.Logf("Deleter %d performed %d deletes", id, deleteCount)
					return
				default:
					key := fmt.Sprintf("initial_%d", (id*10+deleteCount)%100)
					if err := Delete(key); err != nil {
						select {
						case errorChan <- fmt.Errorf("Deleter %d error: %w", id, err):
						default:
						}
						return
					}
					deleteCount++
					time.Sleep(time.Microsecond * 50)
				}
			}
		}(i)
	}

	// Run for specified duration
	time.Sleep(testDuration)
	close(done)
	wg.Wait()
	close(errorChan)

	// Check for errors
	for err := range errorChan {
		t.Error(err)
	}
}

// BenchmarkConcurrentOperations benchmarks concurrent access patterns
func BenchmarkConcurrentOperations(b *testing.B) {
	// Clear store before benchmark
	store.Lock()
	store.m = make(map[string]string)
	store.Unlock()

	// Pre-populate with some data
	for i := 0; i < 1000; i++ {
		Put(fmt.Sprintf("bench_key_%d", i), fmt.Sprintf("bench_value_%d", i))
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := fmt.Sprintf("bench_key_%d", b.N%1000)
			Get(key)
		}
	})
}

// BenchmarkLargePayloads benchmarks large data handling performance
func BenchmarkLargePayloads(b *testing.B) {
	sizes := []int{1024, 10240, 102400, 1024000} // 1KB, 10KB, 100KB, 1MB

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			// Clear store before benchmark
			store.Lock()
			store.m = make(map[string]string)
			store.Unlock()

			value := strings.Repeat("x", size)
			key := "large_payload_key"

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				Put(key, value)
				_, err := Get(key)
				if err != nil {
					b.Error(err)
				}
			}
		})
	}
}

// BenchmarkHighFrequency benchmarks rapid operation performance
func BenchmarkHighFrequency(b *testing.B) {
	// Clear store before benchmark
	store.Lock()
	store.m = make(map[string]string)
	store.Unlock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("freq_key_%d", i%100)
		value := fmt.Sprintf("freq_value_%d", i)
		Put(key, value)
		Get(key)
		if i%10 == 0 {
			Delete(key)
		}
	}
}
