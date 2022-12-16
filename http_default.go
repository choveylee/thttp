/**
 * @Author: lidonglin
 * @Description:
 * @File:  http_default.go
 * @Version: 1.0.0
 * @Date: 2022/05/28 10:46
 */

package thttp

// default client for convenience
var defaultClient = NewHttpClient()

var Defaults = defaultClient.Defaults

var Debug = defaultClient.Debug

var WithOption = defaultClient.WithOption

var WithTimeout = defaultClient.WithTimeout
var WithConnectTimeout = defaultClient.WithConnectTimeout
var WithDeadlineTimeout = defaultClient.WithDeadlineTimeout

var WithProxyType = defaultClient.WithProxyType
var WithProxyAddress = defaultClient.WithProxyAddress
var WithProxyFunc = defaultClient.WithProxyFunc
var WithUnsafeTls = defaultClient.WithUnsafeTls

var WithRetryTransOption = defaultClient.WithRetryTransOption
var WithLogTransOption = defaultClient.WithLogTransOption

var WithCookieJar = defaultClient.WithCookieJar
var WithRedirectPolicy = defaultClient.WithRedirectPolicy

var WithRequestHookFunc = defaultClient.WithRequestHookFunc
var WithResponseHookFunc = defaultClient.WithResponseHookFunc

var WithOptions = defaultClient.WithOptions

var WithHeader = defaultClient.WithHeader
var WithReferer = defaultClient.WithReferer
var WithUserAgent = defaultClient.WithUserAgent
var WithContentType = defaultClient.WithContentType
var WithHeaders = defaultClient.WithHeaders

var Do = defaultClient.Do

var Head = defaultClient.Head
var Get = defaultClient.Get
var GetLen = defaultClient.GetLen

var Post = defaultClient.Post
var PostJson = defaultClient.PostJson
var PostMultipart = defaultClient.PostMultipart

var Put = defaultClient.Put
var PutJson = defaultClient.PutJson

var Patch = defaultClient.Patch
var PatchJson = defaultClient.PatchJson

var Delete = defaultClient.Delete

var Options = defaultClient.Options
var Connect = defaultClient.Connect
var Trace = defaultClient.Trace
