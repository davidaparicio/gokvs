package internal

import (
	"os"
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

	defer os.Remove(filename)

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
	defer os.Remove(filename)

	tl, err := NewTransactionLogger(filename)
	if err != nil {
		t.Error(err)
	}
	tl.Run()
	defer tl.Close()

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
	defer tl2.Close()

	chev, cherr = tl2.ReadEvents()
	for e := range chev {
		t.Log(e)
	}
	err = <-cherr
	if err != nil {
		t.Error(err)
	}

	tl2.WritePut("my-key", "my-value3")
	tl2.WritePut("my-key2", "my-value4")
	tl2.Wait()

	if tl2.lastSequence != 4 {
		t.Errorf("Last sequence mismatch (expected 4; got %d)", tl2.lastSequence)
	}
}

func TestWritePut(t *testing.T) {
	const filename = "/tmp/write-put.txt"
	defer os.Remove(filename)

	tl, _ := NewTransactionLogger(filename)
	tl.Run()
	defer tl.Close()

	tl.WritePut("my-key", "my-value")
	tl.WritePut("my-key", "my-value2")
	tl.WritePut("my-key", "my-value3")
	tl.WritePut("my-key", "my-value4")
	tl.Wait()

	tl2, _ := NewTransactionLogger(filename)
	evin, errin := tl2.ReadEvents()
	defer tl2.Close()

	for e := range evin {
		t.Log(e)
	}

	err := <-errin
	if err != nil {
		t.Error(err)
	}

	if tl.lastSequence != tl2.lastSequence {
		t.Errorf("Last sequence mismatch (%d vs %d)", tl.lastSequence, tl2.lastSequence)
	}
}

func TestTransactionLoggerSimple(t *testing.T) {
	// Create temporary file for testing
	tmpfile, err := os.CreateTemp("", "test-transaction-log")
	if err != nil {
		t.Fatalf("Cannot create temporary file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

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
	defer readLogger.Close()

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
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

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
	defer readLogger.Close()

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
