/**
 * @Author: lidonglin
 * @Description:
 * @File:  http_client.go
 * @Version: 1.0.0
 * @Date: 2022/05/28 10:46
 */

package thttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	_url "net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/json-iterator/go"

	"github.com/choveylee/tlog"
)

const (
	TransProxyTypeHttp int = iota
	TransProxyTypeSocks4
	TransProxyTypeSocks5
	TransProxyTypeSocks4A
)

const (
	OptTimeout int = iota

	// Transport Option
	OptTransConnectTimeout
	OptTransDeadlineTimeout

	OptTransProxyType
	OptTransProxyAddr
	OptTransProxyFunc

	OptTransMaxIdleConns
	OptTransMaxIdleConnsPerHost
	OptTransMaxConnsPerHost

	OptTransUnsafeTls
	OptTransTlsConfig

	OptTransRetry
	OptTransLog

	// Cookie Jar Option
	OptCookieJar

	// Redirect Option
	OptRedirectPolicy

	// Extra Option
	OptExtraRequestHookFunc

	OptExtraResponseHookFunc
)

var (
	OptTransports = map[int]int{
		OptTransConnectTimeout:  OptTransConnectTimeout,
		OptTransDeadlineTimeout: OptTransDeadlineTimeout,

		OptTransProxyType: OptTransProxyType,
		OptTransProxyAddr: OptTransProxyAddr,
		OptTransProxyFunc: OptTransProxyFunc,

		OptTransMaxIdleConns:        OptTransMaxIdleConns,
		OptTransMaxIdleConnsPerHost: OptTransMaxIdleConnsPerHost,
		OptTransMaxConnsPerHost:     OptTransMaxConnsPerHost,

		OptTransUnsafeTls: OptTransUnsafeTls,
		OptTransTlsConfig: OptTransTlsConfig,
	}
)

var defaultTransport = &http.Transport{
	DialContext: defaultTransportDialContext(&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}),

	ForceAttemptHTTP2: true,

	MaxIdleConns:    100,
	IdleConnTimeout: 90 * time.Second,

	TLSHandshakeTimeout: 10 * time.Second,

	ExpectContinueTimeout: 1 * time.Second,
}

type ProxyFunc func(*http.Request) (int, string, error)
type RedirectPolicyFunc func(*http.Request, []*http.Request) error
type RequestHookFunc func(*http.Client, *http.Request)
type ResponseHookFunc func(*http.Response, error)

func prepareRequest(ctx context.Context, method string, url string, headers map[string]string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	for key, val := range headers {
		req.Header.Set(key, val)
	}

	return req, nil
}

func defaultTransportDialContext(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return dialer.DialContext
}

func wrapTransport(transport http.RoundTripper, options map[int]interface{}) (http.RoundTripper, error) {
	// add retry transport
	retryTransOption := defaultRetryTransOption

	srcRetryTransOption, ok := options[OptTransRetry]
	if ok == true {
		desRetryTransOption, ok := srcRetryTransOption.(*RetryTransOption)
		if ok == true {
			retryTransOption = desRetryTransOption
		} else {
			return nil, fmt.Errorf("retry trans option type illegal")
		}
	}

	retryTransport := wrapRetryTransport(transport, retryTransOption)

	// add log transport
	logTransOption := defaultLogTransOption

	srcLogTransOption, ok := options[OptTransLog]
	if ok == true {
		destLogTransOption, ok := srcLogTransOption.(*LogTransOption)
		if ok == true {
			logTransOption = destLogTransOption
		} else {
			return nil, fmt.Errorf("log trans option type illegal")
		}
	}

	logTransport := wrapLogTransport(retryTransport, logTransOption)

	return logTransport, nil
}

