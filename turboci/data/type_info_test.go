// Copyright 2026 The LUCI Authors.
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

package data

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

func TestMakeTypeMatcher(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string

		urls    []string
		wantErr string
		wantN   int

		matches []string
		rejects []string
	}{
		{
			name:    "empty",
			rejects: []string{"hi", URL[*emptypb.Empty]()},
		},
		{
			name: "static",
			urls: []string{
				URL[*emptypb.Empty](),
				URL[*structpb.Value](),
			},
			wantN: 2,
			matches: []string{
				URL[*emptypb.Empty](),
				URL[*structpb.Value](),
			},
			rejects: []string{
				"hi",
				URL[*structpb.ListValue](),
				// This will produce e.g. `google_protobuf_Empty`; if we don't
				// correctly escape the metachars in the regex, then our pattern for
				// `google.protobuf.Empty` would match accidentally.
				strings.ReplaceAll(URL[*emptypb.Empty](), ".", "_"),
			},
		},
		{
			name: "wildcard_package",
			urls: []string{
				URLPatternPackageOf[*structpb.Value](),
			},
			wantN: 1,
			matches: []string{
				URL[*structpb.Value](),
				URL[*structpb.ListValue](),
				URL[*structpb.Struct](),
				URL[*emptypb.Empty](),
			},
			rejects: []string{"hi", URL[*buildbucketpb.Build]()},
		},
		{
			name: "wildcard_all",
			urls: []string{
				TypePrefix + "*",
			},
			wantN: 1,
			matches: []string{
				URL[*structpb.Value](),
				URL[*structpb.ListValue](),
				URL[*structpb.Struct](),
				URL[*emptypb.Empty](),
				URL[*buildbucketpb.Build](),
			},
			rejects: []string{"hi"},
		},
		{
			name: "bad_prefix",
			urls: []string{
				"*",
			},
			wantErr: "expected prefix",
		},
		{
			name: "bad_star",
			urls: []string{
				TypePrefix + "hello*",
			},
			wantErr: "may only be used in a suffix",
		},
		{
			name: "double_star",
			urls: []string{
				TypePrefix + "*hello.*",
			},
			wantErr: "multiple *",
		},
		{
			name: "normalized",
			urls: []string{
				TypePrefix + "very.long.package.*",
				TypePrefix + "very.Cool",
				TypePrefix + "very.long.*",
				TypePrefix + "very.long.package.Spam",
				TypePrefix + "very.Cool",
				TypePrefix + "very.Cool",
			},
			wantN: 2,
			matches: []string{
				TypePrefix + "very.long.Cooltype",
				TypePrefix + "very.Cool",
				TypePrefix + "very.long.package.OtherType",
			},
			rejects: []string{
				TypePrefix + "very.longpkg.Cooltype",
				TypePrefix + "very.Notcool",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			matcher, err := MakeTypeMatcher(orchestratorpb.TypeSet_builder{
				TypeUrls: tc.urls,
			}.Build())
			if tc.wantErr != "" {
				assert.ErrIsLike(t, err, tc.wantErr)
				return
			}

			assert.Loosely(t, matcher.patterns, should.HaveLength(tc.wantN),
				truth.Explain("patterns: %v", matcher.patterns))

			assert.NoErr(t, err)

			for _, matchCandidate := range tc.matches {
				assert.That(t, matcher.Match(matchCandidate), should.BeTrue, truth.Explain(
					"%q does not match %q", matchCandidate, matcher.patterns))
			}
			for _, rejectCandidate := range tc.rejects {
				assert.That(t, matcher.Match(rejectCandidate), should.BeFalse, truth.Explain(
					"%q matches %q", rejectCandidate, matcher.patterns))
			}
		})
	}
}

func TestTypeSetBuilder(t *testing.T) {
	t.Parallel()

	t.Run(`empty`, func(t *testing.T) {
		t.Parallel()

		tb, err := TypeSetBuilder{}.Build()
		assert.NoErr(t, err)
		assert.Loosely(t, tb, should.BeNil)
	})

	t.Run(`fixed`, func(t *testing.T) {
		t.Parallel()

		tb, err := TypeSetBuilder{}.WithMessages(&emptypb.Empty{}, &structpb.Struct{}).Build()
		assert.NoErr(t, err)
		assert.Loosely(t, tb.GetTypeUrls(), should.HaveLength(2))
	})

	t.Run(`normalized`, func(t *testing.T) {
		t.Parallel()

		tb, err := (TypeSetBuilder{}.
			WithMessages(&emptypb.Empty{}, &structpb.Struct{}).
			WithPackagesOf(&structpb.ListValue{}).
			Build())
		assert.NoErr(t, err)
		assert.Loosely(t, tb.GetTypeUrls(), should.HaveLength(1))
		assert.That(t, tb.GetTypeUrls()[0], should.Equal(
			TypePrefix+"google.protobuf.*"))
	})
}
