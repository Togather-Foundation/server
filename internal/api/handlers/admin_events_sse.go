package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/riverqueue/river"
	"github.com/rs/zerolog/log"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/jobs"
	"github.com/Togather-Foundation/server/internal/sse"
)

// AdminEventsSSEHandler streams River job events to SSE clients.
// GET /api/v1/admin/scraper/events — protected by adminCookieAuth middleware.
type AdminEventsSSEHandler struct {
	Broker *sse.Broker
	Env    string
}

func (h *AdminEventsSSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		problem.Write(w, r, http.StatusInternalServerError,
			"https://sel.events/problems/server-error",
			"Streaming not supported", nil, h.Env)
		return
	}

	// Disable write deadline for this long-lived SSE connection (Go 1.20+)
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Tell browser to reconnect after 5s if disconnected
	if _, err := fmt.Fprintf(w, "retry: 5000\n\n"); err != nil {
		return
	}
	flusher.Flush()

	// Subscribe to job-terminal kinds. The upstream River subscription in router.go
	// already limits to these three, but we pass them explicitly so this handler's
	// intent is self-documenting and robust to future upstream changes.
	ch, cancel := h.Broker.Subscribe(
		river.EventKindJobCompleted,
		river.EventKindJobFailed,
		river.EventKindJobCancelled,
	)
	defer cancel()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event, ok := <-ch:
			if !ok || event == nil {
				return // broker shut down
			}
			// Only forward scrape_source jobs
			if event.Job == nil || event.Job.Kind != jobs.JobKindScrapeSource {
				continue
			}
			var args jobs.ScrapeSourceArgs
			if err := json.Unmarshal(event.Job.EncodedArgs, &args); err != nil {
				log.Warn().Err(err).Int64("job_id", event.Job.ID).Msg("admin SSE: failed to unmarshal job args")
				// args is zero value; source_name will be "" in the payload
			}
			payload, _ := json.Marshal(map[string]any{
				"kind":        string(event.Kind),
				"job_kind":    event.Job.Kind,
				"source_name": args.SourceName,
				"job_id":      event.Job.ID,
			})
			if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", event.Job.ID, payload); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
