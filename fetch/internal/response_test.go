package internal

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

var benchmarkResponse *Response

func TestHandleHTTPResponse(t *testing.T) {
	t.Parallel()

	const payload = `{"ok":true}`
	response, err := HandleHttpResponse(&http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": {"application/json"}},
		Body:          io.NopCloser(strings.NewReader(payload)),
		ContentLength: int64(len(payload)),
	}, "https://example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	if response.Body != payload {
		t.Fatalf("body = %q, want %q", response.Body, payload)
	}
	if !response.OK || response.Status != http.StatusOK {
		t.Fatalf("unexpected response status: %+v", response)
	}
}

func BenchmarkHandleHTTPResponse(b *testing.B) {
	payload := []byte(strings.Repeat("x", 4<<10))
	body := &benchmarkBody{data: payload}
	response := &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Header:        make(http.Header),
		Body:          body,
		ContentLength: int64(len(payload)),
	}

	b.ReportAllocs()
	for b.Loop() {
		body.offset = 0
		result, err := HandleHttpResponse(response, "https://example.com", false)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkResponse = result
	}
}

type benchmarkBody struct {
	data   []byte
	offset int
}

func (r *benchmarkBody) Read(dst []byte) (int, error) {
	if r.offset == len(r.data) {
		return 0, io.EOF
	}
	n := copy(dst, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func (*benchmarkBody) Close() error { return nil }
