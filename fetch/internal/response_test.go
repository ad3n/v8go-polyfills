package internal

import (
	"errors"
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
	}, "https://example.com", false, -1)
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

func TestHandleHTTPResponseRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	response := &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader("12345")),
		ContentLength: 5,
	}
	result, err := HandleHttpResponse(response, "https://example.com", false, 4)
	if !errors.Is(err, ErrResponseBodyTooLarge) {
		t.Fatalf("error = %v, want %v", err, ErrResponseBodyTooLarge)
	}
	if result != nil {
		t.Fatalf("result = %+v, want nil", result)
	}
}

func TestHandleHTTPResponseRejectsKnownOversizeWithoutReadingBody(t *testing.T) {
	t.Parallel()

	body := &benchmarkBody{data: []byte("12345")}
	result, err := HandleHttpResponse(&http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Header:        make(http.Header),
		Body:          body,
		ContentLength: 5,
	}, "https://example.com", false, 4)
	if !errors.Is(err, ErrResponseBodyTooLarge) {
		t.Fatalf("error = %v, want %v", err, ErrResponseBodyTooLarge)
	}
	if result != nil {
		t.Fatalf("result = %+v, want nil", result)
	}
	if body.offset != 0 {
		t.Fatalf("read %d body bytes, want 0", body.offset)
	}
}

func TestHandleHTTPResponseAllowsDeclaredLengthForBodylessResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		statusCode int
	}{
		{name: "HEAD", method: http.MethodHead, statusCode: http.StatusOK},
		{name: "no content", method: http.MethodGet, statusCode: http.StatusNoContent},
		{name: "not modified", method: http.MethodGet, statusCode: http.StatusNotModified},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := HandleHttpResponse(&http.Response{
				Status:        http.StatusText(tt.statusCode),
				StatusCode:    tt.statusCode,
				Header:        make(http.Header),
				Body:          io.NopCloser(strings.NewReader("")),
				ContentLength: 1 << 20,
				Request:       &http.Request{Method: tt.method},
			}, "https://example.com", false, 4)
			if err != nil {
				t.Fatal(err)
			}
			if response.Body != "" {
				t.Fatalf("body = %q, want empty", response.Body)
			}
		})
	}
}

func TestHandleHTTPResponseAllowsBodyAtExactLimit(t *testing.T) {
	t.Parallel()

	const payload = "1234"
	response, err := HandleHttpResponse(&http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader(payload)),
		ContentLength: int64(len(payload)),
	}, "https://example.com", false, int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if response.Body != payload {
		t.Fatalf("body = %q, want %q", response.Body, payload)
	}
}

func TestResponseRecorderBoundsMemory(t *testing.T) {
	t.Parallel()

	recorder := NewResponseRecorder(4)
	if _, err := recorder.Write([]byte("1234")); err != nil {
		t.Fatal(err)
	}
	if _, err := recorder.Write([]byte("5")); !errors.Is(err, ErrResponseBodyTooLarge) {
		t.Fatalf("error = %v, want %v", err, ErrResponseBodyTooLarge)
	}
	if got := recorder.body.Len(); got != 4 {
		t.Fatalf("retained body bytes = %d, want 4", got)
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
		result, err := HandleHttpResponse(response, "https://example.com", false, -1)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkResponse = result
	}
}

func BenchmarkHandleHTTPResponseRejectsKnownOversize(b *testing.B) {
	payload := []byte(strings.Repeat("x", 1<<20))
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
		result, err := HandleHttpResponse(response, "https://example.com", false, 64<<10)
		if !errors.Is(err, ErrResponseBodyTooLarge) {
			b.Fatalf("error = %v, want %v", err, ErrResponseBodyTooLarge)
		}
		if result != nil {
			b.Fatalf("result = %+v, want nil", result)
		}
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