func prepareCookieJar(options map[int]interface{}) (http.CookieJar, error) {
	srcOptCookieJar, ok := options[OptCookieJar]
	if ok == true {
		// is bool
		optCookieJar, ok := srcOptCookieJar.(bool)
		if ok == true {
			// default cookieJar
			if optCookieJar == true {
				// TODO: PublicSuffixList
				jar, err := cookiejar.New(nil)
				if err != nil {
					return nil, err
				}

				return jar, nil
			}
		} else {
			jar, ok := srcOptCookieJar.(http.CookieJar)
			if ok == false {
				return nil, fmt.Errorf("cookie cookieJar type illegal, cookie cookieJar supported")
			}

			return jar, nil
		}
	}

	return nil, nil
}

func prepareRedirect(options map[int]interface{}) (func(req *http.Request, via []*http.Request) error, error) {
	var redirectPolicy func(req *http.Request, via []*http.Request) error

	srcRedirectPolicy, ok := options[OptRedirectPolicy]
	if ok == true {
		destRedirectPolicy, ok := srcRedirectPolicy.(func(*http.Request, []*http.Request) error)
		if ok == false {
			return nil, fmt.Errorf("redirect policy type illegal")
		}

		redirectPolicy = destRedirectPolicy
	}

	return redirectPolicy, nil
}

type HttpClient struct {
	// Default options of this client.
	options map[int]interface{}

	// Default Headers of this client.
	headers map[string]string

	// Global transport of this client
	transport *http.Transport

	connectTimeout  time.Duration
	deadlineTimeout time.Duration

	// Global cookie cookieJar of this client
	cookieJar http.CookieJar

	withDebug bool

	sync.RWMutex
}

func NewHttpClient() *HttpClient {
	httpClient := &HttpClient{
		options: make(map[int]interface{}),
		headers: make(map[string]string),

		transport: defaultTransport,

		connectTimeout:  30 * time.Second,
		deadlineTimeout: 30 * time.Second,
	}

	cookieJar, err := cookiejar.New(nil)
	if err == nil {
		httpClient.cookieJar = cookieJar
	}

	return httpClient
}

func (p *HttpClient) Defaults(options map[int]interface{}, headers map[string]string) *HttpClient {
	p.Lock()
	defer p.Unlock()

	// merge options
	if p.options != nil {
		for key, val := range options {
			p.options[key] = val
		}
	}

	// merge headers
	if p.headers != nil {
		for key, val := range headers {
			p.headers[key] = val
		}
	}

	return p
}

func (p *HttpClient) Debug(val bool) *HttpClient {
	p.Lock()
	defer p.Unlock()

	p.withDebug = val

	return p
}

func (p *HttpClient) Transport() http.RoundTripper {
	p.Lock()
	defer p.Lock()

	return p.transport
}

