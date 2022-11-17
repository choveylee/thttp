/**
 * @Author: lidonglin
 * @Description:
 * @File:  http_response.go
 * @Version: 1.0.0
 * @Date: 2022/05/28 10:46
 */

package thttp

import (
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"
)

type Response struct {
	*http.Response
}

func (p *Response) ToBytes() (int, []byte, error) {
	statusCode := p.StatusCode

	defer p.Body.Close()

	var reader io.ReadCloser
	var err error

	switch p.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(p.Body)
		if err != nil {
			p.Body.Close()

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

func (p *Response) ToString() (int, string, error) {
	statusCode, bytes, err := p.ToBytes()
	if err != nil {
		return statusCode, "", err
	}

	return statusCode, string(bytes), nil
}
