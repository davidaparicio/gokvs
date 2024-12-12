package internal

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"sync"
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

func NewTransactionLogger(filename string) (*TransactionLog, error) {
	var err error
	var l TransactionLog = TransactionLog{wg: &sync.WaitGroup{}}

	// Open the transaction log file for reading and writing.
	// Any writes to this file (created if not exist) will append/no overwrite
	// #XXnosec [G304] [-- Acceptable risk, for the CWE-22]
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
