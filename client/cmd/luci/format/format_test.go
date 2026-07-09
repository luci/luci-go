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

package format

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestParseInvocationContext(t *testing.T) {
	t.Parallel()

	ftt.Run(`ParseInvocationContext`, t, func(t *ftt.Test) {
		t.Run(`build invocation`, func(t *ftt.Test) {
			res := ParseInvocationContext("invocations/build-8676971682343117873/tests/test1/results/res1")
			assert.Loosely(t, res, should.Equal("build 8676971682343117873"))
		})

		t.Run(`swarming task invocation`, func(t *ftt.Test) {
			res := ParseInvocationContext("invocations/task-chromium-swarm.appspot.com-7953535662988d11/tests/test1/results/res1")
			assert.Loosely(t, res, should.Equal("task 7953535662988d11"))
		})

		t.Run(`generic invocation`, func(t *ftt.Test) {
			res := ParseInvocationContext("invocations/u-mwarton-2026-07-07/tests/test1/results/res1")
			assert.Loosely(t, res, should.Equal("invocation u-mwarton-2026-07-07"))
		})

		t.Run(`invalid name`, func(t *ftt.Test) {
			res := ParseInvocationContext("invalid_name")
			assert.Loosely(t, res, should.BeEmpty)
		})
	})
}

func TestFormatVariant(t *testing.T) {
	t.Parallel()

	ftt.Run(`FormatVariant`, t, func(t *ftt.Test) {
		t.Run(`nil variant`, func(t *ftt.Test) {
			assert.Loosely(t, FormatVariant(nil), should.BeEmpty)
		})

		t.Run(`empty variant`, func(t *ftt.Test) {
			assert.Loosely(t, FormatVariant(&pb.Variant{}), should.BeEmpty)
		})

		t.Run(`populated variant`, func(t *ftt.Test) {
			v := &pb.Variant{
				Def: map[string]string{
					"builder":    "win-rel",
					"os":         "Windows-10",
					"test_suite": "browser_tests",
				},
			}
			assert.Loosely(t, FormatVariant(v), should.Equal("builder=win-rel os=Windows-10 test_suite=browser_tests"))
		})
	})
}

func TestStripHTML(t *testing.T) {
	t.Parallel()

	ftt.Run(`StripHTML`, t, func(t *ftt.Test) {
		t.Run(`plain text`, func(t *ftt.Test) {
			assert.Loosely(t, StripHTML("hello world"), should.Equal("hello world"))
		})

		t.Run(`simple tags`, func(t *ftt.Test) {
			assert.Loosely(t, StripHTML("<p>hello <b>world</b></p>"), should.Equal("hello world"))
		})

		t.Run(`links`, func(t *ftt.Test) {
			assert.Loosely(t, StripHTML("click <a href=\"http://example.com\">here</a>"), should.Equal("click here"))
		})

		t.Run(`complex tags and entities`, func(t *ftt.Test) {
			assert.Loosely(t, StripHTML(`<div title="foo > bar">text &lt; with &amp; entities</div>`), should.Equal("text < with & entities"))
		})

		t.Run(`empty`, func(t *ftt.Test) {
			assert.Loosely(t, StripHTML(""), should.BeEmpty)
		})
	})
}

func TestFormatSummaryHTML(t *testing.T) {
	t.Parallel()

	ftt.Run(`FormatSummaryHTML without showArtifacts`, t, func(t *ftt.Test) {
		html := `<p>Test failed</p><br><text-artifact artifact-id="log.txt">`
		res := FormatSummaryHTML(context.Background(), nil, nil, "invocations/inv1/tests/t1/results/r1", html, false)
		assert.Loosely(t, res, should.Equal("Test failed\n\n[Embedded Artifact: log.txt (pass --show-artifacts to view)]"))
	})
}
