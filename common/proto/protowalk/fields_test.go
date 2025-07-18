// Copyright 2022 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package protowalk

import (
	"fmt"
	"runtime"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type CustomChecker struct{}

var _ FieldProcessor = CustomChecker{}

func (CustomChecker) Process(field protoreflect.FieldDescriptor, msg protoreflect.Message) (data ResultData, applied bool) {
	chk := proto.GetExtension(field.Options().(*descriptorpb.FieldOptions), E_Custom).(*CustomExt)
	s := msg.Get(field).String()
	if applied = s != chk.MustEqual; applied {
		data = ResultData{Message: fmt.Sprintf("%q doesn't equal %q", s, chk.MustEqual)}
	}
	return
}

func (CustomChecker) ShouldProcess(field protoreflect.FieldDescriptor) ProcessAttr {
	if fo := field.Options().(*descriptorpb.FieldOptions); fo != nil {
		if proto.GetExtension(fo, E_Custom).(*CustomExt) != nil {
			return ProcessAlways
		}
	}
	return ProcessNever
}

func TestFields(t *testing.T) {
	t.Parallel()

	ftt.Run(`Fields`, t, func(t *ftt.Test) {

		t.Run(`fails when constructed without NewWalker`, func(t *ftt.Test) {
			var walker Walker[*Outer]
			assert.Loosely(t, walker.Execute(&Outer{}).Empty(), should.BeTrue)
		})

		t.Run(`for Outer x DeprecatedProcessor`, func(t *ftt.Test) {
			walker := NewWalker[*Outer](&DeprecatedProcessor{})

			t.Run(`works on populated messages`, func(t *ftt.Test) {
				msg := &Outer{Deprecated: "extra"}
				assert.Loosely(t, walker.Execute(msg).Strings(), should.Match([]string{
					".deprecated: deprecated",
				}))
			})

			t.Run(`works on nested populated messages`, func(t *ftt.Test) {
				msg := &Outer{SingleInner: &Inner{Deprecated: "extra"}}
				assert.Loosely(t, walker.Execute(msg).Strings(), should.Match([]string{
					".single_inner.deprecated: deprecated",
				}))
			})

			t.Run(`works on maps`, func(t *ftt.Test) {
				msg := &Outer{
					MapInner: map[string]*Inner{
						"something": {Deprecated: "yo"},
					},
					IntMapInner: map[int32]*Inner{
						20: {Deprecated: "hay"},
					},
				}
				assert.Loosely(t, walker.Execute(msg).Strings(), should.Match([]string{
					".map_inner[\"something\"].deprecated: deprecated",
					".int_map_inner[20].deprecated: deprecated",
				}))
			})

			t.Run(`works on lists`, func(t *ftt.Test) {
				msg := &Outer{
					MultiInner: []*Inner{
						{},
						{},
						{Deprecated: "hay"},
					},
				}
				assert.Loosely(t, walker.Execute(msg).Strings(), should.Match([]string{
					".multi_inner[2].deprecated: deprecated",
				}))
			})

		})

		t.Run(`works with custom check`, func(t *ftt.Test) {
			msg := &Outer{
				MultiInner: []*Inner{
					{Custom: "neat"},
					{},
					{Custom: "hello"},
					{},
				},
			}
			walker := NewWalker[*Outer](CustomChecker{})
			assert.Loosely(t, walker.Execute(msg).Strings(), should.Match([]string{
				`.custom: "" doesn't equal "hello"`,
				`.multi_inner[0].custom: "neat" doesn't equal "hello"`,
				`.multi_inner[1].custom: "" doesn't equal "hello"`,
				// 2 is OK!
				`.multi_inner[3].custom: "" doesn't equal "hello"`,
			}))

		})
	})
}

// goos: darwin
// goarch: amd64
// pkg: go.chromium.org/luci/common/proto/protowalk
// cpu: Intel(R) Core(TM) i9-9980HK CPU @ 2.40GHz
// BenchmarkNewWalker-16    	   60345	     19799 ns/op	    9830 B/op	      90 allocs/op
func BenchmarkNewWalker(b *testing.B) {
	b.ReportAllocs()

	for range b.N {
		runtime.KeepAlive(NewWalker[*Outer](CustomChecker{}))
	}
}

// goos: darwin
// goarch: amd64
// pkg: go.chromium.org/luci/common/proto/protowalk
// cpu: Intel(R) Core(TM) i9-9980HK CPU @ 2.40GHz
// BenchmarkWalkerExecute_Complex-16    	   26097	     45459 ns/op	   18550 B/op	     274 allocs/op
func BenchmarkWalkerExecute_Complex(b *testing.B) {
	b.ReportAllocs()
	msg := &Outer{
		Deprecated: "hey",
		SingleInner: &Inner{
			Regular:    "things",
			Deprecated: "yo",
		},
		MapInner: map[string]*Inner{
			"schwoot": {
				Deprecated: "thing",
				SingleEmbed: &Inner_Embedded{
					Deprecated: "yarp",
				},
				MultiEmbed: []*Inner_Embedded{
					{Deprecated: "yay"},
					{Regular: "ignore"},
				},
			},
			"nerps": {
				Deprecated:  "thing",
				SingleEmbed: &Inner_Embedded{},
				MultiEmbed: []*Inner_Embedded{
					{},
				},
			},
		},
		MultiDeprecated: []*Inner{
			{Regular: "something"},
			{Deprecated: "something else"},
		},
		MultiInner: []*Inner{
			{Custom: "neat"},
			{},
			{Custom: "hello"},
			{},
		},
	}
	checkers := []FieldProcessor{
		DeprecatedProcessor{},
		RequiredProcessor{},
		CustomChecker{},
	}
	cache := NewWalker[*Outer](checkers...)
	expect := []string{
		`.deprecated: deprecated`,
		`.single_inner.deprecated: deprecated`,
		`.map_inner["nerps"].deprecated: deprecated`,
		`.map_inner["schwoot"].deprecated: deprecated`,
		`.map_inner["schwoot"].single_embed.deprecated: deprecated`,
		`.map_inner["schwoot"].multi_embed[0].deprecated: deprecated`,
		`.multi_deprecated: deprecated`,
		`.multi_deprecated[1].deprecated: deprecated`,
		`.req: required`,
		`.single_inner.req: required`,
		`.multi_inner[0].req: required`,
		`.multi_inner[1].req: required`,
		`.multi_inner[2].req: required`,
		`.multi_inner[3].req: required`,
		`.map_inner["nerps"].req: required`,
		`.map_inner["nerps"].single_embed.req: required`,
		`.map_inner["nerps"].multi_embed[0].req: required`,
		`.map_inner["schwoot"].req: required`,
		`.map_inner["schwoot"].single_embed.req: required`,
		`.map_inner["schwoot"].multi_embed[0].req: required`,
		`.map_inner["schwoot"].multi_embed[1].req: required`,
		`.multi_deprecated[0].req: required`,
		`.multi_deprecated[1].req: required`,
		`.custom: "" doesn't equal "hello"`,
		`.single_inner.custom: "" doesn't equal "hello"`,
		`.multi_inner[0].custom: "neat" doesn't equal "hello"`,
		`.multi_inner[1].custom: "" doesn't equal "hello"`,
		`.multi_inner[3].custom: "" doesn't equal "hello"`,
		`.map_inner["nerps"].custom: "" doesn't equal "hello"`,
		`.map_inner["nerps"].single_embed.custom: "" doesn't equal "hello"`,
		`.map_inner["nerps"].multi_embed[0].custom: "" doesn't equal "hello"`,
		`.map_inner["schwoot"].custom: "" doesn't equal "hello"`,
		`.map_inner["schwoot"].single_embed.custom: "" doesn't equal "hello"`,
		`.map_inner["schwoot"].multi_embed[0].custom: "" doesn't equal "hello"`,
		`.map_inner["schwoot"].multi_embed[1].custom: "" doesn't equal "hello"`,
		`.multi_deprecated[0].custom: "" doesn't equal "hello"`,
		`.multi_deprecated[1].custom: "" doesn't equal "hello"`,
	}
	b.StopTimer()
	b.ResetTimer()

	for i := range b.N {
		b.StartTimer()
		reports := cache.Execute(msg)
		b.StopTimer()
		for ridx, report := range reports.Strings() {
			if report != expect[ridx] {
				b.Errorf("iter[%d]: report[%d]: %q != %q", i, ridx, report, expect[ridx])
			}
		}
	}
}

type NoopChecker struct{}

var _ FieldProcessor = NoopChecker{}

func (NoopChecker) Process(field protoreflect.FieldDescriptor, msg protoreflect.Message) (data ResultData, applied bool) {
	return
}

func (NoopChecker) ShouldProcess(field protoreflect.FieldDescriptor) ProcessAttr {
	return ProcessNever
}

// goos: darwin
// goarch: amd64
// pkg: go.chromium.org/luci/common/proto/protowalk
// cpu: Intel(R) Core(TM) i9-9980HK CPU @ 2.40GHz
// BenchmarkNoop-16    	132972100	         9.302 ns/op	       0 B/op	       0 allocs/op
func BenchmarkNoop(b *testing.B) {
	b.ReportAllocs()
	msg := &Outer{
		Deprecated: "hey",
		SingleInner: &Inner{
			Regular:    "things",
			Deprecated: "yo",
		},
		MapInner: map[string]*Inner{
			"schwoot": {
				Deprecated: "thing",
				SingleEmbed: &Inner_Embedded{
					Deprecated: "yarp",
				},
				MultiEmbed: []*Inner_Embedded{
					{Deprecated: "yay"},
					{Regular: "ignore"},
				},
			},
			"nerps": {
				Deprecated:  "thing",
				SingleEmbed: &Inner_Embedded{},
				MultiEmbed: []*Inner_Embedded{
					{},
				},
			},
		},
		MultiDeprecated: []*Inner{
			{Regular: "something"},
			{Deprecated: "something else"},
		},
		MultiInner: []*Inner{
			{Custom: "neat"},
			{},
			{Custom: "hello"},
			{},
		},
	}
	cache := NewWalker[*Outer](NoopChecker{})
	b.ResetTimer()

	for range b.N {
		// this is fast enough that we can't start/stop the timer, as it dominates
		// the runtime and makes the benchmark timeout after 10 minutes. Instead,
		// just run a tight loop with KeepAlive to prevent the compiler from
		// optimizing away `cache.Execute` entriely.
		runtime.KeepAlive(cache.Execute(msg))
	}
}

// Custom2Checker is the simplest possible checker - Despite the complexity in
// Outer, there is only one field in Inner which is marked with this extension.
type Custom2Checker struct{}

var _ FieldProcessor = Custom2Checker{}

func (Custom2Checker) Process(field protoreflect.FieldDescriptor, msg protoreflect.Message) (data ResultData, applied bool) {
	data = ResultData{Message: "found"}
	applied = true
	return
}

func (Custom2Checker) ShouldProcess(field protoreflect.FieldDescriptor) ProcessAttr {
	if fo := field.Options().(*descriptorpb.FieldOptions); fo != nil {
		if proto.GetExtension(fo, E_Custom2).(*Custom2Ext) != nil {
			return ProcessAlways
		}
	}
	return ProcessNever
}

// goos: darwin
// goarch: amd64
// pkg: go.chromium.org/luci/common/proto/protowalk
// cpu: Intel(R) Core(TM) i9-9980HK CPU @ 2.40GHz
// BenchmarkOne-16    	  553950	      2165 ns/op	    1288 B/op	      19 allocs/op
func BenchmarkOne(b *testing.B) {
	b.ReportAllocs()
	msg := &Outer{
		SingleInner: &Inner{
			Custom2: "hi",
		},
	}
	cache := NewWalker[*Outer](Custom2Checker{})
	b.ResetTimer()

	for range b.N {
		// this is fast enough that we can't start/stop the timer, as it dominates
		// the runtime and makes the benchmark timeout after 10 minutes. Instead,
		// just run a tight loop with KeepAlive to prevent the compiler from
		// optimizing away `cache.Execute` entriely.
		if rslt := cache.Execute(msg); rslt == nil {
			panic(rslt)
		}
	}
}