func (p *HttpClient) resetTransport(key int, val interface{}) error {
	if key == OptTransMaxIdleConns {
		destMaxIdleConns, ok := val.(int)
		if ok == true {
			p.transport.MaxIdleConns = destMaxIdleConns

			return nil
		} else {
			return fmt.Errorf("max idle conns type illegal, int supported")
		}
	}

	if key == OptTransMaxIdleConnsPerHost {
		destMaxIdleConnsPerHost, ok := val.(int)
		if ok == true {
			p.transport.MaxIdleConnsPerHost = destMaxIdleConnsPerHost

			return nil
		} else {
			return fmt.Errorf("max idle conns per host type illegal, int supported")
		}
	}

	if key == OptTransMaxConnsPerHost {
		destMaxConnsPerHost, ok := val.(int)
		if ok == true {
			p.transport.MaxConnsPerHost = destMaxConnsPerHost

			return nil
		} else {
			return fmt.Errorf("max conns per host type illegal, int supported")
		}
	}

	if key == OptTransConnectTimeout || key == OptTransDeadlineTimeout {
		if key == OptTransConnectTimeout {
			destConnectTimeout, ok := val.(time.Duration)
			if ok == false {
				destConnectTimeout, ok := val.(int)
				if ok == true {
					p.connectTimeout = time.Duration(destConnectTimeout) * time.Millisecond
				} else {
					return fmt.Errorf("connect timeout type illegal, int supported")
				}
			} else {
				p.connectTimeout = destConnectTimeout
			}
		}

		if key == OptTransDeadlineTimeout {
			destTimeout, ok := val.(time.Duration)
			if ok == false {
				destTimeout, ok := val.(int)
				if ok == true {
					p.deadlineTimeout = time.Duration(destTimeout) * time.Millisecond
				} else {
					return fmt.Errorf("timeout type illegal, int supported")
				}
			} else {
				p.deadlineTimeout = destTimeout
			}
		}

		// fix connect timeout (important, or it might cause a long time wait during connection)
		if p.deadlineTimeout > 0 && (p.connectTimeout > p.deadlineTimeout || p.connectTimeout == 0) {
			p.connectTimeout = p.deadlineTimeout
		}

		p.transport.DialContext = func(ctx context.Context, network string, addr string) (net.Conn, error) {
			var conn net.Conn
			var err error

			if p.connectTimeout > 0 {
				conn, err = net.DialTimeout(network, addr, p.connectTimeout)
				if err != nil {
					return nil, err
				}
			} else {
				conn, err = net.Dial(network, addr)
				if err != nil {
					return nil, err
				}
			}

			conn.SetDeadline(time.Now().Add(p.deadlineTimeout))

			return conn, nil
		}
	}

	// proxy
	if key == OptTransProxyFunc {
		destProxyFunc, ok := val.(func(*http.Request) (int, string, error))
		if ok == true {
			proxyFunc := destProxyFunc

			p.transport.Proxy = func(req *http.Request) (*_url.URL, error) {
				proxyType, proxy, err := proxyFunc(req)
				if err != nil {
					return nil, err
				}

				if proxyType != TransProxyTypeHttp {
					return nil, fmt.Errorf("only proxy http supported")
				}

				if strings.Contains(proxy, "://") == false {
					proxy = "http://" + proxy
				}

				proxyUrl, err := _url.Parse(proxy)
				if err != nil {
					return nil, err
				}

				return proxyUrl, nil
			}

			return nil
		} else {
			return fmt.Errorf("proxy func type illegal")
		}
	} else {
		var proxyType int

		if key == OptTransProxyType {
			destProxyType, ok := val.(int)
			if ok == true {
				proxyType = destProxyType

				if proxyType != TransProxyTypeHttp {
					return fmt.Errorf("only proxy http supported")
				}

				return nil
			} else {
				return fmt.Errorf("proxy type illegal, int supported")
			}
		}

		var proxy string

		if key == OptTransProxyAddr {
			destProxy, ok := val.(string)
			if ok == true {
				proxy = destProxy

				if strings.Contains(proxy, "://") == false {
					proxy = "http://" + proxy
				}

				proxyUrl, err := _url.Parse(proxy)
				if err != nil {
					return err
				}

				p.transport.Proxy = http.ProxyURL(proxyUrl)

				return nil
			} else {
				return fmt.Errorf("proxy type illegal, string supported")
			}
		}
	}

	if key == OptTransUnsafeTls {
		destUnsafeTls, ok := val.(bool)
		if ok == true {
			unsafeTls := destUnsafeTls

			tlsConfig := p.transport.TLSClientConfig

			if tlsConfig == nil {
				tlsConfig = &tls.Config{}
				p.transport.TLSClientConfig = tlsConfig
			}

			tlsConfig.InsecureSkipVerify = unsafeTls

			return nil
		} else {
			return fmt.Errorf("unsafe tls type illegal, bool supported")
		}
	}

	if key == OptTransTlsConfig {
		destTlsConfig, ok := val.(*tls.Config)
		if ok == true {
			p.transport.TLSClientConfig = destTlsConfig
		} else {
			return fmt.Errorf("tls config type illegal, tls config supported")
		}
	}

	return nil
}

