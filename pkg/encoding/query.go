package encoding

import (
	"fmt"
	"net/url"
)

// MergeQuery appends incoming to existing with `&`. Both are used verbatim.
func MergeQuery(existing, incoming string) string {
	if existing == "" {
		return incoming
	}
	if incoming == "" {
		return existing
	}
	return existing + "&" + incoming
}

// MergeQueryDict parses existing as a query string, adds keys from d, and
// re-encodes. Key order is normalized and both are URL-escaped.
func MergeQueryDict(existing string, d map[string]any) (string, error) {
	vals, err := url.ParseQuery(existing)
	if err != nil {
		return "", err
	}
	for k, v := range d {
		vals.Set(k, fmt.Sprintf("%v", v))
	}
	return vals.Encode(), nil
}