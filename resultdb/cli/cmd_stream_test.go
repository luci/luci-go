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

package cli

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/resultdb/internal/schemes"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/sink"
)

func parseStreamRun(t testing.TB, args ...string) *streamRun {
	t.Helper()
	r := cmdStream(Params{}).CommandRun().(*streamRun)
	assert.Loosely(t, r.Flags.Parse(args), should.BeNil)
	return r
}

func exportOuterSink(t testing.TB, ctx context.Context, variant *pb.Variant) context.Context {
	t.Helper()
	srv, err := sink.NewServer(ctx, sink.ServerConfig{
		Recorder:             pb.NewRecorderPRPCClient(nil),
		ArtifactStreamClient: &http.Client{},
		Invocation:           "invocations/u-outer-1",
		UpdateToken:          "token",
		ModuleScheme:         schemes.LegacyScheme,
		Variant:              variant,
	})
	assert.Loosely(t, err, should.BeNil)
	return srv.Export(ctx)
}

func TestInheritBaseVariant(t *testing.T) {
	t.Parallel()

	ftt.Run("inheritBaseVariant", t, func(t *ftt.Test) {
		ctx := context.Background()
		outerCtx := exportOuterSink(t, ctx, pbutil.Variant(
			"board", "x64",
			"builder", "fuchsia-x64",
			"os", "Fuchsia",
		))

		t.Run("inherits parent base variant and lets child -var override", func(t *ftt.Test) {
			r := parseStreamRun(t,
				"-new", "-include",
				"-module-name", "//pkg:foo_test",
				"-module-scheme", "gtest",
				"-var", "cpu:x64",
				"-var", "os:Linux",
				"--", "echo",
			)
			r.inheritBaseVariant(outerCtx)
			assert.Loosely(t, r.validate(outerCtx, r.Flags.Args()), should.BeNil)
			assert.Loosely(t, r.vars, should.Match(map[string]string{
				"board":   "x64",
				"builder": "fuchsia-x64",
				"cpu":     "x64",
				"os":      "Linux",
			}))
			assert.Loosely(t, r.moduleIDFromArgs(), should.Match(&pb.ModuleIdentifier{
				ModuleName:   "//pkg:foo_test",
				ModuleScheme: "gtest",
				ModuleVariant: pbutil.Variant(
					"board", "x64",
					"builder", "fuchsia-x64",
					"cpu", "x64",
					"os", "Linux",
				),
			}))
		})

		t.Run("child empty -var value overrides parent key", func(t *ftt.Test) {
			r := parseStreamRun(t, "-new", "-include", "-var", "os:", "--", "echo")
			r.inheritBaseVariant(outerCtx)
			assert.Loosely(t, r.validate(outerCtx, r.Flags.Args()), should.BeNil)
			assert.Loosely(t, r.vars, should.Match(map[string]string{
				"board":   "x64",
				"builder": "fuchsia-x64",
				"os":      "",
			}))
		})

		t.Run("does not inherit without -include", func(t *ftt.Test) {
			r := parseStreamRun(t, "-new", "-var", "cpu:x64", "--", "echo")
			r.inheritBaseVariant(outerCtx)
			assert.Loosely(t, r.validate(outerCtx, r.Flags.Args()), should.BeNil)
			assert.Loosely(t, r.vars, should.Match(map[string]string{
				"cpu": "x64",
			}))
		})

		t.Run("does not inherit without -new", func(t *ftt.Test) {
			r := parseStreamRun(t, "-var", "cpu:x64", "--", "echo")
			r.inheritBaseVariant(outerCtx)
			assert.Loosely(t, r.validate(outerCtx, r.Flags.Args()), should.BeNil)
			assert.Loosely(t, r.vars, should.Match(map[string]string{
				"cpu": "x64",
			}))
		})

		t.Run("no-op when ResultSink is absent from LUCI_CONTEXT", func(t *ftt.Test) {
			r := parseStreamRun(t, "-new", "-include", "-var", "cpu:x64", "--", "echo")
			r.inheritBaseVariant(ctx)
			assert.Loosely(t, r.validate(ctx, r.Flags.Args()), should.BeNil)
			assert.Loosely(t, r.vars, should.Match(map[string]string{
				"cpu": "x64",
			}))
		})

		t.Run("validates combined variant size after inheritance", func(t *ftt.Test) {
			largeOuterCtx := exportOuterSink(t, ctx, pbutil.Variant(
				"k1", strings.Repeat("a", 200),
				"k2", strings.Repeat("a", 200),
				"k3", strings.Repeat("a", 200),
				"k4", strings.Repeat("a", 200),
			))
			r := parseStreamRun(t, "-new", "-include", "-var", "k5:"+strings.Repeat("b", 250), "--", "echo")
			r.inheritBaseVariant(largeOuterCtx)
			assert.Loosely(t, r.validate(largeOuterCtx, r.Flags.Args()), should.ErrLike("exceeds the maximum size"))
		})
	})
}
