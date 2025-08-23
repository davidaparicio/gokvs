package internal

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

type EventType byte

const (
	_                     = iota // iota == 0; ignore this value
	EventDelete EventType = iota // iota == 1
	EventPut                     // iota == 2; implicitly repeat last
)

type Event struct {
	Sequence  uint64
	EventType EventType
	Key       string
	Value     string
}

type TransactionLogger interface {
	WriteDelete(key string)
	WritePut(key, value string)
	ReadEvents() (<-chan Event, <-chan error)
	Run()
	Wait()
	Close() error
	Err() <-chan error
}

type TransactionLog struct { // implements TransactionLogger
	events       chan<- Event // Write-only channel for sending events
	errors       <-chan error
	lastSequence uint64   // The last used event sequence number
	file         *os.File // The location of the transaction log
	wg           *sync.WaitGroup
}

func (l *TransactionLog) WritePut(key, value string) {
	l.wg.Add(1)
	l.events <- Event{EventType: EventPut, Key: key, Value: url.QueryEscape(value)}
}

func (l *TransactionLog) WriteDelete(key string) {
	l.wg.Add(1)
	l.events <- Event{EventType: EventDelete, Key: key}
}

func (l *TransactionLog) Err() <-chan error {
	return l.errors
}

func NewTransactionLogger(filename string) (TransactionLogger, error) {
	var err error
	var l = TransactionLog{wg: &sync.WaitGroup{}}

	// Open the transaction log file for reading and writing.
	// Any writes to this file (created if not exist) will append/no overwrite
	// #nosec [G304] [-- Acceptable risk, for the CWE-22]
	l.file, err = os.OpenFile(filename, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return nil, fmt.Errorf("cannot open transaction log file: %w", err)
	}

	return &l, nil
}

func (l *TransactionLog) Run() {
	events := make(chan Event, 16)
	l.events = events

	errors := make(chan error, 1)
	l.errors = errors

	// Start retrieving events from the events channel and writing them
	// to the transaction log
	go func() {
		for e := range events {
			l.lastSequence++

			//Write the event to the log
			_, err := fmt.Fprintf(
				l.file,
				"%d\t%d\t%s\t%s\n",
				l.lastSequence, e.EventType, e.Key, e.Value)

			if err != nil {
				errors <- fmt.Errorf("cannot write to log file: %w", err)
			}

			l.wg.Done()
		}
	}()
}

func (l *TransactionLog) Wait() {
	l.wg.Wait()
}

func (l *TransactionLog) Close() error {
	l.wg.Wait()

	if l.events != nil {
		close(l.events) // Terminates Run loop and goroutine
	}

	return l.file.Close()
}

func (l *TransactionLog) ReadEvents() (<-chan Event, <-chan error) {
	scanner := bufio.NewScanner(l.file)
	outEvent := make(chan Event)
	outError := make(chan error, 1)

	go func() {
		var e Event

		defer close(outEvent)
		defer close(outError)

		// Seek to start of file
		if _, err := l.file.Seek(0, 0); err != nil {
			outError <- fmt.Errorf("failed to seek to start of file: %w", err)
			return
		}

		for scanner.Scan() {
			line := scanner.Text()

			n, err := fmt.Sscanf(
				line, "%d\t%d\t%s\t%s",
				&e.Sequence, &e.EventType, &e.Key, &e.Value)

			if (err != nil) && (err != io.EOF) {
				// https://github.com/golang/go/issues/16563
				// https://go.dev/play/p/3kOqJKusGhz
				//log.Printf("Scanner error, failure in fmt.Sscanf: %v", err)
				outError <- fmt.Errorf("input parse error: %w", err)
				return
			}

			// Sanity check ! All lines must have 4 fields
			if err == nil && n < 4 {
				outError <- fmt.Errorf("input wrong number parsed: %w", err)
				return
			}

			// Sanity check ! Are the sequence numbers in increasing order?
			if l.lastSequence >= e.Sequence {
				outError <- fmt.Errorf("transaction numbers out of sequence")
				return
			}

			uv, err := url.QueryUnescape(e.Value)
			if err != nil {
				outError <- fmt.Errorf("value decoding failure: %w", err)
				return
			}

			e.Value = uv
			l.lastSequence = e.Sequence // Update last used sequence #

			outEvent <- e // Send the event along
		}

		if err := scanner.Err(); err != nil {
			outError <- fmt.Errorf("transaction log read failure: %w", err)
			return
		}
	}()

	return outEvent, outError
}

// SQLiteTransactionLogger implements TransactionLogger using SQLite database
type SQLiteTransactionLogger struct {
	db           *sql.DB
	events       chan<- Event // Write-only channel for sending events
	errors       <-chan error
	lastSequence uint64 // The last used event sequence number
	dbPath       string // Path to the SQLite database file
	wg           *sync.WaitGroup
}

