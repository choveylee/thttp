package thttp

import (
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// appendParams appends URL-encoded query parameters while preserving any existing query string and fragment.
func appendParams(rawURL string, params url.Values) string {
	encoded := params.Encode()
	if encoded == "" {
		return rawURL
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	if parsedURL.RawQuery != "" {
		parsedURL.RawQuery += "&"
	}

	parsedURL.RawQuery += encoded

	return parsedURL.String()
}

// loadFormFile streams fileName into a multipart form file field named fieldName.
func loadFormFile(writer *multipart.Writer, fieldName, fileName string) error {
	file, err := os.Open(fileName)
	if err != nil {
		return err
	}

	defer file.Close()

	data, err := writer.CreateFormFile(fieldName, filepath.Base(fileName))
	if err != nil {
		return err
	}

	_, err = io.Copy(data, file)

	return err
}

// GetRealIP uses proxy-set IP headers and falls back to the remote address.
// Use it behind a trusted proxy that replaces incoming forwarding headers.
func GetRealIP(request *http.Request) string {
	if request == nil {
		return "127.0.0.1"
	}

	if address, err := netip.ParseAddr(strings.TrimSpace(request.Header.Get("X-Real-IP"))); err == nil {
		return address.String()
	}

	for _, value := range request.Header.Values("X-Forwarded-For") {
		for hop := range strings.SplitSeq(value, ",") {
			if address, err := netip.ParseAddr(strings.TrimSpace(hop)); err == nil {
				return address.String()
			}
		}
	}

	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		host = request.RemoteAddr
	}

	if address, err := netip.ParseAddr(host); err == nil {
		return address.String()
	}

	return "127.0.0.1"
}

// GetRealHost uses X-Host from a trusted proxy when present.
func GetRealHost(request *http.Request) string {
	if request == nil {
		return ""
	}

	if host := request.Header.Get("X-Host"); host != "" {
		return host
	}

	return request.Host
}

// GetRealPort derives the default port from trusted X-Scheme or direct TLS.
func GetRealPort(request *http.Request) int32 {
	if request == nil {
		return 80
	}

	scheme := strings.TrimSpace(request.Header.Get("X-Scheme"))
	if strings.EqualFold(scheme, "https") || (scheme == "" && request.TLS != nil) {
		return 443
	}

	return 80
}
