package internal

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateFileToSQLite(t *testing.T) {
	// Create temporary file with test data
	tmpfile, err := os.CreateTemp("", "test-migration-*.log")
	require.NoError(t, err)
	logFilePath := tmpfile.Name()
	defer os.Remove(logFilePath)

	// Create temporary database path
	dbTmpfile, err := os.CreateTemp("", "test-migration-*.db")
	require.NoError(t, err)
	dbPath := dbTmpfile.Name()
	dbTmpfile.Close()
	os.Remove(dbPath) // Remove file so SQLite can create it fresh
	defer os.Remove(dbPath)

	// Write test data to the file
	testData := `1	2	test-key1	test-value1
2	1	test-key2	
3	2	test-key3	test-value3
`
	_, err = tmpfile.WriteString(testData)
	require.NoError(t, err)
	err = tmpfile.Close()
	require.NoError(t, err)

	// Perform migration
	sqliteLogger, err := MigrateFileToSQLite(logFilePath, dbPath)
	require.NoError(t, err)
	require.NotNil(t, sqliteLogger)
	defer sqliteLogger.Close()

	// Verify migration results
	count, err := sqliteLogger.GetEventCount()
	require.NoError(t, err)
	assert.Equal(t, int64(3), count, "All events should be migrated")

	// Verify events were migrated correctly
	events, errors := sqliteLogger.ReadEvents()
	receivedEvents := []Event{}

	for i := 0; i < 3; i++ {
		select {
		case event := <-events:
			receivedEvents = append(receivedEvents, event)
		case err := <-errors:
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
		}
	}

	// Verify migrated events
	expectedEvents := []Event{
		{Sequence: 1, EventType: EventPut, Key: "test-key1", Value: "test-value1"},
		{Sequence: 2, EventType: EventDelete, Key: "test-key2", Value: ""},
		{Sequence: 3, EventType: EventPut, Key: "test-key3", Value: "test-value3"},
	}

	require.Len(t, receivedEvents, 3)
	for i, expected := range expectedEvents {
		assert.Equal(t, expected.EventType, receivedEvents[i].EventType, "Event %d type mismatch", i)
		assert.Equal(t, expected.Key, receivedEvents[i].Key, "Event %d key mismatch", i)
		assert.Equal(t, expected.Value, receivedEvents[i].Value, "Event %d value mismatch", i)
	}

	// Verify original file was archived (renamed)
	_, err = os.Stat(logFilePath)
	assert.True(t, os.IsNotExist(err), "Original log file should be archived/renamed")
}

func TestMigrateFileToSQLite_NoFile(t *testing.T) {
	// Test migration when no file exists
	nonExistentFile := "/tmp/non-existent-file.log"

	dbTmpfile, err := os.CreateTemp("", "test-migration-nofile-*.db")
	require.NoError(t, err)
	dbPath := dbTmpfile.Name()
	dbTmpfile.Close()
	os.Remove(dbPath)
	defer os.Remove(dbPath)

	// Perform migration with non-existent file
	sqliteLogger, err := MigrateFileToSQLite(nonExistentFile, dbPath)
	require.NoError(t, err)
	require.NotNil(t, sqliteLogger)
	defer sqliteLogger.Close()

	// Should create empty database
	count, err := sqliteLogger.GetEventCount()
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "Should create empty database when no file exists")
}

