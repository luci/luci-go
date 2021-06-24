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

// SFormatCLErrors is the same as FormatCLErrors but returns a string.
func SFormatCLErrors(ctx context.Context, reasons []*changelist.CLError, cl *changelist.CL, mode run.Mode) (string, error) {
	var sb strings.Builder
	if err := FormatCLErrors(ctx, reasons, cl, mode, &sb); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// FormatCLError formats 1 CL error by writing it into strings.Builder.
//
// mode should be specified if known.
func FormatCLErrors(ctx context.Context, reasons []*changelist.CLError, cl *changelist.CL, mode run.Mode, sb *strings.Builder) error {
	for i, r := range reasons {
		if err := FormatCLError(ctx, r, cl, mode, sb); err != nil {
			return err
		}
		if i > 0 {
			sb.WriteRune('\n')
		}
	}
	return nil
}

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
		return tmplWatchedByManyConfigGroups.Execute(sb, map[string]interface{}{
			"ConfigGroups": cgs,
			"TargetRef":    cl.Snapshot.GetGerrit().GetInfo().GetRef(),
		})

	case *changelist.CLError_InvalidDeps_:
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
			return errors.Reason("unsupported InvalidDeps reason %s", d).Err()
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

	default:
		return errors.Reason("unsupported purge reason %t: %s", v, reason).Err()
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
Please submit directly or via CQ the depenndencies first.
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

This may happens in rare circumstances such as moving a Gerrit Change to a new branch, abandoning & restoring the CL during an ongoing CQ Run, or when different users vote differently on a CQ label.

Please re-trigger the CQ if neccessary.
`)
