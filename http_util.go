package thttp

import (
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// appendParams appends URL-encoded query parameters while preserving any existing query string and fragment.
func appendParams(rawURL string, params url.Values) string {
	if len(params) == 0 {
		return rawURL
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	query := parsedURL.Query()
	for key, values := range params {
		for _, value := range values {
			query.Add(key, value)
		}
	}
	parsedURL.RawQuery = query.Encode()

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

// GetRealIP returns the client IP from X-Real-Ip or the first valid X-Forwarded-For hop, or "127.0.0.1" if none match.
func GetRealIP(r *http.Request) string {
	varRealIP := r.Header.Get("X-Real-Ip")

	if len(varRealIP) > 0 {
		return varRealIP
	}

	valForwardedIP := r.Header.Get("X-Forwarded-For")

	if len(valForwardedIP) > 0 {
		strIPs := strings.Split(valForwardedIP, ",")

		if len(strIPs) > 0 {
			retReg, err := regexp.MatchString("((?:(?:25[0-5]|2[0-4]\\d|[01]?\\d?\\d)\\.){3}(?:25[0-5]|2[0-4]\\d|[01]?\\d?\\d))", strIPs[0])
			if err == nil && retReg == true {
				return strIPs[0]
			}
		}
	}

	return "127.0.0.1"
}

// GetRealHost returns the X-Host header when set, otherwise [http.Request.Host].
func GetRealHost(r *http.Request) string {
	valHost := r.Header.Get("X-Host")

	if valHost != "" {
		return valHost
	}

	return r.Host
}

// GetRealPort returns 443 when X-Scheme is "https", otherwise 80.
func GetRealPort(r *http.Request) int32 {
	valXScheme := r.Header.Get("X-Scheme")

	if valXScheme == "https" {
		return 443
	}

	return 80
}
