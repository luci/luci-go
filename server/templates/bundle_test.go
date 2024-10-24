// Copyright 2015 The LUCI Authors.
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

package templates

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestRender(t *testing.T) {
	assets := map[string]string{
		"includes/base": `
{{define "base"}}
base
{{template "content" .}}
{{end}}
`,
		"pages/page": `
{{define "content"}}
content {{.arg1}} {{.arg2}}
{{end}}
`,
	}

	ftt.Run("AssetsLoader works", t, func(t *ftt.Test) {
		loaderTest(t, AssetsLoader(assets))
	})

	ftt.Run("FileSystemLoader works", t, func(t *ftt.Test) {
		dir, err := os.MkdirTemp("", "luci-go-templates")
		assert.Loosely(t, err, should.BeNil)
		defer os.RemoveAll(dir)

		for k, v := range assets {
			path := filepath.Join(dir, filepath.FromSlash(k))
			assert.Loosely(t, os.MkdirAll(filepath.Dir(path), 0777), should.BeNil)
			assert.Loosely(t, os.WriteFile(path, []byte(v), 0666), should.BeNil)
		}
		loaderTest(t, FileSystemLoader(os.DirFS(dir)))
	})
}

func loaderTest(t testing.TB, l Loader) {
	t.Helper()
	b := Bundle{
		Loader:          l,
		DefaultTemplate: "base",
		DefaultArgs: func(c context.Context, e *Extra) (Args, error) {
			assert.That(t, e.Request.Host, should.Equal("hi.example.com"), truth.LineContext())
			return Args{"arg1": "val1"}, nil
		},
	}
	ctx := Use(context.Background(), &b, &Extra{
		Request: &http.Request{Host: "hi.example.com"},
	})

	tmpl, err := Get(ctx, "pages/page")
	assert.Loosely(t, tmpl, should.NotBeNilInterface, truth.LineContext())
	assert.That(t, err, should.ErrLike(nil), truth.LineContext())

	blob, err := Render(ctx, "pages/page", Args{"arg2": "val2"})
	assert.That(t, err, should.ErrLike(nil), truth.LineContext())
	assert.That(t, string(blob), should.Equal("\nbase\n\ncontent val1 val2\n\n"), truth.LineContext())

	buf := bytes.Buffer{}
	MustRender(ctx, &buf, "pages/page", Args{"arg2": "val2"})
	assert.That(t, buf.String(), should.Equal("\nbase\n\ncontent val1 val2\n\n"), truth.LineContext())
}
