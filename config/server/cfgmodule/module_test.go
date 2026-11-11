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

package cfgmodule

import (
	"flag"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/vars"
	"go.chromium.org/luci/server/module"
)

func TestRegisterVars(t *testing.T) {
	t.Parallel()

	t.Run("registers custom -config-var flags alongside built-in vars", func(t *testing.T) {
		t.Parallel()
		opts := &ModuleOptions{Vars: &vars.VarSet{}}
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		opts.Register(fs)
		assert.NoErr(t, fs.Parse([]string{"-config-var", "foo=bar", "-config-var", "baz=qux"}))

		mod := &serverModule{opts: opts}
		assert.NoErr(t, mod.registerVars(module.HostOptions{CloudProject: "my-proj"}))

		rendered, err := opts.Vars.RenderTemplate(t.Context(), "${appid}:${foo}:${baz}")
		assert.NoErr(t, err)
		assert.That(t, rendered, should.Equal("my-proj:bar:qux"))
	})

	t.Run("rejects overriding built-in vars", func(t *testing.T) {
		t.Parallel()
		for _, reserved := range []string{"appid", "config_service_appid"} {
			opts := &ModuleOptions{Vars: &vars.VarSet{}}
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			opts.Register(fs)
			assert.NoErr(t, fs.Parse([]string{"-config-var", reserved + "=override"}))

			mod := &serverModule{opts: opts}
			assert.ErrIsLike(t, mod.registerVars(module.HostOptions{}), "-config-var cannot override built-in variable")
		}
	})
}