func (p *HttpClient) WithOption(key int, val interface{}) *HttpClient {
	p.Lock()
	defer p.Unlock()

	_, ok := OptTransports[key]
	if ok == true {
		p.resetTransport(key, val)
	} else {
		p.options[key] = val
	}

	return p
}

// WithTimeout timeout option
func (p *HttpClient) WithTimeout(timeout time.Duration) *HttpClient {
	return p.WithOption(OptTimeout, timeout)
}

// WithConnectTimeout connect timeout option
func (p *HttpClient) WithConnectTimeout(timeout time.Duration) *HttpClient {
	return p.WithOption(OptTransConnectTimeout, timeout)
}

// WithDeadlineTimeout deadline timeout option
func (p *HttpClient) WithDeadlineTimeout(timeout time.Duration) *HttpClient {
	return p.WithOption(OptTransConnectTimeout, timeout)
}

// WithProxyType TransProxyTypeHttp
func (p *HttpClient) WithProxyType(proxyType int) *HttpClient {
	return p.WithOption(OptTransProxyType, proxyType)
}

// WithProxyAddress proxy address: ip:port
func (p *HttpClient) WithProxyAddress(addr string) *HttpClient {
	return p.WithOption(OptTransProxyAddr, addr)
}

// WithProxyFunc proxy func
func (p *HttpClient) WithProxyFunc(option ProxyFunc) *HttpClient {
	return p.WithOption(OptTransProxyFunc, option)
}

// WithUnsafeTls https TLS
func (p *HttpClient) WithUnsafeTls(unsafe bool) *HttpClient {
	return p.WithOption(OptTransUnsafeTls, unsafe)
}

// WithRetryTransOption retry trans option
func (p *HttpClient) WithRetryTransOption(option *RetryTransOption) *HttpClient {
	return p.WithOption(OptTransRetry, option)
}

// WithLogTransOption log trans option
func (p *HttpClient) WithLogTransOption(option *LogTransOption) *HttpClient {
	return p.WithOption(OptTransLog, option)
}

// WithCookieJar cookie jar
func (p *HttpClient) WithCookieJar(jar http.CookieJar) *HttpClient {
	return p.WithOption(OptCookieJar, jar)
}

// WithRedirectPolicy redirect policy
func (p *HttpClient) WithRedirectPolicy(option RedirectPolicyFunc) *HttpClient {
	return p.WithOption(OptRedirectPolicy, option)
}

// WithRequestHookFunc request hook func
func (p *HttpClient) WithRequestHookFunc(option RequestHookFunc) *HttpClient {
	return p.WithOption(OptExtraRequestHookFunc, option)
}

// WithResponseHookFunc response hook func
func (p *HttpClient) WithResponseHookFunc(option ResponseHookFunc) *HttpClient {
	return p.WithOption(OptExtraResponseHookFunc, option)
}

func (p *HttpClient) WithOptions(options map[int]interface{}) *HttpClient {
	for key, val := range options {
		p.WithOption(key, val)
	}

	return p
}

func (p *HttpClient) WithHeader(key string, val string) *HttpClient {
	p.Lock()
	defer p.Unlock()

	p.headers[strings.ToLower(key)] = val

	return p
}

func (p *HttpClient) WithReferer(val string) *HttpClient {
	return p.WithHeader("referer", val)
}

func (p *HttpClient) WithUserAgent(val string) *HttpClient {
	return p.WithHeader("user-agent", val)
}

func (p *HttpClient) WithContentType(val string) *HttpClient {
	return p.WithHeader("content-type", val)
}

func (p *HttpClient) WithHeaders(headers map[string]string) *HttpClient {
	for key, val := range headers {
		p.WithHeader(key, val)
	}

	return p
}

