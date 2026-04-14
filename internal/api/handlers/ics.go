package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/domain/events"
	ical "github.com/Togather-Foundation/server/internal/ical"
)

type ICSHandler struct {
	Service *events.Service
	Env     string
	BaseURL string
	Loc     *time.Location
}

func NewICSHandler(service *events.Service, env, baseURL string) *ICSHandler {
	return &ICSHandler{
		Service: service,
		Env:     env,
		BaseURL: baseURL,
	}
}

func (h *ICSHandler) FeedHandler(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	filters, pagination, _, err := events.ParseFilters(r.URL.Query(), h.Loc)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}

	result, err := h.Service.List(r.Context(), filters, pagination)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	serializeResult, err := ical.SerializeEvents(result.Events, ical.SerializeOptions{
		CalendarName: "Togather Events",
	})
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"events.ics\"")

	if result.NextCursor != "" {
		nextURL := h.BaseURL + "/api/v1/events.ics?after=" + result.NextCursor
		w.Header().Set("Link", "<"+nextURL+">; rel=\"next\"")
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(serializeResult.Data)
}

func (h *ICSHandler) SingleEventHandler(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	ulidValue, ok := ValidateAndExtractULID(w, r, "id", h.Env)
	if !ok {
		return
	}

	item, err := h.Service.GetByULID(r.Context(), ulidValue)
	if err != nil {
		if errors.Is(err, events.ErrNotFound) {
			tombstone, tombErr := h.Service.GetTombstoneByEventULID(r.Context(), ulidValue)
			if tombErr == nil && tombstone != nil {
				WriteTombstoneResponse(w, r, tombstone.Payload, tombstone.DeletedAt, "Event")
				return
			}
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	if item.LifecycleState == "deleted" {
		tombstone, tombErr := h.Service.GetTombstoneByEventULID(r.Context(), ulidValue)
		if tombErr == nil && tombstone != nil {
			WriteTombstoneResponse(w, r, tombstone.Payload, tombstone.DeletedAt, "Event")
			return
		}

		payload := map[string]any{
			"@context":      loadDefaultContext(),
			"@type":         "Event",
			"sel:tombstone": true,
			"sel:deletedAt": time.Now().Format(time.RFC3339),
		}
		writeJSON(w, http.StatusGone, payload, contentTypeFromRequest(r))
		return
	}

	serializeResult, err := ical.SerializeSingleEvent(*item, ical.SerializeOptions{
		CalendarName: "Togather Events",
	})
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"event-"+ulidValue+".ics\"")

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(serializeResult.Data)
}
