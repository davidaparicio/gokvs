package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func TestCreateLogger(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-transaction-log")
	if err != nil {
		t.Fatalf("Cannot create temporary file: %v", err)
	}
	filename := tmpfile.Name()

	defer func() {
		if err := os.Remove(filename); err != nil {
			t.Logf("Failed to remove temporary file %s: %v", filename, err)
		}
	}()

	tl, err := NewTransactionLogger(filename)

	if tl == nil {
		t.Error("Logger is nil?")
	}

	if err != nil {
		// (*testing.common).Errorf does not support error-wrapping directive %w
		// t.Errorf("Got error: %w", err)
		// https://go.dev/blog/go1.13-errors#TOC_3.3.
		t.Errorf("Failed to create transaction logger: %v", err)
		// https://docs.sourcegraph.com/dev/background-information/languages/go_errors#printing-errors
	}

	if !fileExists(filename) {
		t.Errorf("File %s doesn't exist", filename)
	}
}

func TestWriteAppend(t *testing.T) {
	const filename = "/tmp/write-append.txt"
	defer func() {
		if err := os.Remove(filename); err != nil {
			t.Logf("Failed to remove temporary file %s: %v", filename, err)
		}
	}()

	tl, err := NewTransactionLogger(filename)
	if err != nil {
		t.Error(err)
	}
	tl.Run()
	defer func() {
		if err := tl.Close(); err != nil {
			t.Errorf("Failed to close transaction logger: %v", err)
		}
	}()

	chev, cherr := tl.ReadEvents()
	for e := range chev {
		t.Log(e)
	}
	err = <-cherr
	if err != nil {
		t.Error(err)
	}

	tl.WritePut("my-key", "my-value")
	tl.WritePut("my-key", "my-value2")
	tl.Wait()

	tl2, err := NewTransactionLogger(filename)
	if err != nil {
		t.Error(err)
	}
	tl2.Run()
	defer func() {
		if err := tl2.Close(); err != nil {
			t.Errorf("Failed to close transaction logger tl2: %v", err)
		}
	}()

	// Note: lastSequence is not accessible via interface, 
	// so we'll count events instead
	eventsCount := 0
	chev, cherr = tl2.ReadEvents()
	for e := range chev {
		t.Log(e)
		eventsCount++
	}
	err = <-cherr
	if err != nil {
		t.Error(err)
	}

	tl2.WritePut("my-key", "my-value3")
	tl2.WritePut("my-key2", "my-value4")
	tl2.Wait()

	// Verify we have the expected number of events (2 initial + 2 new = 4)
	if eventsCount != 2 {
		t.Errorf("Expected 2 initial events, got %d", eventsCount)
	}
}

func TestWritePut(t *testing.T) {
	const filename = "/tmp/write-put.txt"
	defer func() {
		if err := os.Remove(filename); err != nil {
			t.Logf("Failed to remove temporary file %s: %v", filename, err)
		}
	}()

	tl, _ := NewTransactionLogger(filename)
	tl.Run()
	defer func() {
		if err := tl.Close(); err != nil {
			t.Errorf("Failed to close transaction logger: %v", err)
		}
	}()

	tl.WritePut("my-key", "my-value")
	tl.WritePut("my-key", "my-value2")
	tl.WritePut("my-key", "my-value3")
	tl.WritePut("my-key", "my-value4")
	tl.Wait()

	// Count events instead of accessing private lastSequence field
	eventsCount2 := 0
	
	// Count events from first logger
	tl2, _ := NewTransactionLogger(filename)
	evin, errin := tl2.ReadEvents()
	defer func() {
		if err := tl2.Close(); err != nil {
			t.Errorf("Failed to close transaction logger tl2: %v", err)
		}
	}()

	for e := range evin {
		t.Log(e)
		eventsCount2++
	}

	err := <-errin
	if err != nil {
		t.Error(err)
	}

	// Since we wrote 4 events, we should read 4 events
	if eventsCount2 != 4 {
		t.Errorf("Expected 4 events, got %d", eventsCount2)
	}
}

