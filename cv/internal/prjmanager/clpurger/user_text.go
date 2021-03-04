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

package clpurger

import (
	"context"
	"strings"
	"text/template"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

func formatMessage(ctx context.Context, task *prjpb.PurgeCLTask, cl *changelist.CL) (string, error) {
	r := task.GetReason()
	switch v := r.GetReason().(type) {
	case *prjpb.PurgeCLTask_Reason_OwnerLacksEmail:
		if !v.OwnerLacksEmail {
			return "", errors.New("owner_lacks_email must be set to true")
		}
		return tmplExec(tmplCLOwnerLacksEmails, map[string]string{
			"GerritHost": cl.Snapshot.GetGerrit().GetHost(),
		})

	case *prjpb.PurgeCLTask_Reason_WatchedByManyConfigGroups_:
		cgs := v.WatchedByManyConfigGroups.GetConfigGroups()
		if len(cgs) < 2 {
			return "", errors.New("at least 2 config_groups required")
		}
		return tmplExec(tmplWatchedByManyConfigGroups, map[string]interface{}{
			"ConfigGroups": cgs,
			"TargetRef":    cl.Snapshot.GetGerrit().GetInfo().GetRef(),
		})

	case *prjpb.PurgeCLTask_Reason_InvalidDeps_:
		return "TODO(tandrii): IMPLEMENT!", nil

	default:
		return "", errors.Reason("usupported purge reason %t: %s", v, r).Err()
	}
}

func tmplExec(t *template.Template, data interface{}) (string, error) {
	sb := strings.Builder{}
	if err := t.Execute(&sb, data); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func tmplMust(text string) *template.Template {
	text = strings.TrimSpace(text)
	return template.Must(template.New("").Funcs(tmplFuncs).Parse(text))
}

var tmplFuncs = template.FuncMap{
	"CQ_OR_CV": func() string { return "CQ" },
	"CONTACT_YOUR_INFRA": func() string {
		// TODO(tandrii): ideally, CV or even LUCI would provide project-specific
		// URL from a config.
		return "Please, contact your EngProd or infrastructure team"
	},
}

var tmplCLOwnerLacksEmails = tmplMust(`
{{CQ_OR_CV}} can't process the CL because its owner doesn't have a preferred email set in Gerrit settings.

You can set preferred email at https://{{.GerritHost}}/settings/#EmailAddresses
`)

var tmplWatchedByManyConfigGroups = tmplMust(`
{{CQ_OR_CV}} can't process the CL because it is watched by more than 1 config group:
{{range $cg := .ConfigGroups}}  * {{$cg}}
{{end}}
{{CONTACT_YOUR_INFRA}}. For their info:
  * current CL target ref is {{.TargetRef | printf "%q"}},
	* relevant doc https://chromium.googlesource.com/infra/luci/luci-go/+/HEAD/lucicfg/doc/#luci.cq_group
`)
