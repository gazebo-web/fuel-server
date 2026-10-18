package aibuilder

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"
)

// Background turns (design §3.2.2): a turn runs on its own context, not the
// request's, and writes its events to ai_chat_events. A browser follows a
// turn by reading those events, so closing the tab or losing the network
// doesn't stop the agent, and the page can pick the stream up again. Only
// the cancel endpoint stops a turn.

const (
	// Text arrives a few characters at a time; it is stored in pieces of
	// at most textFlushBytes, or every textFlushEvery.
	textFlushBytes = 2048
	textFlushEvery = 150 * time.Millisecond
	// Followers poll the database this often when not woken sooner (a
	// turn running in another process wakes no one here).
	followPoll = time.Second
	// A comment line keeps idle streams open through proxies.
	followHeartbeat = 15 * time.Second
	// A turn's own deadline.
	turnTimeout = 30 * time.Minute
)

type turnRunner struct {
	store *Store

	mu      sync.Mutex
	cancels map[string]context.CancelFunc // running turns in this process
	wake    map[string]chan struct{}      // closed when a turn has new events
}

func newTurnRunner(store *Store) *turnRunner {
	return &turnRunner{store: store, cancels: map[string]context.CancelFunc{}, wake: map[string]chan struct{}{}}
}

// start runs a turn in the background. run sends the engine's events on
// out and closes it when the turn ends.
func (r *turnRunner) start(turn *ChatTurn, run func(ctx context.Context, out chan<- Event)) {
	ctx, cancel := context.WithTimeout(context.Background(), turnTimeout)
	r.mu.Lock()
	r.cancels[turn.ID] = cancel
	r.mu.Unlock()
	out := make(chan Event, 64)
	go run(ctx, out)
	go func() {
		defer cancel()
		status, errText := r.record(ctx, turn.ID, out)
		// Finished before the done event, so whoever sees done (and then
		// reads the history) sees the turn finished. A follower here won't
		// mistake the gap for a turn that died: it's still in cancels.
		if err := r.store.FinishTurn(turn.ID, status, errText); err != nil {
			log.Printf("aibuilder: finish turn %s: %v", turn.ID, err)
		}
		r.append(turn.ID, Event{Type: EventDone, Status: status, Error: errText})
		r.mu.Lock()
		delete(r.cancels, turn.ID)
		r.mu.Unlock()
		r.notify(turn.ID)
	}()
}

// record stores a turn's events as they arrive and returns how it ended.
func (r *turnRunner) record(ctx context.Context, turnID string, out <-chan Event) (status, errText string) {
	var text []byte
	flush := func() {
		if len(text) > 0 {
			r.append(turnID, Event{Type: EventText, Text: string(text)})
			text = text[:0]
		}
	}
	tick := time.NewTicker(textFlushEvery)
	defer tick.Stop()
	status = TurnFailed
	errText = "the assistant stopped without finishing"
	for {
		select {
		case <-tick.C:
			flush()
		case ev, ok := <-out:
			if !ok {
				flush()
				if ctx.Err() != nil && status != TurnDone {
					if errors.Is(ctx.Err(), context.DeadlineExceeded) {
						return TurnFailed, "the turn ran longer than " + turnTimeout.String()
					}
					return TurnCancelled, ""
				}
				return status, errText
			}
			switch ev.Type {
			case EventText:
				text = append(text, ev.Text...)
				if len(text) >= textFlushBytes {
					flush()
				}
				continue
			case EventDone:
				// The runner writes the final done event itself.
				status, errText = TurnDone, ""
				continue
			case EventError:
				if ctx.Err() != nil {
					continue // the cancel or timeout, reported above
				}
				status, errText = TurnFailed, ev.Error
			}
			flush()
			r.append(turnID, ev)
		}
	}
}

func (r *turnRunner) append(turnID string, ev Event) {
	b, _ := json.Marshal(ev)
	if err := r.store.AppendEvent(&ChatEvent{TurnID: turnID, Type: string(ev.Type), Payload: string(b)}); err != nil {
		log.Printf("aibuilder: store event for turn %s: %v", turnID, err)
	}
	r.notify(turnID)
}

func (r *turnRunner) notify(turnID string) {
	r.mu.Lock()
	if ch, ok := r.wake[turnID]; ok {
		close(ch)
		delete(r.wake, turnID)
	}
	r.mu.Unlock()
}

func (r *turnRunner) waitFor(turnID string) <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch, ok := r.wake[turnID]
	if !ok {
		ch = make(chan struct{})
		r.wake[turnID] = ch
	}
	return ch
}

// cancel stops a turn running in this process.
func (r *turnRunner) cancel(turnID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.cancels[turnID]
	if ok {
		c()
	}
	return ok
}

// follow writes a turn's events after the id until its done event, or
// until ctx ends. heartbeat is called when nothing has been written for a
// while.
func (r *turnRunner) follow(ctx context.Context, turnID string, after uint, write func(ChatEvent) error,
	heartbeat func() error) error {
	idle := time.Now()
	for {
		wake := r.waitFor(turnID)
		events, err := r.store.Events(turnID, after)
		if err != nil {
			return err
		}
		for _, e := range events {
			if err := write(e); err != nil {
				return err
			}
			after = e.ID
			idle = time.Now()
			if e.Type == string(EventDone) {
				return nil
			}
		}
		if len(events) == 0 {
			// A turn that finished without a done event (its process died
			// before FailInterrupted ran) would be followed forever.
			if t, err := r.store.turnByID(turnID); err == nil && t.Status != TurnRunning && !r.running(turnID) {
				return write(ChatEvent{ID: after, Type: string(EventDone),
					Payload: mustJSON(Event{Type: EventDone, Status: t.Status, Error: t.Error})})
			}
		}
		if time.Since(idle) > followHeartbeat {
			if err := heartbeat(); err != nil {
				return err
			}
			idle = time.Now()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wake:
		case <-time.After(followPoll):
		}
	}
}

func (r *turnRunner) running(turnID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.cancels[turnID]
	return ok
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
