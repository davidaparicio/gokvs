package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotifierKeyScoping(t *testing.T) {
	n := NewNotifier()

	scoped, cancelScoped := n.Subscribe("a")
	defer cancelScoped()
	all, cancelAll := n.Subscribe("")
	defer cancelAll()

	n.Publish(NotifyEvent{Type: EventPut, Key: "a", Value: "1"})
	n.Publish(NotifyEvent{Type: EventPut, Key: "b", Value: "2"})

	e := <-scoped
	assert.Equal(t, "a", e.Key)
	select {
	case e := <-scoped:
		t.Fatalf("scoped watcher received event for other key: %+v", e)
	default:
	}

	assert.Equal(t, "a", (<-all).Key)
	assert.Equal(t, "b", (<-all).Key)
}

func TestNotifierCancel(t *testing.T) {
	n := NewNotifier()

	ch, cancel := n.Subscribe("")
	require.Equal(t, 1, n.WatcherCount())

	cancel()
	cancel() // idempotent
	assert.Equal(t, 0, n.WatcherCount())

	_, open := <-ch
	assert.False(t, open, "channel must be closed after cancel")

	n.Publish(NotifyEvent{Type: EventPut, Key: "x"}) // must not panic
}

func TestNotifierDropsOldestWhenSlow(t *testing.T) {
	n := NewNotifier()

	ch, cancel := n.Subscribe("")
	defer cancel()

	// Overflow the buffer; Publish must not block and the oldest
	// events must be discarded first.
	for i := 0; i < watcherBuffer+5; i++ {
		n.Publish(NotifyEvent{Type: EventPut, Key: "k", Value: string(rune('a' + i))})
	}

	require.Len(t, ch, watcherBuffer)
	first := <-ch
	assert.Equal(t, string(rune('a'+5)), first.Value, "oldest events should have been dropped")
}