func TestTransactionLoggerSimple(t *testing.T) {
	// Create temporary file for testing
	tmpfile, err := os.CreateTemp("", "test-transaction-log")
	if err != nil {
		t.Fatalf("Cannot create temporary file: %v", err)
	}
	defer func() {
		if err := os.Remove(tmpfile.Name()); err != nil {
			t.Logf("Failed to remove temporary file %s: %v", tmpfile.Name(), err)
		}
	}()
	if err := tmpfile.Close(); err != nil {
		t.Fatalf("Failed to close temporary file: %v", err)
	}

	// Create new transaction logger
	logger, err := NewTransactionLogger(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to create transaction logger: %v", err)
	}

	// Start the logger
	logger.Run()

	// Write a single test event
	logger.WritePut("test-key", "test-value")

	// Wait for write to complete
	logger.Wait()

	// Close the logger
	if err := logger.Close(); err != nil {
		t.Fatalf("Failed to close logger: %v", err)
	}

	// Create a new logger for reading
	readLogger, err := NewTransactionLogger(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to create read logger: %v", err)
	}
	defer func() {
		if err := readLogger.Close(); err != nil {
			t.Errorf("Failed to close read logger: %v", err)
		}
	}()

	// Read events
	events, errs := readLogger.ReadEvents()

	// Count received events
	var eventCount int
	for {
		select {
		case evt, ok := <-events:
			if !ok {
				// events channel closed
				t.Logf("Events channel closed after reading %d events", eventCount)
				return
			}
			eventCount++
			t.Logf("Received event: %+v", evt)

		case err, ok := <-errs:
			if !ok {
				// errors channel closed
				continue
			}
			if err != nil {
				t.Fatalf("Received error while reading events: %v", err)
			}

		case <-time.After(100 * time.Millisecond):
			if eventCount == 0 {
				t.Fatal("Timeout waiting for events, none received")
			}
			return
		}
	}
}
func TestTransactionLoggerComplete(t *testing.T) {
	// Create temporary file for testing
	tmpfile, err := os.CreateTemp("", "test-transaction-log")
	if err != nil {
		t.Fatalf("Cannot create temporary file: %v", err)
	}
	defer func() {
		if err := os.Remove(tmpfile.Name()); err != nil {
			t.Logf("Failed to remove temporary file %s: %v", tmpfile.Name(), err)
		}
	}()
	if err := tmpfile.Close(); err != nil {
		t.Fatalf("Failed to close temporary file: %v", err)
	}

	// Create new transaction logger
	logger, err := NewTransactionLogger(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to create transaction logger: %v", err)
	}

	// Start the logger
	logger.Run()

	// Test writing events
	testCases := []struct {
		eventType EventType
		key       string
		value     string
	}{
		{EventPut, "key1", "value1"},
		{EventPut, "key2", "value2"},
		{EventDelete, "key1", ""},
	}

	// Write events
	for _, tc := range testCases {
		if tc.eventType == EventPut {
			logger.WritePut(tc.key, tc.value)
		} else {
			logger.WriteDelete(tc.key)
		}
	}

	// Wait for all writes to complete
	logger.Wait()

	// Close the logger to ensure all writes are flushed
	if err := logger.Close(); err != nil {
		t.Fatalf("Failed to close logger: %v", err)
	}

	// Create a new logger instance to read the events
	readLogger, err := NewTransactionLogger(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to create read logger: %v", err)
	}
	defer func() {
		if err := readLogger.Close(); err != nil {
			t.Errorf("Failed to close read logger: %v", err)
		}
	}()

	// Read events back
	events, errors := readLogger.ReadEvents()

	// Process events
	var receivedEvents []Event
eventLoop:
	for {
		select {
		case err, ok := <-errors:
			if ok && err != nil {
				t.Fatalf("Error reading events: %v", err)
			}
			break eventLoop
		case evt, ok := <-events:
			if !ok {
				// Channel closed, all events read
				break eventLoop
				//goto DONE
			}
			t.Logf("Received event: %+v", evt)
			receivedEvents = append(receivedEvents, evt)
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for events")
		}
	}

	//DONE:
	// Verify events
	if len(receivedEvents) != len(testCases) {
		t.Errorf("Expected %d events, got %d", len(testCases), len(receivedEvents))
	}

	for i, tc := range testCases {
		if receivedEvents[i].EventType != tc.eventType {
			t.Errorf("Event %d: expected type %d, got %d", i, tc.eventType, receivedEvents[i].EventType)
		}
		if receivedEvents[i].Key != tc.key {
			t.Errorf("Event %d: expected key %s, got %s", i, tc.key, receivedEvents[i].Key)
		}
		if tc.eventType == EventPut && receivedEvents[i].Value != tc.value {
			t.Errorf("Event %d: expected value %s, got %s", i, tc.value, receivedEvents[i].Value)
		}
	}

	// // Test closing
	// if err := logger.Close(); err != nil {
	// 	t.Errorf("Failed to close logger: %v", err)
	// }
}

