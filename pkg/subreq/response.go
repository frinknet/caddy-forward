package subreq

import "net/http"

// Response is what every sub-request returns to a template or handler.
type Response struct {
	Body    string
	Status  int
	Headers http.Header
	Err     error
}

// String satisfies fmt.Stringer so `{{ GET ... }}` prints the body.
func (r Response) String() string {
	return r.Body
}