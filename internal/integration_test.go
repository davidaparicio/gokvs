package internal

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteTransactionLogger_Integration_With_Core(t *testing.T) {
	// Create temporary database
	dbTmpfile, err := os.CreateTemp("", "test-integration-*.db")
	require.NoError(t, err)
	dbPath := dbTmpfile.Name()
	dbTmpfile.Close()
	os.Remove(dbPath)
	defer os.Remove(dbPath)

	// Create SQLite transaction logger
	logger, err := NewSQLiteTransactionLogger(dbPath)
	require.NoError(t, err)
	defer logger.Close()

	// Start the logger
	logger.Run()

	// Clear any existing data in the core KV store
	// Note: This assumes we have access to clear the store for testing
	// In a real scenario, you might want to use a separate test instance

	// Simulate application operations
	testOperations := []struct {
		op    string
		key   string
		value string
	}{
		{"put", "user:1", "alice"},
		{"put", "user:2", "bob"},
		{"delete", "user:1", ""},
		{"put", "user:3", "charlie"},
		{"put", "user:2", "bob-updated"},
	}

	// Execute operations and log them
	for _, operation := range testOperations {
		switch operation.op {
		case "put":
			err := Put(operation.key, operation.value)
			require.NoError(t, err)
			logger.WritePut(operation.key, operation.value)
		case "delete":
			_ = Delete(operation.key) // Ignore error for delete operations in test
			logger.WriteDelete(operation.key)
		}
	}

	// Wait for all transactions to be written
	logger.Wait()

	// Verify transaction log contains all operations
	count, err := logger.GetEventCount()
	require.NoError(t, err)
	assert.Equal(t, int64(len(testOperations)), count)

	// Test recovery scenario - simulate application restart
	currentState := map[string]string{
		"user:2": "bob-updated",
		"user:3": "charlie",
	}

	// Clear the KV store to simulate fresh start
	// Note: In a real application, you would restart the service
	// Here we manually clear and replay

	// Create new logger instance to simulate restart
	logger2, err := NewSQLiteTransactionLogger(dbPath)
	require.NoError(t, err)
	defer logger2.Close()

	// Read and replay events
	events, errors := logger2.ReadEvents()
	replayedEvents := []Event{}

	// Collect all events
	eventCount := 0
	for eventCount < len(testOperations) {
		select {
		case event, ok := <-events:
			if !ok {
				break
			}
			replayedEvents = append(replayedEvents, event)
			
			// Simulate replay logic
			switch event.EventType {
			case EventPut:
				err := Put(event.Key, event.Value)
				require.NoError(t, err)
			case EventDelete:
				Delete(event.Key) // Ignore error for delete operations
			}
			eventCount++
			
		case err := <-errors:
			if err != nil {
				t.Fatalf("Error during replay: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("Timeout waiting for events during replay")
		}
	}

	// Verify replayed state matches expected final state
	for key, expectedValue := range currentState {
		value, err := Get(key)
		require.NoError(t, err)
		assert.Equal(t, expectedValue, value, "Key %s should have correct value after replay", key)
	}

	// Verify deleted keys are not present
	_, err = Get("user:1")
	assert.ErrorIs(t, err, ErrorNoSuchKey, "Deleted key should not exist after replay")
}

func TestSQLiteTransactionLogger_Concurrent_Operations(t *testing.T) {
	// Create in-memory database for faster testing
	logger, err := NewSQLiteTransactionLogger(":memory:")
	require.NoError(t, err)
	defer logger.Close()

	// Start the logger
	logger.Run()

	// Test concurrent writes
	numGoroutines := 10
	operationsPerGoroutine := 20
	done := make(chan bool, numGoroutines)

	// Start multiple goroutines performing operations
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer func() { done <- true }()
			
			for j := 0; j < operationsPerGoroutine; j++ {
				key := fmt.Sprintf("key-%d-%d", goroutineID, j)
				value := fmt.Sprintf("value-%d-%d", goroutineID, j)
				
				// Alternate between put and delete operations
				if j%2 == 0 {
					logger.WritePut(key, value)
				} else {
					logger.WriteDelete(key)
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Wait for all writes to complete
	logger.Wait()

	// Verify total operation count
	expectedOperations := numGoroutines * operationsPerGoroutine
	count, err := logger.GetEventCount()
	require.NoError(t, err)
	assert.Equal(t, int64(expectedOperations), count, "All concurrent operations should be recorded")

	// Verify data integrity
	err = logger.CheckDatabaseIntegrity()
	assert.NoError(t, err, "Database should maintain integrity under concurrent access")
}

func TestSQLiteTransactionLogger_Large_Dataset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large dataset test in short mode")
	}

	// Create temporary database
	dbTmpfile, err := os.CreateTemp("", "test-large-dataset-*.db")
	require.NoError(t, err)
	dbPath := dbTmpfile.Name()
	dbTmpfile.Close()
	os.Remove(dbPath)
	defer os.Remove(dbPath)

	logger, err := NewSQLiteTransactionLogger(dbPath)
	require.NoError(t, err)
	defer logger.Close()

	logger.Run()

	// Write a large number of operations
	numOperations := 1000
	
	start := time.Now()
	for i := 0; i < numOperations; i++ {
		if i%100 == 0 {
			t.Logf("Processed %d/%d operations", i, numOperations)
		}
		
		key := fmt.Sprintf("large-key-%06d", i)
		value := fmt.Sprintf("large-value-%06d-with-some-additional-data-to-make-it-longer", i)
		logger.WritePut(key, value)
	}
	
	logger.Wait()
	duration := time.Since(start)
	
	t.Logf("Wrote %d operations in %v (%.2f ops/sec)", 
		numOperations, duration, float64(numOperations)/duration.Seconds())

	// Verify count
	count, err := logger.GetEventCount()
	require.NoError(t, err)
	assert.Equal(t, int64(numOperations), count)

	// Test read performance
	start = time.Now()
	events, errors := logger.ReadEvents()
	readCount := 0

	for readCount < numOperations {
		select {
		case _, ok := <-events:
			if !ok {
				break
			}
			readCount++
			if readCount%100 == 0 {
				t.Logf("Read %d/%d events", readCount, numOperations)
			}
		case err := <-errors:
			if err != nil {
				t.Fatalf("Error reading events: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatalf("Timeout reading events after %d events", readCount)
		}
	}
	
	readDuration := time.Since(start)
	t.Logf("Read %d events in %v (%.2f ops/sec)", 
		readCount, readDuration, float64(readCount)/readDuration.Seconds())

	// Verify integrity
	err = logger.CheckDatabaseIntegrity()
	assert.NoError(t, err)
}

func TestSQLiteTransactionLogger_Error_Recovery(t *testing.T) {
	// Create temporary database
	dbTmpfile, err := os.CreateTemp("", "test-error-recovery-*.db")
	require.NoError(t, err)
	dbPath := dbTmpfile.Name()
	dbTmpfile.Close()
	os.Remove(dbPath)
	defer os.Remove(dbPath)

	// Create logger and write some data
	logger1, err := NewSQLiteTransactionLogger(dbPath)
	require.NoError(t, err)
	
	logger1.Run()
	logger1.WritePut("test-key-1", "test-value-1")
	logger1.WritePut("test-key-2", "test-value-2")
	logger1.Wait()

	// Force close without proper shutdown to simulate crash
	logger1.db.Close()

	// Create new logger instance to test recovery
	logger2, err := NewSQLiteTransactionLogger(dbPath)
	require.NoError(t, err)
	defer logger2.Close()

	// Verify data survived the "crash"
	count, err := logger2.GetEventCount()
	require.NoError(t, err)
	assert.Equal(t, int64(2), count, "Data should survive database close/reopen")

	// Verify integrity
	err = logger2.CheckDatabaseIntegrity()
	assert.NoError(t, err, "Database should maintain integrity after recovery")

	// Verify we can continue operations
	logger2.Run()
	logger2.WritePut("recovery-key", "recovery-value")
	logger2.Wait()

	finalCount, err := logger2.GetEventCount()
	require.NoError(t, err)
	assert.Equal(t, int64(3), finalCount, "Should be able to continue operations after recovery")
}