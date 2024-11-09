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
	"go.chromium.org/luci/cv/internal/common"
)

// ExtractOptions computes the Run Options from 1 CL.
func ExtractOptions(snapshot *changelist.Snapshot) *Options {
	byKey := make(map[string][]string, len(snapshot.GetMetadata()))
	for _, pair := range snapshot.GetMetadata() {
		k, v := pair.GetKey(), pair.GetValue()
		byKey[k] = append(byKey[k], v)
	}

	valuesOf := func(mkey, lkey string) []string {
		var out []string

		switch k := footer.NormalizeKey(mkey); {
		case mkey == "":
		case k != mkey:
			panic(fmt.Errorf("use normalized key %q not %q in CV code", k, mkey))
		default:
			out = append(out, byKey[mkey]...)
		}

		if lkey != "" {
			out = append(out, byKey[lkey]...)
		}
		return out
	}

	has := func(mkey, lkey, value string) bool {
		if l := strings.ToLower(value); l != value {
			panic(fmt.Errorf("use lowercase value %q not %q in CV code", l, value))
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
	if isTrue(common.FooterNoEquivalentBuilders, common.FooterLegacyNoEquivalentBuilders) {
		o.SkipEquivalentBuilders = true
	}
	if isTrue(common.FooterCQDoNotCancelTryjobs, "") {
		o.AvoidCancellingTryjobs = true
	}
	if isTrue(common.FooterNoTreeChecks, common.FooterLegacyNoTreeChecks) {
		o.SkipTreeChecks = true
	}
	if isTrue(common.FooterNoTry, common.FooterLegacyNoTry) {
		o.SkipTryjobs = true
	}
	if isTrue(common.FooterNoPresubmit, common.FooterLegacyPresubmit) {
		o.SkipPresubmit = true
	}
	o.IncludedTryjobs = append(o.IncludedTryjobs, valuesOf(
		common.FooterCQIncludeTryjobs,
		common.FooterLegacyCQIncludeTryjobs)...)
	o.OverriddenTryjobs = append(o.OverriddenTryjobs, valuesOf(common.FooterOverrideTryjobsForAutomation, "")...)
	o.CustomTryjobTags = append(o.CustomTryjobTags, valuesOf(common.FooterCQClTag, "")...)
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
		IncludedTryjobs:        append(a.IncludedTryjobs, b.IncludedTryjobs...),
		OverriddenTryjobs:      append(a.OverriddenTryjobs, b.OverriddenTryjobs...),
		CustomTryjobTags:       append(a.CustomTryjobTags, b.CustomTryjobTags...),
	}
}
