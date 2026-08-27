/*
 * Copyright (c) 2021 Xingwang Liao
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package fetch

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ad3n/v8go-polyfills/fetch/internal"
	core "github.com/ad3n/v8go-polyfills/internal"

	"github.com/ad3n/v8go"
)

const (
	UserAgentLocal              = "<local>"
	AddrLocal                   = "0.0.0.0:0"
	DefaultMaxResponseBodyBytes = 32 << 20
)

var defaultLocalHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
})

var defaultUserAgentProvider = UserAgentProviderFunc(func(u *url.URL) string {
	if !u.IsAbs() {
		return UserAgentLocal
	}

	return UserAgent()
})

var userAgent = "v8go-polyfills/" + core.Version + " (v8go/" + v8go.Version() + ")"

type Fetcher interface {
	GetLocalHandler() http.Handler

	GetFetchFunctionCallback() v8go.FunctionCallback
}

type fetcher struct {
	// Use local handler to handle the relative path (starts with "/") request
	LocalHandler http.Handler

	UserAgentProvider    UserAgentProvider
	client               *http.Client
	AddrLocal            string
	MaxResponseBodyBytes int64
}

func NewFetcher(opt ...Option) Fetcher {
	ft := &fetcher{
		LocalHandler:         defaultLocalHandler,
		UserAgentProvider:    defaultUserAgentProvider,
		AddrLocal:            AddrLocal,
		MaxResponseBodyBytes: DefaultMaxResponseBodyBytes,
		client: &http.Client{
			Transport: http.DefaultTransport,
			Timeout:   20 * time.Second,
		},
	}

	for _, o := range opt {
		o.apply(ft)
	}

	return ft
}

func (f *fetcher) GetLocalHandler() http.Handler {
	return f.LocalHandler
}

func (f *fetcher) GetFetchFunctionCallback() v8go.FunctionCallback {
	return func(info *v8go.FunctionCallbackInfo) *v8go.Value {
		ctx := info.Context()
		args := info.Args()

		resolver, _ := v8go.NewPromiseResolver(ctx)

		go func() {
			if len(args) == 0 {
				err := errors.New("1 argument required, but only 0 present")
				resolver.Reject(newErrorValue(ctx, err))
				return
			}

			var reqInit internal.RequestInit
			if len(args) > 1 {
				str, err := v8go.JSONStringify(ctx, args[1])
				if err != nil {
					resolver.Reject(newErrorValue(ctx, err))
					return
				}

				reader := strings.NewReader(str)
				if err := json.NewDecoder(reader).Decode(&reqInit); err != nil {
					resolver.Reject(newErrorValue(ctx, err))
					return
				}
			}

			r, err := f.initRequest(args[0].String(), reqInit)
			if err != nil {
				resolver.Reject(newErrorValue(ctx, err))
				return
			}

			var res *internal.Response

			// do local request
			if !r.URL.IsAbs() {
				res, err = f.fetchLocal(r)
			} else {
				res, err = f.fetchRemote(r)
			}
			if err != nil {
				resolver.Reject(newErrorValue(ctx, err))
				return
			}

			resObj, err := newResponseObject(ctx, res)
			if err != nil {
				resolver.Reject(newErrorValue(ctx, err))
				return
			}

			resolver.Resolve(resObj)
		}()

		return resolver.GetPromise().Value
	}
}

func (f *fetcher) initRequest(reqUrl string, reqInit internal.RequestInit) (*internal.Request, error) {
	u, err := internal.ParseRequestURL(reqUrl)
	if err != nil {
		return nil, err
	}

	req := &internal.Request{
		URL:  u,
		Body: reqInit.Body,
		Header: http.Header{
			"Accept": {"*/*"},
		},
	}

	var ua string
	if f.UserAgentProvider != nil {
		ua = f.UserAgentProvider.GetUserAgent(u)
	} else {
		ua = defaultUserAgentProvider(u)
	}

	req.Header.Set("User-Agent", ua)

	// url has no scheme, its a local request
	if !u.IsAbs() {
		req.RemoteAddr = f.AddrLocal
	}

	for h, v := range reqInit.Headers {
		headerName := http.CanonicalHeaderKey(h)
		req.Header.Set(headerName, v)
	}

	if reqInit.Method != "" {
		req.Method = strings.ToUpper(reqInit.Method)
	} else {
		req.Method = http.MethodGet
	}

	switch r := strings.ToLower(reqInit.Redirect); r {
	case "error", "follow", "manual":
		req.Redirect = r
	case "":
		req.Redirect = internal.RequestRedirectFollow
	default:
		return nil, fmt.Errorf("unsupported redirect %q", reqInit.Redirect)
	}

	return req, nil
}

func (f *fetcher) fetchLocal(r *internal.Request) (*internal.Response, error) {
	if f.LocalHandler == nil {
		return nil, errors.New("no local handler present")
	}

	var body io.Reader
	if r.Method != http.MethodGet {
		body = strings.NewReader(r.Body)
	}

	req, err := http.NewRequest(r.Method, r.URL.String(), body)
	if err != nil {
		return nil, err
	}
	req.RemoteAddr = r.RemoteAddr
	req.Header = r.Header

	recorder := internal.NewResponseRecorder(f.MaxResponseBodyBytes)
	f.LocalHandler.ServeHTTP(recorder, req)
	if err := recorder.Err(); err != nil {
		return nil, err
	}

	return recorder.Response(r.URL.String()), nil
}

