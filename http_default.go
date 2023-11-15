/**
 * @Author: lidonglin
 * @Description:
 * @File:  http_default.go
 * @Version: 1.0.0
 * @Date: 2022/05/28 10:46
 */

package thttp

import (
	"context"
	"io"
	"net/http"
	_url "net/url"
	"time"
)

// default client for convenience
var defaultClient = NewHttpClient()

func Defaults(options map[int]interface{}, headers map[string]string) *HttpClient {
	return defaultClient.Defaults(options, headers)
}

func Debug(val bool) *HttpClient {
	return defaultClient.Debug(val)
}

func WithOption(key int, val interface{}) *HttpClient {
	return defaultClient.WithOption(key, val)
}

func WithTimeout(timeout time.Duration) *HttpClient {
	return defaultClient.WithTimeout(timeout)
}

func WithProxyUrl(addr string) *HttpClient {
	return defaultClient.WithProxyUrl(addr)
}

func WithProxyFunc(option ProxyFunc) *HttpClient {
	return defaultClient.WithProxyFunc(option)
}

func WithUnsafeTls(unsafe bool) *HttpClient {
	return defaultClient.WithUnsafeTls(unsafe)
}

func WithRetryTransOption(option *RetryTransOption) *HttpClient {
	return defaultClient.WithRetryTransOption(option)
}

func WithLogTransOption(option *LogTransOption) *HttpClient {
	return defaultClient.WithLogTransOption(option)
}

func WithCookieJar(jar http.CookieJar) *HttpClient {
	return defaultClient.WithCookieJar(jar)
}

func WithRedirectPolicy(option RedirectPolicyFunc) *HttpClient {
	return defaultClient.WithRedirectPolicy(option)
}

func WithRequestHookFunc(option RequestHookFunc) *HttpClient {
	return defaultClient.WithRequestHookFunc(option)
}

func WithResponseHookFunc(option ResponseHookFunc) *HttpClient {
	return defaultClient.WithResponseHookFunc(option)
}

func WithOptions(options map[int]interface{}) *HttpClient {
	return defaultClient.WithOptions(options)
}

func WithHeader(key string, val string) *HttpClient {
	return defaultClient.WithHeader(key, val)
}

func WithReferer(val string) *HttpClient {
	return defaultClient.WithReferer(val)
}

func WithUserAgent(val string) *HttpClient {
	return defaultClient.WithUserAgent(val)
}

func WithContentType(val string) *HttpClient {
	return defaultClient.WithContentType(val)
}

func WithHeaders(headers map[string]string) *HttpClient {
	return defaultClient.WithHeaders(headers)
}

func Do(ctx context.Context, method string, url string, requestOption *RequestOption, body io.Reader) (*Response, error) {
	return defaultClient.Do(ctx, method, url, requestOption, body)
}

func Head(ctx context.Context, url string, requestOption *RequestOption) (*Response, error) {
	return defaultClient.Head(ctx, url, requestOption)
}

func Get(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	return defaultClient.Get(ctx, url, requestOption, params)
}

func GetLen(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (int64, error) {
	return defaultClient.GetLen(ctx, url, requestOption, params)
}

func Post(ctx context.Context, url string, requestOption *RequestOption, params []byte) (*Response, error) {
	return defaultClient.Post(ctx, url, requestOption, params)
}

func PostJson(ctx context.Context, url string, requestOption *RequestOption, params interface{}) (*Response, error) {
	return defaultClient.PostJson(ctx, url, requestOption, params)
}

func PostMultipart(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	return defaultClient.PostMultipart(ctx, url, requestOption, params)
}

func PostMultipartEx(ctx context.Context, url string, requestOption *RequestOption, params []*FormData) (*Response, error) {
	return defaultClient.PostMultipartEx(ctx, url, requestOption, params)
}

func Put(ctx context.Context, url string, requestOption *RequestOption, params []byte) (*Response, error) {
	return defaultClient.Put(ctx, url, requestOption, params)
}

func PutJson(ctx context.Context, url string, requestOption *RequestOption, params interface{}) (*Response, error) {
	return defaultClient.PutJson(ctx, url, requestOption, params)
}

func Patch(ctx context.Context, url string, requestOption *RequestOption, params []byte) (*Response, error) {
	return defaultClient.Patch(ctx, url, requestOption, params)
}

func PatchJson(ctx context.Context, url string, requestOption *RequestOption, params interface{}) (*Response, error) {
	return defaultClient.PatchJson(ctx, url, requestOption, params)
}

func Delete(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	return defaultClient.Delete(ctx, url, requestOption, params)
}

func Options(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	return defaultClient.Options(ctx, url, requestOption, params)
}

func Connect(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	return defaultClient.Connect(ctx, url, requestOption, params)
}

func Trace(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	return defaultClient.Trace(ctx, url, requestOption, params)
}
