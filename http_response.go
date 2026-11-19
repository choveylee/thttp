package thttp

import (
	"bufio"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Response wraps [http.Response] with helpers to read the body.
type Response struct {
	*http.Response
}

// ToBytes reads and closes the response body. It decodes supported encodings
// in reverse order and returns the raw body if any encoding is unsupported.
func (response *Response) ToBytes() (int, []byte, error) {
	if response == nil || response.Response == nil {
		return 0, nil, errors.New("thttp: response object is unavailable")
	}

	statusCode := response.StatusCode

	if response.Body == nil {
		return statusCode, nil, errors.New("thttp: response body is unavailable")
	}

	defer response.Body.Close()

	if response.Body == http.NoBody || statusCode == http.StatusNoContent || statusCode == http.StatusNotModified ||
		(response.Request != nil && response.Request.Method == http.MethodHead) {
		return statusCode, []byte{}, nil
	}

	var reader io.Reader = response.Body
	encodings := responseEncodings(response.Header)
	for index := len(encodings) - 1; index >= 0; index-- {
		encoding := encodings[index]
		var decoder io.ReadCloser
		var err error
		switch encoding {
		case "gzip":
			decoder, err = gzip.NewReader(reader)
		case "deflate":
			decoder, err = deflateReader(reader)
		}

		if err != nil {
			return statusCode, nil, fmt.Errorf("thttp: decode %s response body: %w", encoding, err)
		}

		defer decoder.Close()
		reader = decoder
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return statusCode, nil, err
	}

	return statusCode, body, nil
}

// responseEncodings collects codings in application order. An unsupported
// coding leaves the whole representation available in its original form.
func responseEncodings(headerMap http.Header) []string {
	var encodings []string
	for _, value := range headerMap.Values("Content-Encoding") {
		for part := range strings.SplitSeq(value, ",") {
			encoding := strings.ToLower(strings.TrimSpace(part))
			switch encoding {
			case "gzip", "deflate":
				encodings = append(encodings, encoding)
			case "", "identity":
			default:
				return nil
			}
		}
	}

	return encodings
}

// deflateReader supports the standard zlib wrapper and legacy raw deflate.
func deflateReader(body io.Reader) (io.ReadCloser, error) {
	reader := bufio.NewReader(body)
	header, err := reader.Peek(2)
	if err == nil && header[0]&0x0f == 8 && header[0]>>4 <= 7 &&
		(uint16(header[0])<<8|uint16(header[1]))%31 == 0 {
		return zlib.NewReader(reader)
	}

	return flate.NewReader(reader), nil
}

// ToString reads and closes the response body via [Response.ToBytes].
func (response *Response) ToString() (int, string, error) {
	statusCode, bytes, err := response.ToBytes()
	if err != nil {
		return statusCode, "", err
	}

	return statusCode, string(bytes), nil
}
