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

package git

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/milo/api/config"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConfigValidation(t *testing.T) {
	t.Parallel()
	c := context.Background()

	Convey("Validate & load source_ACLs works", t, func() {
		validate := func(cfg ...*config.Settings_SourceAcls) error {
			ctx := validation.Context{Context: c}
			ctx.SetFile("settings.cfg")
			ValidateACLsConfig(&ctx, cfg)
			return ctx.Finalize()
		}
		mustError := func(cfg ...*config.Settings_SourceAcls) multiError {
			err := validate(cfg...)
			So(err, ShouldNotBeNil)
			return multiError(err.(*validation.Error).Errors)
		}

		valid := config.Settings_SourceAcls{
			Hosts:    []string{"a.googlesource.com"},
			Projects: []string{"https://b.googlesource.com/c"},
			Readers:  []string{"group:g", "user@example.com"},
		}
		So(validate(&valid), ShouldBeNil)

		mustError(&config.Settings_SourceAcls{}).with(
			"at least 1 reader required",
			"at least 1 host or project required",
		)

		Convey("readers", func() {
			mod := valid
			mod.Readers = []string{"bad:kind", "group:", "user", "group:a", "group:a"}
			mustError(&mod).with(
				`invalid readers "bad:kind"`,
				`invalid readers "group:": needs a group name`,
				`invalid readers "user"`,
				`duplicate`,
			)
		})

		Convey("hosts", func() {
			second := config.Settings_SourceAcls{
				Hosts: []string{
					valid.Hosts[0],
					"example.com",
					"repo.googlesource.com/repo",
					"b.googlesource.com", // valid.Project was from here, and it's OK.
				},
				Readers: []string{"group:a"},
			}
			mustError(&valid, &second).with(
				`host "a.googlesource.com"): has already been defined in source_acl #0`,
				`isn't at *.googlesource.com`,
				"shouldn't have path or fragment components",
			)
		})

		Convey("projects", func() {
			second := config.Settings_SourceAcls{
				Hosts: []string{"r.googlesource.com"},
				Projects: []string{
					valid.Projects[0], // dups of prev blocks are OK.
					"r.googlesource.com/redundant",
					"not-repo.googlesource.com",
					"c.googlesource.com/a/repo.git#123",
					"c-review.googlesource.com/src",
					"https://\\meh",
					valid.Projects[0], // dups of projects in this block is not OK.
				},
				Readers: []string{"group:b"},
			}
			mustError(&valid, &second).with(
				`redundant because already covered by its host in the same source_ACLs block`,
				`project "not-repo.googlesource.com"): should not be just a host`,
				`should not contain '/a' path prefix`,
				`should not contain '.git'`,
				`shouldn't have fragment components`,
				`must not be a Gerrit host (try without '-review')`,
				`not a valid URL`,
				`duplicate, already defined in the same source_ACLs block`,
			)
		})
	})
}

type multiError []error

func (m multiError) with(substrings ...string) {
	for i, err := range m {
		if i >= len(substrings) {
			So(fmt.Errorf("extra errors produced: %q", m[i:]), ShouldBeNil)
		} else {
			So(err.Error(), ShouldContainSubstring, substrings[i])
		}
	}
	if len(substrings) > len(m) {
		So(fmt.Errorf("not produced errors: %q", substrings[len(m):]), ShouldBeNil)
	}
}
