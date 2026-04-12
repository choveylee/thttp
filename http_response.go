package thttp

import (
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"
)

// Response wraps [http.Response] with helpers to read the body.
type Response struct {
	*http.Response
}

// ToBytes reads and decompresses the response body when Content-Encoding is gzip or deflate,
// returns the HTTP status code, raw body bytes, and any read or decompression error.
func (p *Response) ToBytes() (int, []byte, error) {
	statusCode := p.StatusCode

	defer p.Body.Close()

	var reader io.ReadCloser
	var err error

	switch p.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(p.Body)
		if err != nil {
			return statusCode, nil, err
		}
	case "deflate":
		reader = flate.NewReader(p.Body)
	default:
		reader = p.Body
	}

	defer reader.Close()

	body, err := io.ReadAll(reader)
	if err != nil {
		return statusCode, nil, err
	}

	return statusCode, body, nil
}

// ToString returns the response body as a UTF-8 string via [Response.ToBytes].
func (p *Response) ToString() (int, string, error) {
	statusCode, bytes, err := p.ToBytes()
	if err != nil {
		return statusCode, "", err
	}

	return statusCode, string(bytes), nil
}
