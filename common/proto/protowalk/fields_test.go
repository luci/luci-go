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
	"sort"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type CustomChecker struct{}

var _ FieldProcessor = (*CustomChecker)(nil)

func (*CustomChecker) Process(field protoreflect.FieldDescriptor, msg protoreflect.Message) (data ResultData, applied bool) {
	chk := proto.GetExtension(field.Options().(*descriptorpb.FieldOptions), E_Custom).(*CustomExt)
	s := msg.Get(field).String()
	if applied = s != chk.MustEqual; applied {
		data = ResultData{Message: fmt.Sprintf("%q doesn't equal %q", s, chk.MustEqual)}
	}
	return
}

func init() {
	RegisterFieldProcessor(&CustomChecker{}, func(field protoreflect.FieldDescriptor) ProcessAttr {
		if fo := field.Options().(*descriptorpb.FieldOptions); fo != nil {
			if proto.GetExtension(fo, E_Custom).(*CustomExt) != nil {
				return ProcessAlways
			}
		}
		return ProcessNever
	})
}

func TestFields(t *testing.T) {
	// These tests interact with globalFieldProcessorCache
	// Hence, no t.Parallel()

	ftt.Run(`Fields`, t, func(t *ftt.Test) {
		// Reset the cache
		resetGlobalFieldProcessorCache()

		t.Run(`works with no FieldProcessors`, func(t *ftt.Test) {
			msg := &Outer{}
			assert.Loosely(t, Fields(msg), should.BeEmpty)
			assert.Loosely(t, globalFieldProcessorCache, should.BeEmpty)
		})

		t.Run(`works with one processor on empty message`, func(t *ftt.Test) {
			msg := &Outer{}
			assert.Loosely(t, Fields(msg, &DeprecatedProcessor{}), should.Resemble(Results{nil}))
			keys := make([]string, 0, len(globalFieldProcessorCache))
			for k := range globalFieldProcessorCache {
				keys = append(keys, fmt.Sprintf("%s+%s", k.message, k.processorT))
			}
			sort.Strings(keys)
			assert.Loosely(t, keys, should.Resemble([]string{
				"protowalk.Inner+*protowalk.DeprecatedProcessor",
				"protowalk.Inner.Embedded+*protowalk.DeprecatedProcessor",
				"protowalk.Outer+*protowalk.DeprecatedProcessor",
			}))
		})

		t.Run(`works on populated messages`, func(t *ftt.Test) {
			msg := &Outer{Deprecated: "extra"}
			assert.Loosely(t, Fields(msg, &DeprecatedProcessor{}).Strings(), should.Resemble([]string{
				".deprecated: deprecated",
			}))
		})

		t.Run(`works on nested populated messages`, func(t *ftt.Test) {
			msg := &Outer{SingleInner: &Inner{Deprecated: "extra"}}
			assert.Loosely(t, Fields(msg, &DeprecatedProcessor{}).Strings(), should.Resemble([]string{
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
			assert.Loosely(t, Fields(msg, &DeprecatedProcessor{}).Strings(), should.Resemble([]string{
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
			assert.Loosely(t, Fields(msg, &DeprecatedProcessor{}).Strings(), should.Resemble([]string{
				".multi_inner[2].deprecated: deprecated",
			}))
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
			assert.Loosely(t, Fields(msg, &CustomChecker{}).Strings(), should.Resemble([]string{
				`.custom: "" doesn't equal "hello"`,
				`.multi_inner[0].custom: "neat" doesn't equal "hello"`,
				`.multi_inner[1].custom: "" doesn't equal "hello"`,
				// 2 is OK!
				`.multi_inner[3].custom: "" doesn't equal "hello"`,
			}))

		})
	})
}

func BenchmarkCache(b *testing.B) {
	b.ReportAllocs()
	desc := (&Outer{}).ProtoReflect().Descriptor()
	checker := &CustomChecker{}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		generateCacheEntry(desc, lookupProcBundles(checker)[0], stringset.New(0), map[string]*cacheEntryBuilder{})
	}
}

func BenchmarkFields(b *testing.B) {
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
		&DeprecatedProcessor{},
		&RequiredProcessor{},
		&CustomChecker{},
	}
	for _, chk := range checkers {
		setCacheEntry(msg.ProtoReflect().Descriptor(), lookupProcBundles(chk)[0], stringset.New(0), map[string]*cacheEntryBuilder{})
	}
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
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		reports := Fields(msg, checkers...)
		b.StopTimer()
		for ridx, report := range reports.Strings() {
			if report != expect[ridx] {
				b.Errorf("iter[%d]: report[%d]: %q != %q", i, ridx, report, expect[ridx])
			}
		}
		b.StartTimer()
	}
}
