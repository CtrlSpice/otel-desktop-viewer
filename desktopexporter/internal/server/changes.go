package server

import (
	"fmt"
	"net/http"
	"time"
)

// Every connection starts with a snapshot, including reconnects. Revisions are
// process-local invalidations, not a durable event log or import receipts.
func (s *Server) changesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	response := http.NewResponseController(w)
	write := func(message string) error {
		if err := response.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
			return err
		}
		if _, err := fmt.Fprint(w, message); err != nil {
			return err
		}
		if err := response.Flush(); err != nil {
			return err
		}
		return response.SetWriteDeadline(time.Time{})
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		revision, changed, closed := s.store.CommitSnapshot()
		if closed {
			return
		}
		if err := write(fmt.Sprintf("event: change\ndata: %d\n\n", revision)); err != nil {
			return
		}
		waiting := true
		for waiting {
			select {
			case <-r.Context().Done():
				return
			case <-s.streamDone:
				return
			case <-changed:
				waiting = false
			case <-heartbeat.C:
				if err := write(": keepalive\n\n"); err != nil {
					return
				}
			}
		}
		// Coalesce ingestion bursts outside the transaction callback. The next
		// snapshot includes all writes that arrive during this interval.
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-r.Context().Done():
			timer.Stop()
			return
		case <-s.streamDone:
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