func (f *fetcher) fetchRemote(r *internal.Request) (*internal.Response, error) {
	var body io.Reader
	if r.Method != http.MethodGet {
		body = strings.NewReader(r.Body)
	}

	req, err := http.NewRequest(r.Method, r.URL.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header = r.Header

	var redirected atomic.Bool

	client := *f.client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		switch r.Redirect {
		case internal.RequestRedirectError:
			return errors.New("redirects are not allowed")
		case internal.RequestRedirectManual:
			return http.ErrUseLastResponse
		default:
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
		}

		redirected.Store(true)
		return nil
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	return internal.HandleHttpResponse(res, r.URL.String(), redirected.Load(), f.MaxResponseBodyBytes)
}

func newResponseObject(ctx *v8go.Context, res *internal.Response) (*v8go.Object, error) {
	iso := ctx.Isolate()

	headers, err := newHeadersObject(ctx, res.Header)
	if err != nil {
		return nil, err
	}

	textFnTmp := v8go.NewFunctionTemplate(iso, func(info *v8go.FunctionCallbackInfo) *v8go.Value {
		ctx := info.Context()
		resolver, _ := v8go.NewPromiseResolver(ctx)

		v, _ := v8go.NewValue(iso, res.Body)
		resolver.Resolve(v)

		return resolver.GetPromise().Value
	})

	jsonFnTmp := v8go.NewFunctionTemplate(iso, func(info *v8go.FunctionCallbackInfo) *v8go.Value {
		ctx := info.Context()

		resolver, _ := v8go.NewPromiseResolver(ctx)

		val, err := v8go.JSONParse(ctx, res.Body)
		if err != nil {
			rejectVal, _ := v8go.NewValue(iso, err.Error())
			resolver.Reject(rejectVal)
			return resolver.GetPromise().Value
		}

		resolver.Resolve(val)

		return resolver.GetPromise().Value
	})

	resTmp := v8go.NewObjectTemplate(iso)

	for _, f := range [...]struct {
		Name string
		Tmp  any
	}{
		{Name: "text", Tmp: textFnTmp},
		{Name: "json", Tmp: jsonFnTmp},
	} {
		if err := resTmp.Set(f.Name, f.Tmp, v8go.ReadOnly); err != nil {
			return nil, err
		}
	}

	resObj, err := resTmp.NewInstance(ctx)
	if err != nil {
		return nil, err
	}

	for _, v := range [...]struct {
		Key string
		Val any
	}{
		{Key: "headers", Val: headers},
		{Key: "ok", Val: res.OK},
		{Key: "redirected", Val: res.Redirected},
		{Key: "status", Val: res.Status},
		{Key: "statusText", Val: res.StatusText},
		{Key: "url", Val: res.URL},
		{Key: "body", Val: res.Body},
	} {
		if err := resObj.Set(v.Key, v.Val); err != nil {
			return nil, err
		}
	}

	return resObj, nil
}

func newHeadersObject(ctx *v8go.Context, h http.Header) (*v8go.Object, error) {
	iso := ctx.Isolate()

	// https://developer.mozilla.org/en-US/docs/Web/API/Headers/get
	getFnTmp := v8go.NewFunctionTemplate(iso, func(info *v8go.FunctionCallbackInfo) *v8go.Value {
		args := info.Args()
		if len(args) == 0 {
			// TODO: this should return an error, but v8go not supported now
			val, _ := v8go.NewValue(iso, "")
			return val
		}

		val, _ := v8go.NewValue(iso, h.Get(args[0].String()))
		return val
	})

	// https://developer.mozilla.org/en-US/docs/Web/API/Headers/has
	hasFnTmp := v8go.NewFunctionTemplate(iso, func(info *v8go.FunctionCallbackInfo) *v8go.Value {
		args := info.Args()
		if len(args) == 0 {
			val, _ := v8go.NewValue(iso, false)
			return val
		}
		val, _ := v8go.NewValue(iso, h.Get(args[0].String()) != "")
		return val
	})

	// create a header template,
	// TODO: if v8go supports Map in the future, change this to a Map Object
	headersTmp := v8go.NewObjectTemplate(iso)

	for _, f := range [...]struct {
		Name string
		Tmp  any
	}{
		{Name: "get", Tmp: getFnTmp},
		{Name: "has", Tmp: hasFnTmp},
	} {
		if err := headersTmp.Set(f.Name, f.Tmp, v8go.ReadOnly); err != nil {
			return nil, err
		}
	}

	headers, err := headersTmp.NewInstance(ctx)
	if err != nil {
		return nil, err
	}

	for k, v := range h {
		var vv string
		if len(v) > 0 {
			// get the first element, like http.Header.Get
			vv = v[0]
		}

		if err := headers.Set(k, vv); err != nil {
			return nil, err
		}
	}

	return headers, nil
}

// v8go currently not support reject a *v8go.Object,
// so we should new *v8go.Value here
func newErrorValue(ctx *v8go.Context, err error) *v8go.Value {
	iso := ctx.Isolate()
	e, _ := v8go.NewValue(iso, "fetch: "+err.Error())
	return e
}

func UserAgent() string {
	return userAgent
}
