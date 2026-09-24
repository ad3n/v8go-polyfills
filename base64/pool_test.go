package base64

import (
	stdBase64 "encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/ad3n/v8go"
)

func BenchmarkBase64Callbacks(b *testing.B) {
	for _, size := range []int{16, 4096, 32768, 131072} {
		for _, operation := range []string{"btoa", "atob", "invalid"} {
			b.Run(fmt.Sprintf("%s/%d", operation, size), func(b *testing.B) {
				input := strings.Repeat("a", size)
				fn := operation
				if operation != "btoa" {
					input = stdBase64.StdEncoding.EncodeToString([]byte(input))
					fn = "atob"
				}

				if operation == "invalid" {
					input = input[:len(input)-1] + "!"
				}

				b.SetBytes(int64(size) * 128)
				b.ReportAllocs()
				for b.Loop() {
					runCallbackBatch(b, fn, input)
				}
			})
		}
	}
}

func runCallbackBatch(tb testing.TB, fn, input string) {
	iso := v8go.NewIsolate()
	defer iso.Dispose()

	global := v8go.NewObjectTemplate(iso)
	if err := InjectTo(iso, global); err != nil {
		tb.Fatal(err)
	}

	ctx := v8go.NewContext(iso, global)
	defer ctx.Close()

	value, err := v8go.NewValue(iso, input)
	if err != nil {
		tb.Fatal(err)
	}

	if err := ctx.Global().Set("input", value); err != nil {
		tb.Fatal(err)
	}

	if _, err := ctx.RunScript("for (let i = 0; i < 128; i++) "+fn+"(input)", "benchmark.js"); err != nil {
		tb.Fatal(err)
	}
}

func TestBase64BufferReuse(t *testing.T) {
	for _, size := range []int{0, 1, 1023, 1024, 1025, 49152, 49153, 65535, 65536, 65537, 131072} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			t.Parallel()

			iso := v8go.NewIsolate()
			defer iso.Dispose()

			global := v8go.NewObjectTemplate(iso)
			if err := InjectTo(iso, global); err != nil {
				t.Fatal(err)
			}

			ctx := v8go.NewContext(iso, global)
			defer ctx.Close()

			input := strings.Repeat("a", size)
			encoded := stdBase64.StdEncoding.EncodeToString([]byte(input))
			script := `const input = ` + strconv.Quote(input) + `;
const encoded = ` + strconv.Quote(encoded) + `;
const savedEncoded = btoa(input);
const savedDecoded = atob(encoded);
for (let i = 0; i < 16; i++) {
 if (btoa("b".repeat(input.length)) !== ` + strconv.Quote(stdBase64.StdEncoding.EncodeToString([]byte(strings.Repeat("b", size)))) + `) throw Error("encode");
 if (atob(encoded + "!") !== "") throw Error("invalid");
 if (atob(encoded) !== input) throw Error("decode");
}
if (savedEncoded !== encoded || savedDecoded !== input) throw Error("retained result changed");`
			if _, err := ctx.RunScript(script, "reuse.js"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func BenchmarkBase64CallbacksParallel(b *testing.B) {
	for _, operation := range []string{"btoa", "atob", "invalid"} {
		b.Run(operation, func(b *testing.B) {
			input := strings.Repeat("a", 4096)
			fn := operation
			if operation != "btoa" {
				input = stdBase64.StdEncoding.EncodeToString([]byte(input))
				fn = "atob"
			}

			if operation == "invalid" {
				input = input[:len(input)-1] + "!"
			}

			b.SetBytes(4096 * 128)
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					runCallbackBatch(b, fn, input)
				}
			})
		})
	}
}
