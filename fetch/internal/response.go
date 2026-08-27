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
func HandleHttpResponse(res *http.Response, url string, redirected bool) (*Response, error) {
	defer res.Body.Close()

	var body strings.Builder
	const maxPreallocate = 1 << 20
	if res.ContentLength > 0 {
		body.Grow(int(min(res.ContentLength, maxPreallocate)))
	}
	buffer := responseCopyBufferPool.Get().(*[]byte)
	for {
		n, readErr := res.Body.Read(*buffer)
		if n > 0 {
			_, _ = body.Write((*buffer)[:n])
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
