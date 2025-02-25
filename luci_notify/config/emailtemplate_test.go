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

package config

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/memory"

	"go.chromium.org/luci/luci_notify/common"
)

func TestEmailTemplate(t *testing.T) {
	t.Parallel()

	ftt.Run("fetchAllEmailTemplates", t, func(t *ftt.Test) {
		c := gologger.StdConfig.Use(context.Background())
		c = logging.SetLevel(c, logging.Debug)
		c = common.SetAppIDForTest(c, "luci-notify")

		cfgService := memory.New(map[config.Set]memory.Files{
			"projects/x": {
				"luci-notify/email-templates/a.template":            "aSubject\n\naBody",
				"luci-notify/email-templates/b.template":            "bSubject\n\nbBody",
				"luci-notify/email-templates/invalid name.template": "subject\n\nbody",
			},
			"projects/y": {
				"luci-notify/email-templates/c.template": "cSubject\n\ncBody",
			},
		})
		templates, err := fetchAllEmailTemplates(c, cfgService, "x")
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, templates, should.Match(map[string]*EmailTemplate{
			"a": {
				Name:                "a",
				SubjectTextTemplate: "aSubject",
				BodyHTMLTemplate:    "aBody",
				DefinitionURL:       "https://example.com/view/here/luci-notify/email-templates/a.template",
			},
			"b": {
				Name:                "b",
				SubjectTextTemplate: "bSubject",
				BodyHTMLTemplate:    "bBody",
				DefinitionURL:       "https://example.com/view/here/luci-notify/email-templates/b.template",
			},
		}))
	})
}
