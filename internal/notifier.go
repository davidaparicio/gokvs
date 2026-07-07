package internal

import "sync"

// NotifyEvent is a change notification pushed to watchers.
type NotifyEvent struct {
	Type  EventType // EventPut or EventDelete
	Key   string
	Value string
}

// watcher is a single subscription: a key filter and a delivery channel.
type watcher struct {
	key string // "" means all keys
	ch  chan NotifyEvent
}

// Notifier fans out change notifications to key-scoped subscribers.
// A subscriber that falls behind has the oldest pending event dropped
// rather than blocking writers.
type Notifier struct {
	mu       sync.RWMutex
	watchers map[*watcher]struct{}
}

const watcherBuffer = 16

func NewNotifier() *Notifier {
	return &Notifier{watchers: make(map[*watcher]struct{})}
}

// Subscribe registers interest in changes to key ("" for all keys).
// It returns the event channel and a cancel function that must be called
// to release the subscription. The channel is closed on cancel.
func (n *Notifier) Subscribe(key string) (<-chan NotifyEvent, func()) {
	w := &watcher{key: key, ch: make(chan NotifyEvent, watcherBuffer)}

	n.mu.Lock()
	n.watchers[w] = struct{}{}
	n.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			n.mu.Lock()
			delete(n.watchers, w)
			n.mu.Unlock()
			close(w.ch)
		})
	}
	return w.ch, cancel
}

// WatcherCount reports the number of active subscriptions.
func (n *Notifier) WatcherCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.watchers)
}

// Publish delivers an event to every matching subscriber without blocking:
// if a subscriber's buffer is full, its oldest event is discarded.
func (n *Notifier) Publish(e NotifyEvent) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	for w := range n.watchers {
		if w.key != "" && w.key != e.Key {
			continue
		}
		for {
			select {
			case w.ch <- e:
			default:
				// Buffer full: drop the oldest event and retry.
				select {
				case <-w.ch:
				default:
				}
				continue
			}
			break
		}
	}
}
