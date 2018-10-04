// Copyright 2017 The LUCI Authors.
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

package pprof

import (
	"context"
	"fmt"
	"html"
	"html/template"

	"go.chromium.org/luci/server/portal"
)

type portalPage struct {
	portal.BasePage
}

func (portalPage) Title(c context.Context) (string, error) {
	return "Pprof profiling", nil
}

func (portalPage) Overview(c context.Context) (template.HTML, error) {
	return `<p>This page allows to generate special tokens that authenticate
access to Go pprof endpoints, such as <code>/debug/pprof/heap</code>.</p>

<p>Normally (when using <a href="https://golang.org/pkg/net/http/pprof/">net/http/pprof</a>
package) they are wide open to anyone who has access to the serving port. This
is inconvenient in environments that don't easily allow filtering requests
before they hit the server (GAE Flex is an example). Using tokens for
authentication allows exposing pprof endpoints directly.</p>

<p>Usage:</p>
<ul>
<li>Hit "Generate pprof token" button below.</li>
<li>Copy the token it produces.</li>
<li>Pass it as <code>?tok=...</code> URL parameter to pprof endpoints.</li>
</ul>

<p>Remember that in most cases (e.g. on GAE Flex, and other load-balanced
environments) requests are routed to some random backend process, so hitting
same profile page multiple times may result in completely different profiles.
</p>
`, nil
}

func (portalPage) Actions(c context.Context) ([]portal.Action, error) {
	return []portal.Action{
		{
			ID:    "GeneratePprofToken",
			Title: "Generate pprof token",
			Callback: func(c context.Context) (template.HTML, error) {
				tok, err := generateToken(c)
				if err != nil {
					return "", err
				}
				return template.HTML(fmt.Sprintf(
					`<p>The generated pprof token is:</p><pre>%s</pre>`+
						`<p>It is valid for 12 hours. Pass it a pprof endpoint via `+
						`<code>?tok=...</code> URL parameter, e.g</p>`+
						`<pre>$ go tool pprof https://host/debug/pprof/heap?tok=%s...</pre>`,
					html.EscapeString(tok), html.EscapeString(tok[:5]))), nil
			},
		},
	}, nil
}

func init() {
	portal.RegisterPage("pprof", portalPage{})
}