// NewSQLiteTransactionLogger creates a new SQLite-based transaction logger
func NewSQLiteTransactionLogger(dbPath string) (*SQLiteTransactionLogger, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_sync=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("cannot open SQLite database: %w", err)
	}

	// Configure connection pool for SQLite
	db.SetMaxOpenConns(1) // SQLite works best with single connection
	db.SetMaxIdleConns(1)

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("cannot ping SQLite database: %w", err)
	}

	logger := &SQLiteTransactionLogger{
		db:     db,
		dbPath: dbPath,
		wg:     &sync.WaitGroup{},
	}

	// Initialize database schema
	if err := logger.initializeSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize database schema: %w", err)
	}

	// Get the last sequence number from the database
	if err := logger.loadLastSequence(); err != nil {
		return nil, fmt.Errorf("failed to load last sequence: %w", err)
	}

	return logger, nil
}

// initializeSchema creates the necessary tables and indexes
func (l *SQLiteTransactionLogger) initializeSchema() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS transaction_events (
			sequence_id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type INTEGER NOT NULL,
			key TEXT NOT NULL,
			value TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sequence_id ON transaction_events(sequence_id)`,
		`CREATE INDEX IF NOT EXISTS idx_key ON transaction_events(key)`,
	}

	for _, query := range queries {
		_, err := l.db.Exec(query)
		if err != nil {
			return fmt.Errorf("failed to execute schema query: %w", err)
		}
	}

	return nil
}

// loadLastSequence retrieves the highest sequence number from the database
func (l *SQLiteTransactionLogger) loadLastSequence() error {
	var lastSeq sql.NullInt64
	err := l.db.QueryRow("SELECT MAX(sequence_id) FROM transaction_events").Scan(&lastSeq)
	if err != nil {
		return fmt.Errorf("failed to query last sequence: %w", err)
	}

	if lastSeq.Valid {
		l.lastSequence = uint64(lastSeq.Int64)
	} else {
		l.lastSequence = 0
	}

	return nil
}

// WritePut implements TransactionLogger interface for PUT operations
func (l *SQLiteTransactionLogger) WritePut(key, value string) {
	l.wg.Add(1)
	l.events <- Event{EventType: EventPut, Key: key, Value: url.QueryEscape(value)}
}

// WriteDelete implements TransactionLogger interface for DELETE operations
func (l *SQLiteTransactionLogger) WriteDelete(key string) {
	l.wg.Add(1)
	l.events <- Event{EventType: EventDelete, Key: key}
}

// Err returns the error channel for monitoring transaction errors
func (l *SQLiteTransactionLogger) Err() <-chan error {
	return l.errors
}

// Run starts the SQLite transaction logger goroutine
func (l *SQLiteTransactionLogger) Run() {
	events := make(chan Event, 16)
	l.events = events

	errors := make(chan error, 1)
	l.errors = errors

	// Start retrieving events from the events channel and writing them to SQLite
	go func() {
		// Prepare the INSERT statement for better performance
		stmt, err := l.db.Prepare("INSERT INTO transaction_events (event_type, key, value) VALUES (?, ?, ?)")
		if err != nil {
			errors <- fmt.Errorf("failed to prepare insert statement: %w", err)
			return
		}
		defer stmt.Close()

		for e := range events {
			// Insert the event into the database
			result, err := stmt.Exec(e.EventType, e.Key, e.Value)
			if err != nil {
				errors <- fmt.Errorf("cannot write to SQLite database: %w", err)
				l.wg.Done()
				continue
			}

			// Update the last sequence number
			seqID, err := result.LastInsertId()
			if err != nil {
				errors <- fmt.Errorf("failed to get last insert ID: %w", err)
			} else {
				l.lastSequence = uint64(seqID)
			}

			l.wg.Done()
		}
	}()
}

// Wait blocks until all pending transactions are written
func (l *SQLiteTransactionLogger) Wait() {
	l.wg.Wait()
}

// Close closes the SQLite transaction logger
func (l *SQLiteTransactionLogger) Close() error {
	l.wg.Wait()

	if l.events != nil {
		close(l.events) // Terminates Run loop and goroutine
	}

	return l.db.Close()
}

// ReadEvents reads all events from the SQLite database
func (l *SQLiteTransactionLogger) ReadEvents() (<-chan Event, <-chan error) {
	outEvent := make(chan Event)
	outError := make(chan error, 1)

	go func() {
		defer close(outEvent)
		defer close(outError)

		// Query all events in sequence order
		rows, err := l.db.Query(`
			SELECT sequence_id, event_type, key, COALESCE(value, '') as value 
			FROM transaction_events 
			ORDER BY sequence_id ASC
		`)
		if err != nil {
			outError <- fmt.Errorf("failed to query events: %w", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var e Event
			var eventType int
			var value string

			err := rows.Scan(&e.Sequence, &eventType, &e.Key, &value)
			if err != nil {
				outError <- fmt.Errorf("failed to scan event row: %w", err)
				return
			}

			e.EventType = EventType(eventType)

			// URL decode the value
			uv, err := url.QueryUnescape(value)
			if err != nil {
				outError <- fmt.Errorf("value decoding failure: %w", err)
				return
			}
			e.Value = uv

			// Update last sequence number
			l.lastSequence = e.Sequence

			outEvent <- e
		}

		if err := rows.Err(); err != nil {
			outError <- fmt.Errorf("SQLite transaction log read failure: %w", err)
			return
		}
	}()

	return outEvent, outError
}

// CheckDatabaseIntegrity performs an integrity check on the SQLite database
func (l *SQLiteTransactionLogger) CheckDatabaseIntegrity() error {
	var result string
	err := l.db.QueryRow("PRAGMA integrity_check").Scan(&result)
	if err != nil {
		return fmt.Errorf("failed to run integrity check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("database integrity check failed: %s", result)
	}
	return nil
}

// GetEventCount returns the total number of events in the database
func (l *SQLiteTransactionLogger) GetEventCount() (int64, error) {
	var count int64
	err := l.db.QueryRow("SELECT COUNT(*) FROM transaction_events").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count events: %w", err)
	}
	return count, nil
}

// LoggerConfig holds configuration options for transaction loggers
type LoggerConfig struct {
	Type            string // "file" or "sqlite"
	FilePath        string // For file logger
	DBPath          string // For SQLite logger
	MigrateFromFile bool   // Auto-migrate from file
}

// MigrateFileToSQLite migrates transaction events from a file-based log to SQLite
func MigrateFileToSQLite(logFile, dbPath string) (*SQLiteTransactionLogger, error) {
	// Check if the file exists and has content
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		// No file to migrate, just create a new SQLite logger
		return NewSQLiteTransactionLogger(dbPath)
	}

	// Create file logger to read existing events
	fileLogger, err := NewTransactionLogger(logFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create file logger for migration: %w", err)
	}
	defer fileLogger.Close()

	// Create SQLite logger
	sqliteLogger, err := NewSQLiteTransactionLogger(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create SQLite logger: %w", err)
	}

	// Check if SQLite database already has events
	count, err := sqliteLogger.GetEventCount()
	if err != nil {
		return nil, fmt.Errorf("failed to check existing events: %w", err)
	}
	if count > 0 {
		// Database already has events, skip migration
		return sqliteLogger, nil
	}

	// Start the SQLite logger to accept events
	sqliteLogger.Run()
	defer sqliteLogger.Wait()

	// Read events from file and write to SQLite
	events, errors := fileLogger.ReadEvents()
	migratedCount := 0

	for {
		select {
		case event, ok := <-events:
			if !ok {
				// Events channel closed, migration complete
				goto migrationComplete
			}

			// Write event to SQLite
			switch event.EventType {
			case EventPut:
				sqliteLogger.WritePut(event.Key, event.Value)
			case EventDelete:
				sqliteLogger.WriteDelete(event.Key)
			}
			migratedCount++

		case err, ok := <-errors:
			if !ok {
				// Errors channel closed
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("migration failed while reading file: %w", err)
			}
		}
	}

migrationComplete:
	// Wait for all writes to complete
	sqliteLogger.Wait()

	// Archive the original file
	archiveFile := logFile + ".migrated." + fmt.Sprintf("%d", os.Getpid())
	if err := os.Rename(logFile, archiveFile); err != nil {
		// If rename fails, it's not critical, just log a warning
		// In production, you might want to handle this differently
		fmt.Printf("Warning: failed to archive original log file %s: %v\n", logFile, err)
	}

	fmt.Printf("Migration completed: %d events migrated from %s to %s\n", migratedCount, logFile, dbPath)
	return sqliteLogger, nil
}

// NewTransactionLoggerWithConfig creates a transaction logger based on configuration
func NewTransactionLoggerWithConfig(config LoggerConfig) (TransactionLogger, error) {
	switch config.Type {
	case "sqlite":
		if config.MigrateFromFile && config.FilePath != "" {
			// Migrate from file to SQLite
			return MigrateFileToSQLite(config.FilePath, config.DBPath)
		}
		// Create new SQLite logger
		return NewSQLiteTransactionLogger(config.DBPath)

	case "file":
		// Create file logger
		return NewTransactionLogger(config.FilePath)

	default:
		return nil, fmt.Errorf("unknown logger type: %s", config.Type)
	}
}
