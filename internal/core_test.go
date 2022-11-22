package internal

import (
	"errors"
	"testing"
)

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

func TestGet(t *testing.T) {
	const key = "read-key"
	const value = "read-value"

	var val interface{}
	var err error

	defer delete(store.m, key)

	// Read a non-thing
	val, err = Get(key)
	if err == nil {
		t.Error("expected an error")
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

	Delete(key)

	_, contains = store.m[key]
	if contains {
		t.Error("Delete failed")
	}
}

func BenchmarkGet(b *testing.B) {
	const key = "read-key"
	const value = "read-value"
	store.m[key] = value

	for i := 0; i < b.N; i++ {
		Get(key)
	}
}

func BenchmarkGet_BigInputs(b *testing.B) {
	keys := []string{"", "bar", "eye", "foo"}
	values := []string{"empty", "beer", "glasses", "bar"}

	for i, key := range keys {
		store.m[key] = values[i]
	}

	for i := 0; i < b.N; i++ {
		for _, key := range keys {
			Get(key)
		}
	}
}

func FuzzGet(f *testing.F) {
	f.Add("kayak")
	f.Fuzz(func(t *testing.T, str string) {
		Put("fuzz", str)
		val, err := Get("fuzz")
		if err != nil {
			t.Error("unexpected error:", err)
		}
		if val != str {
			t.Fail()
		}
	})
}
