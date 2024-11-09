// Copyright 2019 The LUCI Authors.
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

package luciexe

import (
	"flag"
	"io"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestOutputFlag(t *testing.T) {
	t.Parallel()

	ftt.Run(`OutputFlag`, t, func(t *ftt.Test) {
		fs := &flag.FlagSet{}
		fs.SetOutput(io.Discard)
		of := AddOutputFlagToSet(fs)

		t.Run(`good`, func(t *ftt.Test) {
			t.Run(`empty`, func(t *ftt.Test) {
				assert.Loosely(t, fs.Parse(nil), should.BeNil)
				assert.Loosely(t, of.Path, should.BeEmpty)
				assert.Loosely(t, of.Codec, should.Resemble(buildFileCodecNoop{}))
			})

			t.Run(`missing`, func(t *ftt.Test) {
				assert.Loosely(t, fs.Parse([]string{"other", "stuff"}), should.BeNil)
				assert.Loosely(t, of.Path, should.BeEmpty)
				assert.Loosely(t, of.Codec, should.Resemble(buildFileCodecNoop{}))
			})

			t.Run(`equal`, func(t *ftt.Test) {
				assert.Loosely(t, fs.Parse([]string{OutputCLIArg + "=out.textpb", "a", "b"}), should.BeNil)
				assert.Loosely(t, of.Path, should.Match("out.textpb"))
				assert.Loosely(t, of.Codec, should.Resemble(buildFileCodecText{}))
				assert.Loosely(t, fs.Args(), should.Resemble([]string{"a", "b"}))
			})

			t.Run(`two val`, func(t *ftt.Test) {
				assert.Loosely(t, fs.Parse([]string{OutputCLIArg, "out.json", "a", "b"}), should.BeNil)
				assert.Loosely(t, of.Path, should.Match("out.json"))
				assert.Loosely(t, of.Codec, should.Resemble(buildFileCodecJSON{}))
				assert.Loosely(t, fs.Args(), should.Resemble([]string{"a", "b"}))
			})

			t.Run(`explicit noop`, func(t *ftt.Test) {
				assert.Loosely(t, fs.Parse([]string{OutputCLIArg + "="}), should.BeNil)
				assert.Loosely(t, of.Path, should.BeEmpty)
				assert.Loosely(t, of.Codec, should.Resemble(buildFileCodecNoop{}))
			})

		})

		t.Run(`bad`, func(t *ftt.Test) {
			t.Run(`incomplete`, func(t *ftt.Test) {
				assert.Loosely(t, fs.Parse([]string{OutputCLIArg}), should.ErrLike("flag needs an argument"))
				assert.Loosely(t, of.Path, should.BeEmpty)
				assert.Loosely(t, of.Codec, should.Resemble(buildFileCodecNoop{}))
			})

			t.Run(`bad extension`, func(t *ftt.Test) {
				assert.Loosely(t, fs.Parse([]string{OutputCLIArg, "nope", "arg"}), should.ErrLike("bad extension"))
				assert.Loosely(t, of.Path, should.BeEmpty)
				assert.Loosely(t, of.Codec, should.Resemble(buildFileCodecNoop{}))
			})
		})
	})
}
