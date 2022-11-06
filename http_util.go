/**
 * @Author: lidonglin
 * @Description:
 * @File:  http_util.go
 * @Version: 1.0.0
 * @Date: 2022/05/28 10:46
 */

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

func appendParams(url string, params url.Values) string {
	if len(params) == 0 {
		return url
	}

	if strings.Contains(url, "?") == false {
		url += "?"
	}

	if strings.HasSuffix(url, "?") || strings.HasSuffix(url, "&") {
		url += params.Encode()
	} else {
		url += "&" + params.Encode()
	}

	return url
}

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

func GetRealHost(r *http.Request) string {
	valHost := r.Header.Get("X-Host")

	if valHost != "" {
		return valHost
	}

	return r.Host
}

func GetRealPort(r *http.Request) int32 {
	valXScheme := r.Header.Get("X-Scheme")

	if valXScheme == "https" {
		return 443
	}

	return 80
}