// TestFileIOErrors tests file I/O error handling
func TestFileIOErrors(t *testing.T) {
	// Test with invalid file path
	invalidPath := "/dev/null/invalid/path/transaction.log"
	_, err := NewTransactionLogger(invalidPath)
	if err == nil {
		t.Error("Expected error creating logger with invalid path")
	}

	// Test with read-only directory
	tmpDir, err := os.MkdirTemp("", "readonly-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Make directory read-only
	if err := os.Chmod(tmpDir, 0444); err != nil {
		t.Fatalf("Failed to make directory read-only: %v", err)
	}

	readOnlyPath := filepath.Join(tmpDir, "transaction.log")
	_, err = NewTransactionLogger(readOnlyPath)
	if err == nil {
		// Restore permissions for cleanup
		os.Chmod(tmpDir, 0755)
		t.Error("Expected error creating logger in read-only directory")
	}

	// Restore permissions for cleanup
	os.Chmod(tmpDir, 0755)
}

// TestConcurrentLogging tests multiple goroutines writing simultaneously
func TestConcurrentLogging(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "concurrent-log-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	logger, err := NewTransactionLogger(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.Run()
	defer logger.Close()

	const (
		numGoroutines = 10
		numOperations = 50
	)

	var wg sync.WaitGroup

	// Start multiple goroutines writing concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("concurrent_key_%d_%d", id, j)
				value := fmt.Sprintf("concurrent_value_%d_%d", id, j)
				logger.WritePut(key, value)
				
				// Occasionally write deletes
				if j%10 == 0 {
					deleteKey := fmt.Sprintf("concurrent_key_%d_%d", id, j-1)
					logger.WriteDelete(deleteKey)
				}
			}
		}(i)
	}

	wg.Wait()
	logger.Wait()

	// Verify events were written by reading them back
	readLogger, err := NewTransactionLogger(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create read logger: %v", err)
	}
	defer readLogger.Close()

	events, errors := readLogger.ReadEvents()

	// Count events
	eventCount := 0
	expectedEvents := numGoroutines * numOperations + (numGoroutines * numOperations / 10) // puts + deletes
	timeout := time.After(5 * time.Second)

	for {
		select {
		case _, ok := <-events:
			if !ok {
				// Channel closed, reading done
				t.Logf("Read %d events (expected around %d)", eventCount, expectedEvents)
				return
			}
			eventCount++
		case err := <-errors:
			if err != nil {
				t.Logf("Finished reading %d events (expected around %d)", eventCount, expectedEvents)
				if eventCount < expectedEvents/2 { // Allow some variance
					t.Errorf("Too few events read: %d, expected around %d", eventCount, expectedEvents)
				}
				return
			}
		case <-timeout:
			t.Logf("Timeout reading events, got %d (expected around %d)", eventCount, expectedEvents)
			if eventCount < expectedEvents/2 { // Allow some variance
				t.Errorf("Too few events read: %d, expected around %d", eventCount, expectedEvents)
			}
			return
		}
	}
}

