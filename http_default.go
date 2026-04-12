package thttp

import (
	"context"
	"io"
	"net/http"
	_url "net/url"
	"time"
)

// defaultClient is the package-wide [HttpClient] used by the top-level helper functions.
var defaultClient = NewHttpClient()

// Defaults merges options and headers into the default client. See [HttpClient.Defaults].
func Defaults(options map[int]interface{}, headers map[string]string) *HttpClient {
	return defaultClient.Defaults(options, headers)
}

// Debug toggles verbose request logging on the default client. See [HttpClient.Debug].
func Debug(val bool) *HttpClient {
	return defaultClient.Debug(val)
}

// WithOption sets an option on the default client. See [HttpClient.WithOption].
func WithOption(key int, val interface{}) *HttpClient {
	return defaultClient.WithOption(key, val)
}

// WithTimeout sets the default request timeout. See [HttpClient.WithTimeout].
func WithTimeout(timeout time.Duration) *HttpClient {
	return defaultClient.WithTimeout(timeout)
}

// WithProxyUrl sets a static HTTP proxy on the default client. See [HttpClient.WithProxyUrl].
func WithProxyUrl(addr string) *HttpClient {
	return defaultClient.WithProxyUrl(addr)
}

// WithProxyFunc sets a proxy resolver on the default client. See [HttpClient.WithProxyFunc].
func WithProxyFunc(option ProxyFunc) *HttpClient {
	return defaultClient.WithProxyFunc(option)
}

// WithUnsafeTls controls TLS verification on the default client. See [HttpClient.WithUnsafeTls].
func WithUnsafeTls(unsafe bool) *HttpClient {
	return defaultClient.WithUnsafeTls(unsafe)
}

// WithRetryTransOption enables request retries on the default client. See [HttpClient.WithRetryTransOption].
func WithRetryTransOption(option *RetryTransOption) *HttpClient {
	return defaultClient.WithRetryTransOption(option)
}

// WithLogTransOption enables transport logging on the default client. See [HttpClient.WithLogTransOption].
func WithLogTransOption(option *LogTransOption) *HttpClient {
	return defaultClient.WithLogTransOption(option)
}

// WithCookieJar sets the cookie jar on the default client. See [HttpClient.WithCookieJar].
func WithCookieJar(jar http.CookieJar) *HttpClient {
	return defaultClient.WithCookieJar(jar)
}

// WithRedirectPolicy sets the redirect policy on the default client. See [HttpClient.WithRedirectPolicy].
func WithRedirectPolicy(option RedirectPolicyFunc) *HttpClient {
	return defaultClient.WithRedirectPolicy(option)
}

// WithRequestHookFunc registers a pre-request hook on the default client. See [HttpClient.WithRequestHookFunc].
func WithRequestHookFunc(option RequestHookFunc) *HttpClient {
	return defaultClient.WithRequestHookFunc(option)
}

// WithResponseHookFunc registers a post-request hook on the default client. See [HttpClient.WithResponseHookFunc].
func WithResponseHookFunc(option ResponseHookFunc) *HttpClient {
	return defaultClient.WithResponseHookFunc(option)
}

// WithOptions applies multiple options to the default client. See [HttpClient.WithOptions].
func WithOptions(options map[int]interface{}) *HttpClient {
	return defaultClient.WithOptions(options)
}

// WithHeader sets a default header on the default client. See [HttpClient.WithHeader].
func WithHeader(key string, val string) *HttpClient {
	return defaultClient.WithHeader(key, val)
}

// WithReferer sets the default Referer header. See [HttpClient.WithReferer].
func WithReferer(val string) *HttpClient {
	return defaultClient.WithReferer(val)
}

// WithUserAgent sets the default User-Agent header. See [HttpClient.WithUserAgent].
func WithUserAgent(val string) *HttpClient {
	return defaultClient.WithUserAgent(val)
}

// WithContentType sets the default Content-Type header. See [HttpClient.WithContentType].
func WithContentType(val string) *HttpClient {
	return defaultClient.WithContentType(val)
}

// WithHeaders merges default headers on the default client. See [HttpClient.WithHeaders].
func WithHeaders(headers map[string]string) *HttpClient {
	return defaultClient.WithHeaders(headers)
}

// Do issues a request using the default client. See [HttpClient.Do].
func Do(ctx context.Context, method string, url string, requestOption *RequestOption, body io.Reader) (*Response, error) {
	return defaultClient.Do(ctx, method, url, requestOption, body)
}

// Head sends a HEAD request using the default client. See [HttpClient.Head].
func Head(ctx context.Context, url string, requestOption *RequestOption) (*Response, error) {
	return defaultClient.Head(ctx, url, requestOption)
}

// Get sends a GET request using the default client. See [HttpClient.Get].
func Get(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	return defaultClient.Get(ctx, url, requestOption, params)
}

// GetLen returns Content-Length from a HEAD request via the default client. See [HttpClient.GetLen].
func GetLen(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (int64, error) {
	return defaultClient.GetLen(ctx, url, requestOption, params)
}

// Post sends a POST request using the default client. See [HttpClient.Post].
func Post(ctx context.Context, url string, requestOption *RequestOption, params []byte) (*Response, error) {
	return defaultClient.Post(ctx, url, requestOption, params)
}

// PostJson sends a JSON POST using the default client. See [HttpClient.PostJson].
func PostJson(ctx context.Context, url string, requestOption *RequestOption, params interface{}) (*Response, error) {
	return defaultClient.PostJson(ctx, url, requestOption, params)
}

// PostMultipart sends multipart/form-data using the default client. See [HttpClient.PostMultipart].
func PostMultipart(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	return defaultClient.PostMultipart(ctx, url, requestOption, params)
}

// PostMultipartEx sends structured multipart data using the default client. See [HttpClient.PostMultipartEx].
func PostMultipartEx(ctx context.Context, url string, requestOption *RequestOption, params []*FormData) (*Response, error) {
	return defaultClient.PostMultipartEx(ctx, url, requestOption, params)
}

// Put sends a PUT request using the default client. See [HttpClient.Put].
func Put(ctx context.Context, url string, requestOption *RequestOption, params []byte) (*Response, error) {
	return defaultClient.Put(ctx, url, requestOption, params)
}

// PutJson sends a JSON PUT using the default client. See [HttpClient.PutJson].
func PutJson(ctx context.Context, url string, requestOption *RequestOption, params interface{}) (*Response, error) {
	return defaultClient.PutJson(ctx, url, requestOption, params)
}

// Patch sends a PATCH request using the default client. See [HttpClient.Patch].
func Patch(ctx context.Context, url string, requestOption *RequestOption, params []byte) (*Response, error) {
	return defaultClient.Patch(ctx, url, requestOption, params)
}

// PatchJson sends a JSON PATCH using the default client. See [HttpClient.PatchJson].
func PatchJson(ctx context.Context, url string, requestOption *RequestOption, params interface{}) (*Response, error) {
	return defaultClient.PatchJson(ctx, url, requestOption, params)
}

// Delete sends a DELETE request using the default client. See [HttpClient.Delete].
func Delete(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	return defaultClient.Delete(ctx, url, requestOption, params)
}

// Options sends an OPTIONS request using the default client. See [HttpClient.Options].
func Options(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	return defaultClient.Options(ctx, url, requestOption, params)
}

// Connect sends a CONNECT request using the default client. See [HttpClient.Connect].
func Connect(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	return defaultClient.Connect(ctx, url, requestOption, params)
}

// Trace sends a TRACE request using the default client. See [HttpClient.Trace].
func Trace(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	return defaultClient.Trace(ctx, url, requestOption, params)
}
