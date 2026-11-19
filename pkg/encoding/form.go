package encoding

import (
	"fmt"
	"net/url"
)

// EncodeForm encodes a dict as application/x-www-form-urlencoded.
func EncodeForm(d map[string]any) (string, error) {
	vals := url.Values{}
	for k, v := range d {
		vals.Set(k, fmt.Sprintf("%v", v))
	}
	return vals.Encode(), nil
}