func (p *HttpClient) Do(ctx context.Context, method string, url string, requestOption *RequestOption, body io.Reader) (*Response, error) {
	p.RLock()
	defer p.RUnlock()

	// prepare all request configs
	// merge options
	options := make(map[int]interface{})

	for key, val := range p.options {
		options[key] = val
	}

	if requestOption != nil {
		for key, val := range requestOption.Options {
			options[key] = val
		}
	}

	// merge headers
	headers := make(map[string]string)

	for key, val := range p.headers {
		headers[key] = val
	}

	if requestOption != nil {
		for key, val := range requestOption.Headers {
			headers[key] = val
		}
	}

	// set cookies
	cookies := make([]*http.Cookie, 0)

	if requestOption != nil {
		cookies = requestOption.Cookies
	}

	// transport
	transport, err := wrapTransport(p.transport, options)
	if err != nil {
		return nil, err
	}

	// cookieJar
	cookieJar, err := prepareCookieJar(options)
	if err != nil {
		return nil, err
	}

	// redirect
	redirect, err := prepareRedirect(options)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Transport:     transport,
		CheckRedirect: redirect,
		Jar:           cookieJar,
	}

	timeout := time.Second * 30

	srcTimeout, ok := options[OptTimeout]
	if ok == true {
		destTimeout, ok := srcTimeout.(time.Duration)
		if ok == false {
			return nil, fmt.Errorf("timeout type illegal, time.duration supported")
		} else {
			timeout = destTimeout
		}
	}

	client.Timeout = timeout

	var bodyBytes []byte

	if body != nil {
		bodyBytes, _ = io.ReadAll(body)

		// body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		body = bytes.NewReader(bodyBytes)
	}

	request, err := prepareRequest(ctx, method, url, headers, body)
	if err != nil {
		return nil, err
	}

	// output debug info
	if p.withDebug == true {
		dump, err := httputil.DumpRequestOut(request, true)

		if err == nil {
			fmt.Printf("%s\n", dump)
		}
	}

	// cookieJar is not nil
	if cookieJar != nil {
		cookieJar.SetCookies(request.URL, cookies)
	} else {
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
	}

	srcRequestHookFunc, ok := options[OptExtraRequestHookFunc]
	if ok == true {
		requestHookFunc, ok := srcRequestHookFunc.(func(context.Context, *http.Client, *http.Request))
		if ok == true {
			requestHookFunc(ctx, client, request)
		}
	}

	response, err := client.Do(request)

	if err != nil || response.StatusCode != http.StatusOK {
		event := tlog.W(request.Context()).Err(err).Detailf("req.method: %s", request.Method).
			Detailf("req.host: %s", request.Host).Detailf("req.url: %s", request.URL.String()).
			Detailf("req.body: %s", string(bodyBytes))

		for key, vals := range request.Header {
			event = event.Detailf("req.header.%s: %s", key, strings.Join(vals, ";"))
		}

		if response != nil {
			event = event.Detailf("resp.status code: %d", response.StatusCode)
		}

		event.Msg("http client abnormal log")
	}

	srcResponseHookFunc, ok := options[OptExtraResponseHookFunc]
	if ok == true {
		responseHookFunc, ok := srcResponseHookFunc.(func(context.Context, *http.Response, error))
		if ok == true {
			responseHookFunc(ctx, response, err)
		}
	}

	return &Response{response}, err
}

func (p *HttpClient) send(ctx context.Context, method string, url string, requestOption *RequestOption, params interface{}) (*Response, error) {
	var body io.Reader

	switch retParams := params.(type) {
	case nil:
		body = bytes.NewReader(nil)
	case []byte:
		body = bytes.NewReader(retParams)
	case string:
		body = strings.NewReader(retParams)
	case *bytes.Reader:
		body = retParams
	case _url.Values:
		body = strings.NewReader(retParams.Encode())
	default:
		return nil, fmt.Errorf("params type not support")
	}

	return p.Do(ctx, method, url, requestOption, body)
}

