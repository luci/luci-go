// Copyright 2018 The LUCI Authors.
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

package notify

import (
	"context"
	"testing"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/common"
	"go.chromium.org/luci/luci_notify/config"
)

func TestEmailGen(t *testing.T) {
	t.Parallel()

	ftt.Run(`bundle`, t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		c = common.SetAppIDForTest(c, "luci-config")
		c = caching.WithEmptyProcessCache(c)

		chromium := &config.Project{Name: "chromium", Revision: "deadbeef"}
		chromiumKey := datastore.KeyForObj(c, chromium)
		assert.Loosely(t, datastore.Put(c, chromium), should.BeNil)

		templates := []*config.EmailTemplate{
			{
				ProjectKey:          chromiumKey,
				Name:                "default",
				SubjectTextTemplate: "Build {{.Build.Id}} completed",
				BodyHTMLTemplate:    `Build {{.Build.Id}} completed with status {{.Build.Status}}`,
			},
			{
				ProjectKey:          chromiumKey,
				Name:                "using_other_files",
				SubjectTextTemplate: "",
				BodyHTMLTemplate: `
Reusing templates from another files.
{{template "inlineEntireFile" .}}
{{template "steps" .}}`,
			},
			{
				ProjectKey:          chromiumKey,
				Name:                "inlineEntireFile",
				SubjectTextTemplate: "this file is shared",
				BodyHTMLTemplate:    `Build {{.Build.Id}}`,
			},
			{
				ProjectKey:          chromiumKey,
				Name:                "shared",
				SubjectTextTemplate: "this file is shared",
				BodyHTMLTemplate:    `{{define "steps"}}steps of build {{.Build.Id}} go here{{end}}`,
			},
			{
				ProjectKey:          chromiumKey,
				Name:                "bad",
				SubjectTextTemplate: "bad template",
				BodyHTMLTemplate:    `{{.FieldDoesNotExist}}`,
			},
		}
		assert.Loosely(t, datastore.Put(c, templates), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		bundle, err := getBundle(c, chromium.Name)
		assert.Loosely(t, err, should.BeNil)

		t.Run("bundles are cached", func(t *ftt.Test) {
			secondBundle, err := getBundle(c, chromium.Name)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, secondBundle, should.Equal(bundle)) // pointers match
		})

		t.Run("caching honors revision", func(t *ftt.Test) {
			chromium.Revision = "badcoffee"
			assert.Loosely(t, datastore.Put(c, chromium), should.BeNil)
			secondBundle, err := getBundle(c, chromium.Name)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, secondBundle, should.NotEqual(bundle)) // pointers mismatch, new bundle
		})

		t.Run(`GenerateEmail`, func(t *ftt.Test) {
			input := &notifypb.TemplateInput{
				BuildbucketHostname: "buildbucket.example.com",
				Build: &buildbucketpb.Build{
					Id: 54,
					Builder: &buildbucketpb.BuilderID{
						Project: "chromium",
						Bucket:  "ci",
						Builder: "linux-rel",
					},
					Status: buildbucketpb.Status_SUCCESS,
				},
			}
			t.Run("simple template", func(t *ftt.Test) {
				subject, body := bundle.GenerateEmail("default", input)
				assert.Loosely(t, subject, should.Equal("Build 54 completed"))
				assert.Loosely(t, body, should.Equal("Build 54 completed with status SUCCESS"))
			})

			t.Run("template using other files", func(t *ftt.Test) {
				_, body := bundle.GenerateEmail("using_other_files", input)
				assert.Loosely(t, body, should.Equal(`
Reusing templates from another files.
Build 54
steps of build 54 go here`))
			})

			t.Run("error", func(t *ftt.Test) {
				_, body := bundle.GenerateEmail("bad", input)
				assert.Loosely(t, body, should.ContainSubstring("spartan"))
			})
		})
	})
}
