package subreq

import (
	"net/http"
	"time"
)

// Requester performs sub-requests with shared client and size limit.
type Requester struct {
	Client          *http.Client
	MaxResponseSize int64
}

// NewRequester builds a Requester from timeout and max response size.
func NewRequester(timeout time.Duration, maxSize int64) *Requester {
	return &Requester{
		Client:          &http.Client{Timeout: timeout},
		MaxResponseSize: maxSize,
	}
}
