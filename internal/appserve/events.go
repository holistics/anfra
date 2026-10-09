package appserve

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/holistics/anfra/shared/jsonkit"
)

// Event is what a live-reloading frontend hears: Data App definitions (or files next to them)
// changed, by their paths under apps/; or the AML changed, so datasets and problems may have.
type Event struct {
	Type  string   `json:"type" enum:"apps,aml"`
	Paths []string `json:"paths,omitempty"`
}

// broadcaster fans events out to every open event stream.
type broadcaster struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func newBroadcaster() *broadcaster { return &broadcaster{subs: map[chan Event]struct{}{}} }

func (b *broadcaster) publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default: // a stalled stream misses an event rather than blocking everyone
		}
	}
}

func (b *broadcaster) closeAll() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		close(ch)
		delete(b.subs, ch)
	}
}

// serve streams events (Server-Sent Events) until the client goes or the server closes.
func (b *broadcaster) serve(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil || rc.Flush() != nil {
		return
	}

	ch := make(chan Event, 16)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case e, open := <-ch:
			if !open {
				return
			}
			data, _ := jsonkit.Marshal(e)
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}
