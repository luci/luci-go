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
	"sort"
	"strings"
	"text/template"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

func formatMessage(ctx context.Context, task *prjpb.PurgeCLTask, cl *changelist.CL) (string, error) {
	// TODO(tandrii): support >1 reason.
	r := task.GetReasons()[0]
	switch v := r.GetKind().(type) {
	case *prjpb.CLError_OwnerLacksEmail:
		if !v.OwnerLacksEmail {
			return "", errors.New("owner_lacks_email must be set to true")
		}
		return tmplExec(tmplCLOwnerLacksEmails, map[string]string{
			"GerritHost": cl.Snapshot.GetGerrit().GetHost(),
		})

	case *prjpb.CLError_WatchedByManyConfigGroups_:
		cgs := v.WatchedByManyConfigGroups.GetConfigGroups()
		if len(cgs) < 2 {
			return "", errors.New("at least 2 config_groups required")
		}
		return tmplExec(tmplWatchedByManyConfigGroups, map[string]interface{}{
			"ConfigGroups": cgs,
			"TargetRef":    cl.Snapshot.GetGerrit().GetInfo().GetRef(),
		})

	case *prjpb.CLError_InvalidDeps_:
		// Although it's possible for a CL to have several kinds of wrong deps,
		// it's rare in practice, so simply error out on the most important kind.
		var bad []*changelist.Dep
		args := make(map[string]interface{}, 2)
		var t *template.Template
		switch d := v.InvalidDeps; {
		case len(d.GetUnwatched()) > 0:
			bad, t = d.GetUnwatched(), tmplUnwatchedDeps
		case len(d.GetWrongConfigGroup()) > 0:
			bad, t = d.GetWrongConfigGroup(), tmplWrongDepsConfigGroup
		case len(d.GetIncompatMode()) > 0:
			bad, t = d.GetIncompatMode(), tmplIncompatDepsMode
			args["mode"] = task.GetTrigger().GetMode()
		default:
			return "", errors.Reason("usupported InvalidDeps reason %s", d).Err()
		}
		urls, err := depsURLs(ctx, bad)
		if err != nil {
			return "", err
		}
		sort.Strings(urls)
		args["deps"] = urls
		return tmplExec(t, args)

	default:
		return "", errors.Reason("usupported purge reason %t: %s", v, r).Err()
	}
}

func depsURLs(ctx context.Context, deps []*changelist.Dep) ([]string, error) {
	cls := make([]*changelist.CL, len(deps))
	for i, d := range deps {
		cls[i] = &changelist.CL{ID: common.CLID(d.GetClid())}
	}
	if err := datastore.Get(ctx, cls); err != nil {
		return nil, errors.Annotate(err, "failed to load deps as CLs").Tag(transient.Tag).Err()
	}
	urls := make([]string, len(deps))
	for i, cl := range cls {
		var err error
		if urls[i], err = cl.URL(); err != nil {
			return nil, err
		}
	}
	return urls, nil
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

var tmplUnwatchedDeps = tmplMust(`
{{CQ_OR_CV}} can't process the CL because its deps are not watched by the same LUCI project:
{{range $url := .deps}}  * {{$url}}
{{end}}
Please check Cq-Depend: in CL description (commit message). If you think this is a mistake, {{CONTACT_YOUR_INFRA}}.
`)

var tmplWrongDepsConfigGroup = tmplMust(`
{{CQ_OR_CV}} can't process the CL because its deps do not belong to the same config group:
{{range $url := .deps}}  * {{$url}}
{{end}}
`)

var tmplIncompatDepsMode = tmplMust(`
{{CQ_OR_CV}} can't process the CL because its mode {{.mode | printf "%q"}} does not match mode on its dependencies:
{{range $url := .deps}}  * {{$url}}
{{end}}
`)
