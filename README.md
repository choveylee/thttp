<!--
 * @Author: lidonglin
 * @Date: 2022-05-10 14:25:00
 * @LastEditTime: 2022-05-10 14:25:00
 * @LastEditors: lidonglin
 * @Description: 功能描述
-->
# 模块简介
本模块提供一批 API 提供网络Http请求简易封装。
1. 默认实例，使用默认实例快速进行Http请求
2. 新建实例，复用新建实例进行Http请求
3. httpclient支持线程安全

## 使用方式
```
import "dev.rcrai.com/rcrai/huskar2/http"
```

## 接口定义
httpclient支持HTTP协议中所定义了所有方法
并且针对json类型及文件上传定制了独立的api接口
1. 对于常规接口，入参接受[]byte数据类型
2. 对于json接口(XXXJson)，入参接受map及struct数据类型
3. 对于文件上传接口(PostMultipart)，key值以@作为前缀

```
HEAD
func (p *HttpClient) Head(ctx context.Context, url string, requestOption *RequestOption) (*Response, error) 
func (p *HttpClient) GetLen(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (int64, error) 

GET
func (p *HttpClient) Get(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) 

POST
func (p *HttpClient) Post(ctx context.Context, url string, requestOption *RequestOption, params []byte) (*Response, error) 
func (p *HttpClient) PostJson(ctx context.Context, url string, requestOption *RequestOption, params interface{}) (*Response, error) 
func (p *HttpClient) PostMultipart(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) 

PUT
func (p *HttpClient) Put(ctx context.Context, url string, requestOption *RequestOption, params []byte) (*Response, error) 
func (p *HttpClient) PutJson(ctx context.Context, url string, requestOption *RequestOption, params interface{}) (*Response, error) 

PATCH
func (p *HttpClient) Patch(ctx context.Context, url string, requestOption *RequestOption, params []byte) (*Response, error) 
func (p *HttpClient) PatchJson(ctx context.Context, url string, requestOption *RequestOption, params interface{}) (*Response, error) 

DELETE
func (p *HttpClient) Delete(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) 

OPTIONS
func (p *HttpClient) Options(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) 

CONNECT
func (p *HttpClient) Connect(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) 

TRACE
func (p *HttpClient) Trace(ctx context.Context, url string, requestOption *RequestOption, params _url.Values) (*Response, error) 
```

## 自定义公共请求
可以自定义公共请求参数(定义的参数，可以被复用)
### 自定义公共请求Header
```
// WithHeader 单独添加Header参数
func (p *HttpClient) WithHeader(key string, val string) *HttpClient

// WithHeaders 批量添加Header参数
func (p *HttpClient) WithHeaders(headers map[string]string) *HttpClient

// WithReferer 配置Referer参数（调用WithHeader接口）
func (p *HttpClient) WithReferer(val string) *HttpClient

// WithUserAgent 配置UserAgent参数（调用WithHeader接口）
func (p *HttpClient) WithUserAgent(val string) *HttpClient

// WithContentType 配置ContentType参数（调用WithHeader接口）
func (p *HttpClient) WithContentType(val string) *HttpClient
```
### 自定义公共请求Option
可以配置超时时间、代理、CookieJar、自定义重试Option，以及添加Request和Response的Hook函数
```
// WithOption 单独添加Option参数
func (p *HttpClient) WithOption(key int, val interface{}) *HttpClient

// WithOptions 批量添加Option参数
func (p *HttpClient) WithOptions(options map[int]interface{}) *HttpClient

// WithConnectTimeout connect timeout option
func (p *HttpClient) WithConnectTimeout(timeout int) *HttpClient

// WithTimeout timeout option
func (p *HttpClient) WithTimeout(timeout int) *HttpClient

// WithProxyType enum: TRANS_PROXY_TYPE_HTTP
func (p *HttpClient) WithProxyType(proxyType int) *HttpClient

// WithProxyAddress proxy address: ip:port
func (p *HttpClient) WithProxyAddress(addr string) *HttpClient 

// WithProxyFunc proxy func
func (p *HttpClient) WithProxyFunc(option ProxyFunc) *HttpClient

// WithUnsafeTls https TLS
func (p *HttpClient) WithUnsafeTls(unsafe bool) *HttpClient 

// WithRetryOption retry option
func (p *HttpClient) WithRetryOption(option *RetryOption) *HttpClient

// WithCookieJar cookie jar
func (p *HttpClient) WithCookieJar(jar http.CookieJar) *HttpClient

// WithRedirectPolicy redirect policy
func (p *HttpClient) WithRedirectPolicy(option RedirectPolicyFunc) *HttpClient

// WithRequestHookFunc request hook func
func (p *HttpClient) WithRequestHookFunc(option RequestHookFunc) *HttpClient

// WithResponseHookFunc response hook func
func (p *HttpClient) WithResponseHookFunc(option ResponseHookFunc) *HttpClient
```

