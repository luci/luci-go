// Copyright 2021 The LUCI Authors.
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

package usertext

import (
	"strings"
	"text/template"
)

// tmplFuncs are commonly used constants, usable via Go templates.
var tmplFuncs = template.FuncMap{
	"CQ_OR_CV": func() string { return "CQ" },
	"CONTACT_YOUR_INFRA": func() string {
		// TODO(tandrii): ideally, CV or even LUCI would provide project-specific
		// URL from a config.
		return "Please contact your EngProd or infrastructure team"
	},
}

func tmplMust(text string) *template.Template {
	text = strings.TrimSpace(text)
	return template.Must(template.New("").Funcs(tmplFuncs).Parse(text))
}