func (p *HttpClient) sendJson(ctx context.Context, method string, url string, requestOption *RequestOption, params interface{}) (*Response, error) {
	if requestOption != nil {
		requestOption = NewRequestOption()
	}

	requestOption.WithContentType(ContentTypeApplicationJson)

	var body io.Reader

	switch retParams := params.(type) {
	case nil:
		body = bytes.NewReader(nil)
	case []byte:
		body = bytes.NewReader(retParams)
	case string:
		body = strings.NewReader(retParams)
	case *bytes.Reader:
		body = retParams
	default:
		data, err := jsoniter.Marshal(retParams)
		if err != nil {
			return nil, err
		}

		body = bytes.NewReader(data)
	}

	return p.Do(ctx, method, url, requestOption, body)
}

func (p *HttpClient) Head(ctx context.Context, url string, requestOption *RequestOption) (*Response, error) {
	return p.Do(ctx, "HEAD", url, requestOption, nil)
}

func (p *HttpClient) Get(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	url = appendParams(url, params)

	return p.Do(ctx, "GET", url, requestOption, nil)
}

func (p *HttpClient) GetLen(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (int64, error) {
	url = appendParams(url, params)

	resp, err := p.Do(ctx, "HEAD", url, requestOption, nil)
	if err != nil {
		return -1, err
	}

	contentLen := resp.Header.Get("Content-Length")

	length, err := strconv.ParseInt(contentLen, 10, 64)
	if err != nil {
		return -1, err
	}

	return length, nil
}

// Post support data type: []byte
func (p *HttpClient) Post(ctx context.Context, url string, requestOption *RequestOption, params []byte) (*Response, error) {
	/*
		switch retParams := params.(type) {
		case _url.Values:
			for key := range retParams {
				if len(key) > 0 && key[0] == '@' {
					return p.PostMultipart(ctx, url, retParams)
				}
			}
		}
	*/

	return p.send(ctx, "POST", url, requestOption, params)
}

// PostJson support data type: map[string]interface{}/struct
func (p *HttpClient) PostJson(ctx context.Context, url string, requestOption *RequestOption, params interface{}) (*Response, error) {
	return p.sendJson(ctx, "POST", url, requestOption, params)
}

// PostMultipart support data type: file
func (p *HttpClient) PostMultipart(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	for key, values := range params {
		for _, value := range values {
			// if value is file
			if len(key) > 0 && key[0] == '@' {
				err := loadFormFile(writer, key[1:], value)
				if err != nil {
					return nil, err
				}
			} else {
				err := writer.WriteField(key, value)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	if requestOption == nil {
		requestOption = NewRequestOption()
	}

	requestOption.WithContentType(writer.FormDataContentType())

	err := writer.Close()
	if err != nil {
		return nil, err
	}

	return p.Do(ctx, "POST", url, requestOption, body)
}

func (p *HttpClient) Put(ctx context.Context, url string, requestOption *RequestOption, params []byte) (*Response, error) {
	return p.send(ctx, "PUT", url, requestOption, params)
}

func (p *HttpClient) PutJson(ctx context.Context, url string, requestOption *RequestOption, params interface{}) (*Response, error) {
	return p.sendJson(ctx, "PUT", url, requestOption, params)
}

func (p *HttpClient) Patch(ctx context.Context, url string, requestOption *RequestOption, params []byte) (*Response, error) {
	return p.send(ctx, "PATCH", url, requestOption, params)
}

func (p *HttpClient) PatchJson(ctx context.Context, url string, requestOption *RequestOption, params interface{}) (*Response, error) {
	return p.sendJson(ctx, "PATCH", url, requestOption, params)
}

func (p *HttpClient) Delete(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	url = appendParams(url, params)

	return p.send(ctx, "DELETE", url, requestOption, nil)
}

func (p *HttpClient) Options(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	url = appendParams(url, params)

	return p.send(ctx, "OPTIONS", url, requestOption, nil)
}

func (p *HttpClient) Connect(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	url = appendParams(url, params)

	return p.send(ctx, "CONNECT", url, requestOption, nil)
}

func (p *HttpClient) Trace(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) {
	url = appendParams(url, params)

	return p.send(ctx, "TRACE", url, requestOption, nil)
}
