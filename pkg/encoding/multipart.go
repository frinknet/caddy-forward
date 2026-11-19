package encoding

import (
	"bytes"
	"fmt"
	"mime/multipart"
)

// HasBinary reports whether any value in the dict is []byte.
func HasBinary(d map[string]any) bool {
	for _, v := range d {
		if _, ok := v.([]byte); ok {
			return true
		}
	}
	return false
}

// EncodeMultipart encodes a dict as multipart/form-data.
// Returns the body bytes and the full Content-Type header value (with boundary).
func EncodeMultipart(d map[string]any) ([]byte, string, error) {
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)

	for k, v := range d {
		switch val := v.(type) {
		case []byte:
			fw, err := w.CreateFormFile(k, k)
			if err != nil {
				return nil, "", err
			}
			if _, err := fw.Write(val); err != nil {
				return nil, "", err
			}
		default:
			if err := w.WriteField(k, fmt.Sprintf("%v", v)); err != nil {
				return nil, "", err
			}
		}
	}

	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}