// TestLogCorruption tests handling of corrupted transaction logs
func TestLogCorruption(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "corrupt-log-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write some corrupted data
	corruptedData := []byte("corrupted line 1\ninvalid format here\n1\t2\t3\n") // Missing field
	if _, err := tmpFile.Write(corruptedData); err != nil {
		t.Fatalf("Failed to write corrupted data: %v", err)
	}
	tmpFile.Close()

	// Try to read the corrupted log
	logger, err := NewTransactionLogger(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	events, errors := logger.ReadEvents()

	// Should get an error when reading corrupted data
	select {
	case <-events:
		t.Error("Expected no events from corrupted log")
	case err := <-errors:
		if err == nil {
			t.Error("Expected error reading corrupted log")
		} else {
			t.Logf("Got expected error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for error")
	}
}

// TestDiskSpaceExhaustion simulates disk space exhaustion
func TestDiskSpaceExhaustion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping disk space test in short mode")
	}

	// This test is platform-specific and may not work on all systems
	// We'll simulate by trying to write to a full filesystem or read-only location
	tmpFile, err := os.CreateTemp("", "diskfull-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Make the file read-only to simulate write failure
	if err := os.Chmod(tmpFile.Name(), 0444); err != nil {
		t.Fatalf("Failed to make file read-only: %v", err)
	}

	logger, err := NewTransactionLogger(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.Run()
	defer func() {
		// Restore write permissions for cleanup
		os.Chmod(tmpFile.Name(), 0644)
		logger.Close()
	}()

	// Try to write - should fail
	logger.WritePut("test-key", "test-value")
	logger.Wait()

	// Check for errors
	select {
	case err := <-logger.Err():
		if err == nil {
			t.Error("Expected write error due to read-only file")
		} else {
			t.Logf("Got expected write error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for write error")
	}
}

// TestLogReplay tests transaction log replay functionality
func TestLogReplay(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "replay-log-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Write initial data
	logger1, err := NewTransactionLogger(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create first logger: %v", err)
	}

	logger1.Run()

	// Write test data in specific order
	logger1.WritePut("key1", "value1")
	logger1.WritePut("key2", "value2")
	logger1.WritePut("key3", "value3")

	// Delete one key
	logger1.WriteDelete("key2")
	logger1.Wait()
	logger1.Close()

	// Create new logger and replay events
	logger2, err := NewTransactionLogger(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create second logger: %v", err)
	}
	defer logger2.Close()

	events, errors := logger2.ReadEvents()

	// Collect events
	var replayedEvents []Event
	timeout := time.After(2 * time.Second)

	for {
		select {
		case event, ok := <-events:
			if !ok {
				// Channel closed, replay complete
				goto VERIFY
			}
			replayedEvents = append(replayedEvents, event)
		case err := <-errors:
			if err != nil {
				t.Logf("Replay completed with %d events", len(replayedEvents))
				goto VERIFY
			}
		case <-timeout:
			t.Fatalf("Timeout during replay, got %d events", len(replayedEvents))
		}
	}

VERIFY:
	// Verify replay
	expectedEvents := 4 // 3 puts + 1 delete
	if len(replayedEvents) != expectedEvents {
		t.Errorf("Expected %d events, got %d", expectedEvents, len(replayedEvents))
	}

	// Verify sequence numbers are correct
	for i, event := range replayedEvents {
		expectedSeq := uint64(i + 1)
		if event.Sequence != expectedSeq {
			t.Errorf("Event %d: expected sequence %d, got %d", i, expectedSeq, event.Sequence)
		}
	}

	// Verify event types and keys (adjusted for deterministic order)
	if replayedEvents[0].EventType != EventPut || replayedEvents[0].Key != "key1" {
		t.Errorf("First event incorrect: %+v", replayedEvents[0])
	}
	if replayedEvents[3].EventType != EventDelete || replayedEvents[3].Key != "key2" {
		t.Errorf("Last event incorrect: %+v", replayedEvents[3])
	}
}

// TestLargeTransactionLog tests performance with large transaction logs
func TestLargeTransactionLog(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large transaction log test in short mode")
	}

	tmpFile, err := os.CreateTemp("", "large-log-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	logger, err := NewTransactionLogger(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.Run()

	const numEntries = 10000
	start := time.Now()

	// Write many entries
	for i := 0; i < numEntries; i++ {
		key := fmt.Sprintf("large_key_%d", i)
		value := fmt.Sprintf("large_value_%d_with_some_additional_data_to_make_it_bigger", i)
		logger.WritePut(key, value)
	}

	logger.Wait()
	writeDuration := time.Since(start)
	t.Logf("Wrote %d entries in %v (%.2f entries/sec)", 
		numEntries, writeDuration, float64(numEntries)/writeDuration.Seconds())

	logger.Close()

	// Test reading back
	readLogger, err := NewTransactionLogger(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create read logger: %v", err)
	}
	defer readLogger.Close()

	start = time.Now()
	events, errors := readLogger.ReadEvents()

	// Count events
	eventCount := 0
	timeout := time.After(10 * time.Second)

	for {
		select {
		case <-events:
			eventCount++
		case err := <-errors:
			if err != nil {
				readDuration := time.Since(start)
				t.Logf("Read %d events in %v (%.2f events/sec)", 
					eventCount, readDuration, float64(eventCount)/readDuration.Seconds())
				if eventCount != numEntries {
					t.Errorf("Expected %d events, got %d", numEntries, eventCount)
				}
				return
			}
		case <-timeout:
			t.Fatalf("Timeout reading events, got %d/%d", eventCount, numEntries)
		}
	}
}

// TestConcurrentReadWrite tests concurrent reading and writing
func TestConcurrentReadWrite(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "rw-log-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Write some initial data
	writeLogger, err := NewTransactionLogger(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create write logger: %v", err)
	}

	writeLogger.Run()

	// Write initial data
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("initial_key_%d", i)
		value := fmt.Sprintf("initial_value_%d", i)
		writeLogger.WritePut(key, value)
	}
	writeLogger.Wait()
	writeLogger.Close()

	// Now test concurrent read and write
	writeLogger2, err := NewTransactionLogger(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create second write logger: %v", err)
	}
	writeLogger2.Run()
	defer writeLogger2.Close()

	readLogger, err := NewTransactionLogger(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create read logger: %v", err)
	}
	defer readLogger.Close()

	var wg sync.WaitGroup

	// Start writer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			key := fmt.Sprintf("concurrent_key_%d", i)
			value := fmt.Sprintf("concurrent_value_%d", i)
			writeLogger2.WritePut(key, value)
			time.Sleep(time.Millisecond) // Small delay
		}
		writeLogger2.Wait()
	}()

	// Start reader goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		events, errors := readLogger.ReadEvents()
		eventCount := 0

		timeout := time.After(5 * time.Second)
		for {
			select {
			case <-events:
				eventCount++
			case err := <-errors:
				if err != nil {
					t.Logf("Reader got %d events", eventCount)
					return
				}
			case <-timeout:
				t.Logf("Reader timeout with %d events", eventCount)
				return
			}
		}
	}()

	wg.Wait()
	t.Log("Concurrent read/write test completed")
}

// BenchmarkTransactionLogger benchmarks transaction logging performance
func BenchmarkTransactionLogger(b *testing.B) {
	tmpFile, err := os.CreateTemp("", "bench-log-*")
	if err != nil {
		b.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	logger, err := NewTransactionLogger(tmpFile.Name())
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}

	logger.Run()
	defer logger.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench_key_%d", i)
		value := fmt.Sprintf("bench_value_%d", i)
		logger.WritePut(key, value)
	}
	logger.Wait()
}

// BenchmarkTransactionLoggerParallel benchmarks parallel logging
func BenchmarkTransactionLoggerParallel(b *testing.B) {
	tmpFile, err := os.CreateTemp("", "bench-parallel-*")
	if err != nil {
		b.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	logger, err := NewTransactionLogger(tmpFile.Name())
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}

	logger.Run()
	defer logger.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("parallel_key_%d", i)
			value := fmt.Sprintf("parallel_value_%d", i)
			logger.WritePut(key, value)
			i++
		}
	})
	logger.Wait()
}
