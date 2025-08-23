package internal

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSQLiteTransactionLogger(t *testing.T) {
	// Create temporary database file for testing
	tmpfile, err := os.CreateTemp("", "test-sqlite-*.db")
	require.NoError(t, err)
	dbPath := tmpfile.Name()
	tmpfile.Close()
	os.Remove(dbPath) // Remove file so SQLite can create it fresh

	defer func() {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			t.Logf("Failed to remove temporary database %s: %v", dbPath, err)
		}
	}()

	// Create new SQLite transaction logger
	logger, err := NewSQLiteTransactionLogger(dbPath)
	require.NoError(t, err)
	require.NotNil(t, logger)

	defer func() {
		if err := logger.Close(); err != nil {
			t.Errorf("Failed to close logger: %v", err)
		}
	}()

	// Verify database file was created
	_, err = os.Stat(dbPath)
	assert.NoError(t, err, "Database file should exist")

	// Verify integrity check passes
	err = logger.CheckDatabaseIntegrity()
	assert.NoError(t, err, "Database integrity check should pass")
}

func TestSQLiteTransactionLogger_WritePut(t *testing.T) {
	// Setup in-memory SQLite database for faster testing
	logger, err := NewSQLiteTransactionLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	// Start the logger
	logger.Run()

	// Test WritePut operation
	logger.WritePut("test-key", "test-value")
	logger.Wait()

	// Verify data in database
	count, err := logger.GetEventCount()
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Read events and verify
	events, errors := logger.ReadEvents()
	
	select {
	case event := <-events:
		assert.Equal(t, EventPut, event.EventType)
		assert.Equal(t, "test-key", event.Key)
		assert.Equal(t, "test-value", event.Value)
		assert.Equal(t, uint64(1), event.Sequence)
	case err := <-errors:
		t.Fatalf("Unexpected error: %v", err)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for event")
	}
}

func TestSQLiteTransactionLogger_WriteDelete(t *testing.T) {
	// Setup in-memory SQLite database
	logger, err := NewSQLiteTransactionLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	// Start the logger
	logger.Run()

	// Test WriteDelete operation
	logger.WriteDelete("test-key")
	logger.Wait()

	// Verify data in database
	count, err := logger.GetEventCount()
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Read events and verify
	events, errors := logger.ReadEvents()
	
	select {
	case event := <-events:
		assert.Equal(t, EventDelete, event.EventType)
		assert.Equal(t, "test-key", event.Key)
		assert.Equal(t, "", event.Value)
		assert.Equal(t, uint64(1), event.Sequence)
	case err := <-errors:
		t.Fatalf("Unexpected error: %v", err)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for event")
	}
}

