package internal

import (
	"errors"
	"sync"
)

// var store = make(map[string]string)
var store = struct {
	sync.RWMutex
	m map[string]string
}{m: make(map[string]string)}

var ErrorNoSuchKey = errors.New("no such key")

func Get(key string) (string, error) {
	store.RLock()
	value, ok := store.m[key]
	store.RUnlock()

	if !ok {
		return "", ErrorNoSuchKey
	}

	return value, nil
}

func Put(key string, value string) error {
	store.Lock()
	store.m[key] = value
	store.Unlock()
	return nil
}

func Delete(key string) error {
	store.Lock()
	delete(store.m, key)
	store.Unlock()
	return nil
}