func TestMigrateFileToSQLite_ExistingDatabase(t *testing.T) {
	// Create file with test data
	tmpfile, err := os.CreateTemp("", "test-migration-existing-*.log")
	require.NoError(t, err)
	logFilePath := tmpfile.Name()
	defer os.Remove(logFilePath)

	testData := `1	2	file-key	file-value
`
	_, err = tmpfile.WriteString(testData)
	require.NoError(t, err)
	err = tmpfile.Close()
	require.NoError(t, err)

	// Create database with existing data
	dbTmpfile, err := os.CreateTemp("", "test-migration-existing-*.db")
	require.NoError(t, err)
	dbPath := dbTmpfile.Name()
	dbTmpfile.Close()
	os.Remove(dbPath)
	defer os.Remove(dbPath)

	// Create SQLite logger and add some data
	existingLogger, err := NewSQLiteTransactionLogger(dbPath)
	require.NoError(t, err)
	existingLogger.Run()
	existingLogger.WritePut("existing-key", "existing-value")
	existingLogger.Wait()
	existingLogger.Close()

	// Attempt migration with existing database
	sqliteLogger, err := MigrateFileToSQLite(logFilePath, dbPath)
	require.NoError(t, err)
	require.NotNil(t, sqliteLogger)
	defer sqliteLogger.Close()

	// Should skip migration and keep existing data only
	count, err := sqliteLogger.GetEventCount()
	require.NoError(t, err)
	assert.Equal(t, int64(1), count, "Should skip migration when database already has events")

	// Verify existing data is preserved
	events, errors := sqliteLogger.ReadEvents()

	select {
	case event := <-events:
		assert.Equal(t, EventPut, event.EventType)
		assert.Equal(t, "existing-key", event.Key)
		assert.Equal(t, "existing-value", event.Value)
	case err := <-errors:
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	}

	// File should not be archived since migration was skipped
	_, err = os.Stat(logFilePath)
	assert.NoError(t, err, "Original log file should not be archived when migration is skipped")
}

func TestNewTransactionLoggerWithConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   LoggerConfig
		wantType string
		wantErr  bool
	}{
		{
			name: "SQLite logger without migration",
			config: LoggerConfig{
				Type:   "sqlite",
				DBPath: ":memory:",
			},
			wantType: "*internal.SQLiteTransactionLogger",
			wantErr:  false,
		},
		{
			name: "File logger",
			config: LoggerConfig{
				Type:     "file",
				FilePath: "/tmp/test-config.log",
			},
			wantType: "*internal.TransactionLog",
			wantErr:  false,
		},
		{
			name: "Invalid logger type",
			config: LoggerConfig{
				Type: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := NewTransactionLoggerWithConfig(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, logger)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, logger)

			// Clean up
			if logger != nil {
				logger.Close()
			}

			// Clean up test file if created
			if tt.config.Type == "file" && tt.config.FilePath != "" {
				os.Remove(tt.config.FilePath)
			}
		})
	}
}

func TestNewTransactionLoggerWithConfig_Migration(t *testing.T) {
	// Create test file with data
	tmpfile, err := os.CreateTemp("", "test-config-migration-*.log")
	require.NoError(t, err)
	logFilePath := tmpfile.Name()
	defer os.Remove(logFilePath)

	testData := `1	2	migration-key	migration-value
`
	_, err = tmpfile.WriteString(testData)
	require.NoError(t, err)
	err = tmpfile.Close()
	require.NoError(t, err)

	// Create database path
	dbTmpfile, err := os.CreateTemp("", "test-config-migration-*.db")
	require.NoError(t, err)
	dbPath := dbTmpfile.Name()
	dbTmpfile.Close()
	os.Remove(dbPath)
	defer os.Remove(dbPath)

	// Test SQLite with migration
	config := LoggerConfig{
		Type:            "sqlite",
		FilePath:        logFilePath,
		DBPath:          dbPath,
		MigrateFromFile: true,
	}

	logger, err := NewTransactionLoggerWithConfig(config)
	require.NoError(t, err)
	require.NotNil(t, logger)
	defer logger.Close()

	// Cast to SQLite logger to test specific functionality
	sqliteLogger, ok := logger.(*SQLiteTransactionLogger)
	require.True(t, ok, "Should return SQLiteTransactionLogger when type is sqlite")

	// Verify migration occurred
	count, err := sqliteLogger.GetEventCount()
	require.NoError(t, err)
	assert.Equal(t, int64(1), count, "Migration should have transferred the event")
}
