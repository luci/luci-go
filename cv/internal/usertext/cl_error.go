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
	"context"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

// SFormatCLError is the same as FormatCLError but returns a string.
func SFormatCLError(ctx context.Context, reason *changelist.CLError, cl *changelist.CL, mode run.Mode) (string, error) {
	var sb strings.Builder
	if err := FormatCLError(ctx, reason, cl, mode, &sb); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// FormatCLError formats 1 CL error by writing it into strings.Builder.
//
// mode should be specified if known.
func FormatCLError(ctx context.Context, reason *changelist.CLError, cl *changelist.CL, mode run.Mode, sb *strings.Builder) error {
	switch v := reason.GetKind().(type) {
	case *changelist.CLError_OwnerLacksEmail:
		if !v.OwnerLacksEmail {
			return errors.New("owner_lacks_email must be set to true")
		}
		return tmplCLOwnerLacksEmails.Execute(sb, map[string]string{
			"GerritHost": cl.Snapshot.GetGerrit().GetHost(),
		})

	case *changelist.CLError_UnsupportedMode:
		if v.UnsupportedMode == "" {
			return errors.New("unsupported_mode must be set")
		}
		return tmplUnsupportedMode.Execute(sb, v)

	case *changelist.CLError_SelfCqDepend:
		if !v.SelfCqDepend {
			return errors.New("self_cq_depend must be set")
		}
		return tmplSelfCQDepend.Execute(sb, nil)

	case *changelist.CLError_CorruptGerritMetadata:
		if v.CorruptGerritMetadata == "" {
			return errors.New("corrupt_gerrit_metadata must be set")
		}
		return tmplCorruptGerritCLMetadata.Execute(sb, v)

	case *changelist.CLError_WatchedByManyConfigGroups_:
		cgs := v.WatchedByManyConfigGroups.GetConfigGroups()
		if len(cgs) < 2 {
			return errors.New("at least 2 config_groups required")
		}
		return tmplWatchedByManyConfigGroups.Execute(sb, map[string]any{
			"ConfigGroups": cgs,
			"TargetRef":    cl.Snapshot.GetGerrit().GetInfo().GetRef(),
		})

	case *changelist.CLError_WatchedByManyProjects_:
		projects := v.WatchedByManyProjects.GetProjects()
		if len(projects) < 2 {
			return errors.New("at least 2 projects required")
		}
		return tmplWatchedByManyProjects.Execute(sb, map[string]any{
			"Projects": projects,
		})

	case *changelist.CLError_InvalidDeps_:
		// Although it's possible for a CL to have several kinds of wrong deps,
		// it's rare in practice, so simply error out on the most important kind.
		var bad []*changelist.Dep
		args := make(map[string]any, 2)
		var t *template.Template
		switch d := v.InvalidDeps; {
		case len(d.GetUnwatched()) > 0:
			bad, t = d.GetUnwatched(), tmplUnwatchedDeps
		case len(d.GetWrongConfigGroup()) > 0:
			bad, t = d.GetWrongConfigGroup(), tmplWrongDepsConfigGroup
		case len(d.GetSingleFullDeps()) > 0:
			bad, t = d.GetSingleFullDeps(), tmplSingleFullOpenDeps
			args["mode"] = string(mode)
		case len(d.GetCombinableUntriggered()) > 0:
			bad, t = d.GetCombinableUntriggered(), tmplCombinableUntriggered
		case len(d.GetCombinableMismatchedMode()) > 0:
			bad, t = d.GetCombinableMismatchedMode(), tmplCombinableMismatchedMode
			args["mode"] = string(mode)
		case d.GetTooMany() != nil:
			args["max"] = d.GetTooMany().GetMaxAllowed()
			args["actual"] = d.GetTooMany().GetActual()
			t = tmplTooManyDeps
			bad = nil // there is no point listing all of them.
		default:
			return errors.Fmt("unsupported InvalidDeps reason %s", d)
		}
		urls, err := depsURLs(ctx, bad)
		if err != nil {
			return err
		}
		sort.Strings(urls)
		args["deps"] = urls
		return t.Execute(sb, args)

	case *changelist.CLError_ReusedTrigger_:
		if v.ReusedTrigger == nil {
			return errors.New("reused_trigger must be set")
		}
		return tmplReusedTrigger.Execute(sb, v.ReusedTrigger)

	case *changelist.CLError_CommitBlocked:
		return tmplCommitBlocked.Execute(sb, nil)

	case *changelist.CLError_TriggerDeps_:
		if v.TriggerDeps == nil {
			return errors.New("trigger_deps must be set")
		}
		var msgs []string
		var deps []*changelist.Dep
		for _, denied := range v.TriggerDeps.GetPermissionDenied() {
			var mb strings.Builder
			fmt.Fprintf(&mb, "no permission to vote")
			if denied.GetEmail() != "" {
				fmt.Fprintf(&mb, " on behalf of %s", denied.GetEmail())
			}
			msgs = append(msgs, mb.String())
			deps = append(deps, &changelist.Dep{Clid: denied.GetClid()})
		}
		for _, clid := range v.TriggerDeps.GetNotFound() {
			msgs = append(msgs, "the CL no longer exists in Gerrit")
			deps = append(deps, &changelist.Dep{Clid: clid})
		}
		for _, clid := range v.TriggerDeps.GetInternalGerritError() {
			msgs = append(msgs, "internal Gerrit error")
			deps = append(deps, &changelist.Dep{Clid: clid})
		}
		urls, err := depsURLs(ctx, deps)
		if err != nil {
			return err
		}
		return tmplTriggerDeps.Execute(sb, map[string]any{"urls": urls, "messages": msgs})

	case *changelist.CLError_DepRunFailed:
		if v.DepRunFailed == 0 {
			return errors.New("dep_run_failed must be > 0")
		}
		urls, err := depsURLs(ctx, []*changelist.Dep{{Clid: v.DepRunFailed}})
		if err != nil {
			return err
		}
		return tmplDepRunFailed.Execute(sb, map[string]any{"url": urls[0]})

	default:
		return errors.Fmt("unsupported purge reason %t: %s", v, reason)
	}
}

func depsURLs(ctx context.Context, deps []*changelist.Dep) ([]string, error) {
	cls := make([]*changelist.CL, len(deps))
	for i, d := range deps {
		cls[i] = &changelist.CL{ID: common.CLID(d.GetClid())}
	}
	if err := datastore.Get(ctx, cls); err != nil {
		return nil, transient.Tag.Apply(errors.Fmt("failed to load deps as CLs: %w", err))
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

var tmplCLOwnerLacksEmails = tmplMust(`
{{CQ_OR_CV}} can't process the CL because its owner doesn't have a preferred email set in Gerrit settings.

You can set preferred email at https://{{.GerritHost}}/settings/#EmailAddresses
`)

var tmplUnsupportedMode = tmplMust(`
{{CQ_OR_CV}} can't process the CL because its mode {{.UnsupportedMode | printf "%q"}} is not supported.
{{CONTACT_YOUR_INFRA}}
`)

var tmplSelfCQDepend = tmplMust(`
{{CQ_OR_CV}} can't process the CL because it depends on itself.

Please check Cq-Depend: in CL description (commit message). If you think this is a mistake, {{CONTACT_YOUR_INFRA}}.
`)

var tmplCorruptGerritCLMetadata = tmplMust(`
{{CQ_OR_CV}} can't process the CL because its Gerrit metadata looks corrupted.

{{.CorruptGerritMetadata}}

Consider filing a Gerrit bug or {{CONTACT_YOUR_INFRA}}.
In the meantime, consider re-uploading your CL(s).
`)

var tmplWatchedByManyConfigGroups = tmplMust(`
{{CQ_OR_CV}} can't process the CL because it is watched by more than 1 config group:
{{range $cg := .ConfigGroups}}  * {{$cg}}
{{end}}
{{CONTACT_YOUR_INFRA}}. For their info:
  * current CL target ref is {{.TargetRef | printf "%q"}},
	* relevant doc https://chromium.googlesource.com/infra/luci/luci-go/+/HEAD/lucicfg/doc/#luci.cq_group
`)

var tmplWatchedByManyProjects = tmplMust(`
{{CQ_OR_CV}} can't process the CL because it is watched by more than 1 LUCI project:
{{range $p := .Projects}}  * {{$p}}
{{end}}
{{CONTACT_YOUR_INFRA}}. Relevant doc for their info:
https://chromium.googlesource.com/infra/luci/luci-go/+/HEAD/lucicfg/doc/#luci.cq_group
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

var tmplSingleFullOpenDeps = tmplMust(`
{{CQ_OR_CV}} can't process the CL in {{.mode | printf "%q"}} mode because it has not yet submitted dependencies:
{{range $url := .deps}}  * {{$url}}
{{end}}
Please submit directly or via CQ the dependencies first.
`)

var tmplCombinableUntriggered = tmplMust(`
{{CQ_OR_CV}} can't process the CL because its dependencies weren't CQ-ed at all:
{{range $url := .deps}}  * {{$url}}
{{end}}
Please trigger this CL and its dependencies at the same time.
`)

var tmplCombinableMismatchedMode = tmplMust(`
{{CQ_OR_CV}} can't process the CL because its mode {{.mode | printf "%q"}} does not match mode on its dependencies:
{{range $url := .deps}}  * {{$url}}
{{end}}
`)

var tmplTooManyDeps = tmplMust(`
{{CQ_OR_CV}} can't process the CL because it has too many deps: {{.actual}} (max supported: {{.max}})
`)

// TODO(crbug/1223350): once we have a way to reference prior Runs, add a link
// to the Run.
var tmplReusedTrigger = tmplMust(`
{{CQ_OR_CV}} can't process the CL because it has previously completed a Run ({{.Run | printf "%q"}}) triggered by the same vote(s).

This may happen in rare circumstances such as moving a Gerrit Change to a new branch, abandoning & restoring the CL during an ongoing CQ Run, or when different users vote differently on a CQ label.

Please re-trigger the CQ if necessary.
`)

var tmplCommitBlocked = tmplMust(`
{{CQ_OR_CV}} won't start a full run for the CL because it has a "Commit: false" footer.

The "Commit: false" footer is used to prevent accidental submission of a CL. You may try a dry run; if you want to submit the CL, you must first remove the "Commit: false" footer from the CL description.
`)

var tmplTriggerDeps = tmplMust(`
{{CQ_OR_CV}} failed to vote the CQ label on the following dependencies.
{{range $i, $url := .urls}}  * {{$url}} - {{index $.messages $i}}
{{end}}
`)

var tmplDepRunFailed = tmplMust(`
Revoking the CQ vote, because a Run failed on [CL]({{.url}}) that this CL depends on.
`)
