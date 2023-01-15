package internal

import (
	"os"
	"testing"
)

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func TestCreateLogger(t *testing.T) {
	const filename = "/tmp/create-logger.txt"
	defer os.Remove(filename)

	tl, err := NewTransactionLogger(filename)

	if tl == nil {
		t.Error("Logger is nil?")
	}

	if err != nil {
		// (*testing.common).Errorf does not support error-wrapping directive %w
		// t.Errorf("Got error: %w", err)
		// https://go.dev/blog/go1.13-errors#TOC_3.3.
		t.Errorf("Got error: %v", err)
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