## 自定义个性请求
可以自定义个性请求参数实例RequestOption(定义的参数，仅在当前请求生效)
自定义个性请求作为请求参数传入，如果不需要，传值为nil即可
### 构建RequestOption实例
```
// NewRequestOption 构建RequestOption实例
func NewRequestOption() *RequestOption
```

### 自定义个性请求Header
```
// WithHeader 单独添加Header参数
func (p *RequestOption) WithHeader(key string, val string) *RequestOption

// WithHeaders 批量添加Header参数
func (p * RequestOption) WithHeaders(headers map[string]string) *RequestOption

// WithReferer 配置Referer参数（调用WithHeader接口）
func (p * RequestOption) WithReferer(val string) *RequestOption 

// WithUserAgent 配置UserAgent参数（调用WithHeader接口）
func (p * RequestOption) WithUserAgent(val string) *RequestOption 

// WithContentType 配置ContentType参数（调用WithHeader接口）
func (p * RequestOption) WithContentType(val string) *RequestOption
```

### 自定义个性请求Cookie
```
// WithCookie 单独添加Cookie参数
func (p *RequestOption) WithCookie(cookie *http.Cookie) *HttpClient

// WithCookies 批量添加Cookie参数
func (p *RequestOption) WithCookies(cookies ...*http.Cookie) *HttpClient
```

### 自定义个性请求Option
```
// WithOption 单独添加Option参数
func (p *RequestOption) WithOption(key int, val interface{}) *RequestOption

// WithOptions 批量添加Option参数
func (p *RequestOption) WithOptions(options map[int]interface{}) *RequestOption

// WithConnectTimeout connect timeout option
func (p *RequestOption) WithConnectTimeout(timeout int) *RequestOption

// WithTimeout timeout option
func (p *RequestOption) WithTimeout(timeout int) *RequestOption

// WithProxyType enum: TRANS_PROXY_TYPE_HTTP
func (p *RequestOption) WithProxyType(proxyType int) *RequestOption

// WithProxyAddress proxy address: ip:port
func (p *RequestOption) WithProxyAddress(addr string) *RequestOption

// WithProxyFunc proxy func
func (p *RequestOption) WithProxyFunc(option ProxyFunc) *RequestOption

// WithUnsafeTls https TLS
func (p *RequestOption) WithUnsafeTls(unsafe bool) *RequestOption

// WithRetryTransOption retry trans option
func (p *RequestOption) WithRetryTransOption(option *RetryTransOption) *RequestOption

// WithLogTransOption log trans option
func (p *RequestOption) WithLogTransOption(option *LogTransOption) *RequestOption

// WithCookieJar cookie jar
func (p *RequestOption) WithCookieJar(jar http.CookieJar) *RequestOption

// WithRedirectPolicy redirect policy
func (p *RequestOption) WithRedirectPolicy(option RedirectPolicyFunc) *RequestOption

// WithRequestHookFunc request hook func
func (p *RequestOption) WithRequestHookFunc(option RequestHookFunc) *RequestOption

// WithResponseHookFunc response hook func
func (p *RequestOption) WithResponseHookFunc(option ResponseHookFunc) *RequestOption
```

