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

package internal

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

const responseCopyBufferSize = 32 << 10

var responseCopyBufferPool = sync.Pool{
	New: func() any {
		buffer := make([]byte, responseCopyBufferSize)
		return &buffer
	},
}

var ErrResponseBodyTooLarge = errors.New("fetch: response body exceeds configured limit")

/*
Response keeps the *http.Response
*/
type Response struct {
	Header     http.Header
	StatusText string
	URL        string
	Body       string
	Status     int32
	OK         bool
	Redirected bool
}

/*
Handle the *http.Response, return *Response
*/
func HandleHttpResponse(res *http.Response, url string, redirected bool, maxBodyBytes int64) (*Response, error) {
	defer res.Body.Close()
	if responseMayHaveBody(res) && maxBodyBytes >= 0 && res.ContentLength > maxBodyBytes {
		return nil, ErrResponseBodyTooLarge
	}

	var body strings.Builder
	const maxPreallocate = 1 << 20
	if res.ContentLength > 0 {
		grow := min(res.ContentLength, maxPreallocate)
		if maxBodyBytes >= 0 {
			grow = min(grow, maxBodyBytes)
		}
		body.Grow(int(grow))
	}
	buffer := responseCopyBufferPool.Get().(*[]byte)
	var total int64
	emptyReads := 0
	for {
		n, readErr := res.Body.Read(*buffer)
		if n < 0 || n > len(*buffer) {
			responseCopyBufferPool.Put(buffer)
			return nil, fmt.Errorf("fetch: invalid response body read count %d", n)
		}
		if n > 0 {
			emptyReads = 0
			writeBytes := int64(n)
			if maxBodyBytes >= 0 {
				writeBytes = min(writeBytes, maxBodyBytes-total)
			}
			if writeBytes > 0 {
				_, _ = body.Write((*buffer)[:writeBytes])
				total += writeBytes
			}
			if writeBytes < int64(n) {
				responseCopyBufferPool.Put(buffer)
				return nil, ErrResponseBodyTooLarge
			}
		} else if readErr == nil {
			emptyReads++
			if emptyReads >= 100 {
				responseCopyBufferPool.Put(buffer)
				return nil, io.ErrNoProgress
			}
		}
		if readErr != nil {
			responseCopyBufferPool.Put(buffer)
			if readErr != io.EOF {
				return nil, readErr
			}
			break
		}
	}

	return &Response{
		Header:     res.Header,
		Status:     int32(res.StatusCode), // int type is not support by v8go
		StatusText: res.Status,
		OK:         res.StatusCode >= 200 && res.StatusCode < 300,
		Redirected: redirected,
		URL:        url,
		Body:       body.String(),
	}, nil
}

func responseMayHaveBody(res *http.Response) bool {
	if res.Request != nil && res.Request.Method == http.MethodHead {
		return false
	}
	return res.StatusCode < 100 || res.StatusCode >= 200 && res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusNotModified
}

// ResponseRecorder is a bounded http.ResponseWriter for local fetch handlers.
type ResponseRecorder struct {
	header http.Header
	body   strings.Builder
	limit  int64
	status int
	err    error
}

func NewResponseRecorder(limit int64) *ResponseRecorder {
	return &ResponseRecorder{header: make(http.Header), limit: limit}
}

func (r *ResponseRecorder) Header() http.Header { return r.header }

func (r *ResponseRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *ResponseRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if r.err != nil {
		return 0, r.err
	}
	if r.limit >= 0 && int64(len(p)) > r.limit-int64(r.body.Len()) {
		r.err = ErrResponseBodyTooLarge
		return 0, r.err
	}
	return r.body.Write(p)
}

func (r *ResponseRecorder) Err() error { return r.err }

func (r *ResponseRecorder) Response(url string) *Response {
	status := r.status
	if status == 0 {
		status = http.StatusOK
	}
	return &Response{
		Header:     r.header.Clone(),
		Status:     int32(status),
		StatusText: fmt.Sprintf("%d %s", status, http.StatusText(status)),
		OK:         status >= 200 && status < 300,
		URL:        url,
		Body:       r.body.String(),
	}
}
