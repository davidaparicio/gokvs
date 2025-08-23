package helpers

import (
	"fmt"
	"os"
	"testing"

	"github.com/davidaparicio/gokvs/internal"
)

// StorageHelper provides utilities for testing storage operations
type StorageHelper struct {
	t         *testing.T
	TempFiles []string
}

// NewStorageHelper creates a new storage helper
func NewStorageHelper(t *testing.T) *StorageHelper {
	return &StorageHelper{
		t:         t,
		TempFiles: make([]string, 0),
	}
}

// CreateTempTransactionLogger creates a temporary transaction logger for testing
func (sh *StorageHelper) CreateTempTransactionLogger() (internal.TransactionLogger, string, error) {
	tmpFile, err := os.CreateTemp("", "test-transaction-*.log")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temp file: %w", err)
	}
	
	filename := tmpFile.Name()
	tmpFile.Close()
	sh.TempFiles = append(sh.TempFiles, filename)

	logger, err := internal.NewTransactionLogger(filename)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create transaction logger: %w", err)
	}

	return logger, filename, nil
}

// CreateCorruptedLogFile creates a corrupted transaction log file for testing error handling
func (sh *StorageHelper) CreateCorruptedLogFile() (string, error) {
	tmpFile, err := os.CreateTemp("", "test-corrupted-*.log")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	
	filename := tmpFile.Name()
	sh.TempFiles = append(sh.TempFiles, filename)

	// Write some corrupted data
	corruptedData := []byte("corrupted\ninvalid\ndata\nhere\n")
	if _, err := tmpFile.Write(corruptedData); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write corrupted data: %w", err)
	}
	
	tmpFile.Close()
	return filename, nil
}

// CreateLargeLogFile creates a large transaction log file for performance testing
func (sh *StorageHelper) CreateLargeLogFile(numEntries int) (string, error) {
	logger, filename, err := sh.CreateTempTransactionLogger()
	if err != nil {
		return "", err
	}

	logger.Run()
	defer logger.Close()

	// Write many entries
	for i := 0; i < numEntries; i++ {
		key := fmt.Sprintf("key_%d", i)
		value := fmt.Sprintf("value_%d", i)
		logger.WritePut(key, value)
	}

	logger.Wait()
	return filename, nil
}

// PopulateStore populates the in-memory store with test data
func (sh *StorageHelper) PopulateStore(data map[string]string) error {
	for key, value := range data {
		if err := internal.Put(key, value); err != nil {
			return fmt.Errorf("failed to put %s=%s: %w", key, value, err)
		}
	}
	return nil
}

// ClearStore clears the in-memory store
func (sh *StorageHelper) ClearStore() {
	// This would require access to the internal store, 
	// which we'll handle in the enhanced core tests
}

// GetStoreSize returns the current size of the store
func (sh *StorageHelper) GetStoreSize() int {
	// This would require access to the internal store
	// For now, we'll return -1 to indicate not implemented
	return -1
}

// VerifyStoreContents verifies that the store contains expected data
func (sh *StorageHelper) VerifyStoreContents(expected map[string]string) error {
	for key, expectedValue := range expected {
		actualValue, err := internal.Get(key)
		if err != nil {
			return fmt.Errorf("key %s not found: %w", key, err)
		}
		if actualValue != expectedValue {
			return fmt.Errorf("key %s: expected %s, got %s", key, expectedValue, actualValue)
		}
	}
	return nil
}

// CreateTestDataSet creates a standard test dataset
func (sh *StorageHelper) CreateTestDataSet(size int) map[string]string {
	data := make(map[string]string)
	for i := 0; i < size; i++ {
		key := fmt.Sprintf("test_key_%d", i)
		value := fmt.Sprintf("test_value_%d", i)
		data[key] = value
	}
	return data
}

// CreateLargeDataSet creates a large dataset for performance testing
func (sh *StorageHelper) CreateLargeDataSet(keySize, valueSize, count int) map[string]string {
	data := make(map[string]string)
	
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("key_%0*d", keySize-10, i)
		// Pad value to desired size
		baseValue := fmt.Sprintf("value_%d", i)
		padding := valueSize - len(baseValue)
		if padding > 0 {
			for j := 0; j < padding; j++ {
				baseValue += "x"
			}
		}
		data[key] = baseValue
	}
	
	return data
}

// Cleanup removes all temporary files created by this helper
func (sh *StorageHelper) Cleanup() {
	for _, file := range sh.TempFiles {
		if err := os.Remove(file); err != nil {
			sh.t.Logf("Failed to remove temporary file %s: %v", file, err)
		}
	}
}