func TestSQLiteTransactionLogger_Multiple_Operations(t *testing.T) {
	// Setup in-memory SQLite database
	logger, err := NewSQLiteTransactionLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	// Start the logger
	logger.Run()

	// Test multiple operations
	testCases := []struct {
		eventType EventType
		key       string
		value     string
	}{
		{EventPut, "key1", "value1"},
		{EventPut, "key2", "value2"},
		{EventDelete, "key1", ""},
		{EventPut, "key3", "value3"},
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

	// Verify total event count
	count, err := logger.GetEventCount()
	require.NoError(t, err)
	assert.Equal(t, int64(len(testCases)), count)

	// Read all events and verify order and content
	events, errors := logger.ReadEvents()
	receivedEvents := []Event{}

	for i := 0; i < len(testCases); i++ {
		select {
		case event := <-events:
			receivedEvents = append(receivedEvents, event)
		case err := <-errors:
			t.Fatalf("Unexpected error: %v", err)
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for events")
		}
	}

	// Verify events match expected
	for i, tc := range testCases {
		event := receivedEvents[i]
		assert.Equal(t, tc.eventType, event.EventType, "Event %d type mismatch", i)
		assert.Equal(t, tc.key, event.Key, "Event %d key mismatch", i)
		assert.Equal(t, tc.value, event.Value, "Event %d value mismatch", i)
		assert.Equal(t, uint64(i+1), event.Sequence, "Event %d sequence mismatch", i)
	}
}

func TestSQLiteTransactionLogger_URL_Encoding(t *testing.T) {
	// Setup in-memory SQLite database
	logger, err := NewSQLiteTransactionLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	// Start the logger
	logger.Run()

	// Test with special characters that need URL encoding
	specialValue := "hello world & special chars: +/?#[]@!$&'()*+,;="
	logger.WritePut("special-key", specialValue)
	logger.Wait()

	// Read events and verify proper encoding/decoding
	events, errors := logger.ReadEvents()
	
	select {
	case event := <-events:
		assert.Equal(t, EventPut, event.EventType)
		assert.Equal(t, "special-key", event.Key)
		assert.Equal(t, specialValue, event.Value, "Special characters should be properly encoded/decoded")
	case err := <-errors:
		t.Fatalf("Unexpected error: %v", err)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for event")
	}
}

func TestSQLiteTransactionLogger_Error_Handling(t *testing.T) {
	// Test with invalid database path
	_, err := NewSQLiteTransactionLogger("/invalid/path/database.db")
	assert.Error(t, err, "Should fail with invalid database path")

	// Test with valid logger
	logger, err := NewSQLiteTransactionLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	// Test integrity check
	err = logger.CheckDatabaseIntegrity()
	assert.NoError(t, err, "Integrity check should pass on new database")
}

func TestSQLiteTransactionLogger_Persistence(t *testing.T) {
	// Create temporary database file
	tmpfile, err := os.CreateTemp("", "test-sqlite-persistence-*.db")
	require.NoError(t, err)
	dbPath := tmpfile.Name()
	tmpfile.Close()
	os.Remove(dbPath) // Remove file so SQLite can create it fresh

	defer func() {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			t.Logf("Failed to remove temporary database %s: %v", dbPath, err)
		}
	}()

	// Create first logger and write some data
	logger1, err := NewSQLiteTransactionLogger(dbPath)
	require.NoError(t, err)
	
	logger1.Run()
	logger1.WritePut("persistent-key", "persistent-value")
	logger1.WriteDelete("temp-key")
	logger1.Wait()
	
	err = logger1.Close()
	require.NoError(t, err)

	// Create second logger with same database
	logger2, err := NewSQLiteTransactionLogger(dbPath)
	require.NoError(t, err)
	defer logger2.Close()

	// Verify data persisted
	count, err := logger2.GetEventCount()
	require.NoError(t, err)
	assert.Equal(t, int64(2), count, "Events should persist across logger instances")

	// Read events and verify
	events, errors := logger2.ReadEvents()
	receivedEvents := []Event{}

	for i := 0; i < 2; i++ {
		select {
		case event := <-events:
			receivedEvents = append(receivedEvents, event)
		case err := <-errors:
			t.Fatalf("Unexpected error: %v", err)
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for events")
		}
	}

	// Verify events
	assert.Equal(t, EventPut, receivedEvents[0].EventType)
	assert.Equal(t, "persistent-key", receivedEvents[0].Key)
	assert.Equal(t, "persistent-value", receivedEvents[0].Value)

	assert.Equal(t, EventDelete, receivedEvents[1].EventType)
	assert.Equal(t, "temp-key", receivedEvents[1].Key)
}

func BenchmarkSQLiteTransactionLogger_WritePut(b *testing.B) {
	logger, err := NewSQLiteTransactionLogger(":memory:")
	require.NoError(b, err)
	defer logger.Close()

	logger.Run()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.WritePut(fmt.Sprintf("key%d", i), "value")
	}
	logger.Wait()
}

func BenchmarkSQLiteTransactionLogger_WriteDelete(b *testing.B) {
	logger, err := NewSQLiteTransactionLogger(":memory:")
	require.NoError(b, err)
	defer logger.Close()

	logger.Run()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.WriteDelete(fmt.Sprintf("key%d", i))
	}
	logger.Wait()
}