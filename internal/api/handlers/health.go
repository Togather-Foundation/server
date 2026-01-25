package handlers

import (
	"encoding/json"
	"net/http"
)

type healthResponse struct {
	Status string `json:"status"`
}

// Healthz returns a lightweight liveness response.
func Healthz() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondHealth(w, http.StatusOK, "ok")
	})
}

// Readyz returns a readiness response. Future dependencies can be checked here.
func Readyz() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondHealth(w, http.StatusOK, "ready")
	})
}

func respondHealth(w http.ResponseWriter, status int, value string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(healthResponse{Status: value})
}