## 返回值Response
返回值支持对返回接口进行解析，获取状态码及返回值(仅解析状态码为200的返回)
```
// Response得到状态码及Body内容（[]byte格式）
func (p *Response) ToBytes() (int, []byte, error)

// Response得到状态码及Body内容（string格式）
func (p *Response) ToString() (int, string, error)
```

## 错误重试选项
### 构建RetryTransOption实例，并通过WithRetryTransOption接口设置
```
// NewRetryTransOption 构建RetryTransOption实例
func NewRetryTransOption() *RetryTransOption
```

### 自定义错误重试选项参数
```
// WithMaxCount 最大重试次数
func (p *RetryTransOption) WithMaxCount(maxCount int) *RetryTransOption

// WithWaitTime  重试等待时间
func (p *RetryTransOption) WithWaitTime(minWaitTime, maxWaitTime time.Duration) *RetryTransOption

// WithCheckRetry 重试验证策略
func (p *RetryTransOption) WithCheckRetry(checkRetryFunc CheckRetryFunc) *RetryTransOption

// WithBackoff 重试Backoff策略
func (p *RetryTransOption) WithBackoff(backoffFunc BackoffFunc) *RetryTransOption

// WithRetryError 重试错误Hook函数
func (p *RetryTransOption) WithRetryError(retryErrorFunc RetryErrorFunc) *RetryTransOption
```

## 日志选项
### 构建LogTransOption实例，并通过WithLogTransOption接口设置
```
// NewLogTransOption 构建LogTransOption实例
func NewLogTransOption() *LogTransOption
```

### 自定义日志选项参数
```
// WithSlowLog 是否开启慢查询，及慢查询时间阈值
func (p *LogTransOption) WithSlowLog(enableSlowLog bool, slowLatency time.Duration) *LogTransOption

// IgnoreNotFound 慢查询是否忽略404请求
func (p *LogTransOption) IgnoreNotFound(ignoreNotFound bool) *LogTransOption

// WithAccessLog 是否开启Access Log
func (p *LogTransOption) WithAccessLog(enableAccessLog bool) *LogTransOption

// IncludeHeaders  Access Log 是否包含Header信息
func (p *LogTransOption) IncludeHeaders(includeHeaders bool) *LogTransOption
```

## 使用方式
### 默认实例进行Http请求
```
import (
  "context"
  
  "dev.rcrai.com/rcrai/huskar2/http"
)

func main() {
  ctx := context.Background() 

  response, err := httpclient.Get(ctx, "https://www.baidu.com", nil, nil)
  if err != nil {
      return
  }
  
  status, data := response.ToBytes()
  
  // TODO
}
```

### 新建实例进行Http请求
```
import (
  "context"
  
  "dev.rcrai.com/rcrai/huskar2/http"
)

func main() {
  ctx := context.Background() 

  httpClient := httpclient.NewHttpClient()

  response, err := httpClient.Get(ctx, "https://www.baidu.com", nil, nil)
  if err != nil {
      return
  }

  status, data := response.ToBytes()
  
  // TODO
}
```

### 新建实例开启访问日志
```
import (
  "context"
  
  "dev.rcrai.com/rcrai/huskar2/http"
)

func main() {
  ctx := context.Background() 

  httpClient := httpclient.NewHttpClient()

  logTransOption := httpclient.LogTransOption{}

  logTransOption.WithAccessLog(true)

  httpClient.WithLogTransOption(logTransOption)

  response, err := httpClient.Get(ctx, "https://www.baidu.com", nil, nil)
  if err != nil {
      return
  }

  status, data := response.ToBytes()
  
  // TODO
}
```