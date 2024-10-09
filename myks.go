package myks

import (
	"errors"
	"iter"
	"maps"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("entry was not found")
	ErrExpired  = errors.New("entry is expired")
)

// KeyStore is the main structure of this module
type KeyStore[T any] struct {
	mu              sync.RWMutex
	data            map[string]entry[T]
	stopChan        chan struct{}
	cleanupInterval time.Duration
}

// entry is a keystore value, with an optional expiration
type entry[T any] struct {
	value      T
	expiration *time.Time
}

// New creates a new keystore with an optional goroutine to automatically clean expired values
func New[T any](cleanupInterval time.Duration) *KeyStore[T] {
	ks := &KeyStore[T]{ // do not set cleanupInterval here
		data:     make(map[string]entry[T]),
		stopChan: make(chan struct{}),
	}

	go ks.StartCleanup(cleanupInterval)

	return ks
}

// Set saves a key-value pair with a given expiration.
// If duration <= 0, the entry will never expire.
func (ks *KeyStore[T]) Set(key string, value T, duration time.Duration) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	var expiration *time.Time
	if duration > 0 {
		exp := time.Now().Add(duration)
		expiration = &exp
	}

	ks.data[key] = entry[T]{
		value:      value,
		expiration: expiration,
	}
}

// Get returns the value for the given key if it exists and it's not expired.
func (ks *KeyStore[T]) Get(key string) (*T, error) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	ent, exists := ks.data[key]
	if !exists {
		return nil, ErrNotFound
	}

	if ent.expiration != nil && ent.expiration.Before(time.Now()) {
		delete(ks.data, key)
		return nil, ErrExpired
	}

	return &ent.value, nil
}

// Delete removes a key-value pair from the KeyStore.
func (ks *KeyStore[T]) Delete(key string) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	delete(ks.data, key)
}

// Keys returns an iterator for all not-expired keys in the KeyStore.
func (ks *KeyStore[T]) Keys() iter.Seq[string] {
	ks.Clean()

	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return maps.Keys(ks.data)
}

// Clean deletes all expired keys from the keystore.
func (ks *KeyStore[T]) Clean() {
	now := time.Now()
	ks.mu.Lock()
	defer ks.mu.Unlock()

	for k, ent := range ks.data {
		if ent.expiration != nil && ent.expiration.Before(now) {
			delete(ks.data, k)
		}
	}
}

// StartCleanup starts a goroutine that periodically deletes all expired keys from the keystore.
func (ks *KeyStore[T]) StartCleanup(cleanupInterval time.Duration) {
	if cleanupInterval == 0 {
		return
	}

	ks.StopCleanup()
	ks.cleanupInterval = cleanupInterval

	ticker := time.NewTicker(ks.cleanupInterval)
	for {
		select {
		case <-ticker.C:
			ks.Clean()
		case <-ks.stopChan:
			ticker.Stop()
			return
		}
	}
}

// StopCleanup stops the cleanup goroutine.
func (ks *KeyStore[T]) StopCleanup() {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if ks.cleanupInterval != 0 {
		close(ks.stopChan)
		ks.cleanupInterval = 0
	}
}
