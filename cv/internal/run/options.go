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

package run

import (
	"fmt"
	"strings"

	"go.chromium.org/luci/common/git/footer"
	"go.chromium.org/luci/cv/internal/changelist"
)

// ExtractOptions computes the Run Options from 1 CL.
func ExtractOptions(snapshot *changelist.Snapshot) *Options {
	ci := snapshot.GetGerrit().GetInfo()
	clDescription := ci.GetRevisions()[ci.GetCurrentRevision()].GetCommit().GetMessage()
	modern := footer.ParseMessage(clDescription)
	legacy := footer.ParseLegacyMetadata(clDescription)

	valuesOf := func(mkey, lkey string) []string {
		var out []string

		switch k := footer.NormalizeKey(mkey); {
		case mkey == "":
		case k != mkey:
			panic(fmt.Errorf("Use normalized key %q not %q in CV code", k, mkey))
		default:
			out = append(out, modern[mkey]...)
		}

		if lkey != "" {
			out = append(out, legacy[lkey]...)
		}
		return out
	}

	has := func(mkey, lkey, value string) bool {
		if l := strings.ToLower(value); l != value {
			panic(fmt.Errorf("Use lowercase value %q not %q in CV code", l, value))
		}
		for _, v := range valuesOf(mkey, lkey) {
			if strings.ToLower(v) == value {
				return true
			}
		}
		return false
	}

	isTrue := func(mkey, lkey string) bool { return has(mkey, lkey, "true") }

	o := &Options{}
	if isTrue("No-Equivalent-Builders", "NO_EQUIVALENT_BUILDERS") {
		o.SkipEquivalentBuilders = true
	}
	if isTrue("Cq-Do-Not-Cancel-Tryjobs", "") {
		o.AvoidCancellingTryjobs = true
	}
	if isTrue("No-Tree-Checks", "NOTREECHECKS") {
		o.SkipTreeChecks = true
	}
	if isTrue("No-Try", "NOTRY") {
		o.SkipTryjobs = true
	}
	if isTrue("No-Presubmit", "NOPRESUBMIT") {
		o.SkipPresubmit = true
	}
	// TODO(tandrii): parse CQ-IncludeTrybots.
	return o
}

// MergeOptions merges two Run Options.
//
// Does not modify the passed object, but may return either of them.
func MergeOptions(a, b *Options) *Options {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	}
	return &Options{
		AvoidCancellingTryjobs: a.AvoidCancellingTryjobs || b.AvoidCancellingTryjobs,
		SkipTryjobs:            a.SkipTryjobs || b.SkipTryjobs,
		SkipPresubmit:          a.SkipPresubmit || b.SkipPresubmit,
		SkipEquivalentBuilders: a.SkipEquivalentBuilders || b.SkipEquivalentBuilders,
		SkipTreeChecks:         a.SkipTreeChecks || b.SkipTreeChecks,
	}
}
