package proofs

import "sync"

// notifier wakes a connector's long-polling GET /connector/tasks/next as
// soon as a proof is enqueued for its agent, instead of making it wait for
// the next once-a-second (now once-per-fallback-interval) database poll.
// It is purely in-process: with several api replicas behind a load
// balancer, a poller only wakes early when the enqueue happened to land on
// the same replica. That is fine — the fallback poll in http_connector.go
// still covers every other case, including a multi-instance deployment.
type notifier struct {
	mu   sync.Mutex
	subs map[string][]chan struct{}
}

func newNotifier() *notifier { return &notifier{subs: map[string][]chan struct{}{}} }

// wait subscribes to wake-ups for agentID. The returned channel receives a
// value (never closed) each time notify(agentID) is called while it is
// still subscribed; cancel must be called exactly once, typically deferred,
// to unsubscribe and let the map entry be garbage collected.
func (n *notifier) wait(agentID string) (ch chan struct{}, cancel func()) {
	ch = make(chan struct{}, 1)
	n.mu.Lock()
	n.subs[agentID] = append(n.subs[agentID], ch)
	n.mu.Unlock()
	cancel = func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		list := n.subs[agentID]
		for i, c := range list {
			if c == ch {
				n.subs[agentID] = append(list[:i], list[i+1:]...)
				break
			}
		}
		if len(n.subs[agentID]) == 0 {
			delete(n.subs, agentID)
		}
	}
	return ch, cancel
}

// notify wakes every current subscriber for agentID. It never blocks: a
// subscriber that is not ready to receive (its buffered slot already full,
// or it raced past its select) simply polls on its own fallback timer
// instead.
func (n *notifier) notify(agentID string) {
	n.mu.Lock()
	subs := append([]chan struct{}{}, n.subs[agentID]...)
	n.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
