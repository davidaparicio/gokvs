package internal

import (
	"errors"
	"testing"
)

func TestGet(t *testing.T) {
	const key = "read-key"
	const value = "read-value"

	var val interface{}
	var err error

	defer delete(store.m, key)

	// Read a non-thing
	val, err = Get(key) //nolint:ineffassign
	if err == nil {
		t.Error("expected an error: ", err)
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

	if err := Delete(key); err != nil {
		t.Error("Delete returns an error: ", err)
	}

	_, contains = store.m[key]
	if contains {
		t.Error("Delete failed")
	}
}

func TestPutAndGet(t *testing.T) {
	// Clear the store before testing
	store.Lock()
	store.m = make(map[string]string)
	store.Unlock()

	tests := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		{
			name:    "basic put and get",
			key:     "test-key",
			value:   "test-value",
			wantErr: false,
		},
		{
			name:    "empty value",
			key:     "empty",
			value:   "",
			wantErr: false,
		},
		{
			name:    "special characters",
			key:     "special!@#$",
			value:   "value!@#$",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Put
			if err := Put(tt.key, tt.value); (err != nil) != tt.wantErr {
				t.Errorf("Put() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Test Get
			got, err := Get(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.value {
				t.Errorf("Get() got = %v, want %v", got, tt.value)
			}
		})
	}
}

func TestGetNonExistentKey(t *testing.T) {
	store.Lock()
	store.m = make(map[string]string)
	store.Unlock()

	_, err := Get("non-existent-key")
	if err != ErrorNoSuchKey {
		t.Errorf("Get() error = %v, want %v", err, ErrorNoSuchKey)
	}
}

func BenchmarkGet(b *testing.B) {
	const key = "read-key"
	const value = "read-value"
	store.m[key] = value
	var err error

	for i := 0; i < b.N; i++ {
		if _, err = Get(key); err != nil {
			b.Error("Get returns an error: ", err)
		}
	}
}

func BenchmarkGet_BigInputs(b *testing.B) {
	keys := []string{"", "bar", "eye", "foo"}
	values := []string{"empty", "beer", "glasses", "bar"}
	var err error

	for i, key := range keys {
		store.m[key] = values[i]
	}

	for i := 0; i < b.N; i++ {
		for _, key := range keys {
			if _, err = Get(key); err != nil {
				b.Error("Get returns an error: ", err)
			}
		}
	}
}

func FuzzGet(f *testing.F) {
	var val string
	var err error
	f.Add("kayak")
	f.Fuzz(func(t *testing.T, str string) {
		if err = Put("fuzz", str); err != nil {
			t.Error("Get returns an error: ", err)
		}
		val, err = Get("fuzz")
		if err != nil {
			t.Error("Get returns an error: ", err)
		}
		if val != str {
			t.Fail()
		}
	})
}
