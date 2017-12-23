package main

import (
	"net/http"
	"sync"
	"time"
)

type lastActivityHTTPHandler struct {
	h http.Handler

	duration   time.Duration
	timer      *time.Timer
	timerMutex sync.Mutex
}

func (h *lastActivityHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.timerMutex.Lock()
	h.timer.Reset(h.duration)
	h.timerMutex.Unlock()
	h.h.ServeHTTP(w, r)
}

func newLastActivityHTTPHandler(h http.Handler, d time.Duration) *lastActivityHTTPHandler {
	if h == nil {
		h = http.DefaultServeMux
	}
	return &lastActivityHTTPHandler{
		h:        h,
		duration: d,
		timer:    time.NewTimer(d),
	}
}
