// Copyright 2023 The LUCI Authors.
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

package common

const (
	// FooterNoEquivalentBuilders instructs LUCI CV to ignore the equivalent
	// builder. Meaning always launch the main builder if possible.
	FooterNoEquivalentBuilders = "No-Equivalent-Builders"
	// FooterLegacyNoEquivalentBuilders is the legacy version of
	// FooterNoEquivalentBuilders
	FooterLegacyNoEquivalentBuilders = "NO_EQUIVALENT_BUILDERS"
	// FooterCQDoNotCancelTryjobs instructs LUCI CV to not to cancel the Tryjobs.
	FooterCQDoNotCancelTryjobs = "Cq-Do-Not-Cancel-Tryjobs"
	// FooterNoTreeChecks instructs LUCI CV to skip tree check before submission.
	FooterNoTreeChecks = "No-Tree-Checks"
	// FooterLegacyNoTreeChecks is the legacy version of FooterNoTreeChecks.
	FooterLegacyNoTreeChecks = "NOTREECHECKS"
	// FooterNoTry instructs LUCI CV to skip all Tryjobs except Presubmit.
	FooterNoTry = "No-Try"
	// FooterLegacyNoTry is the legacy version of FooterNoTry
	FooterLegacyNoTry = "NOTRY"
	// FooterNoPresubmit instructs LUCI CV to skip the Presubmit Tryjob.
	//
	// CAVEAT: crbug.com/1292195 - It use `disable_reuse` field to decide
	// whether a Tryjob is Presubmit which produce false positive.
	FooterNoPresubmit = "No-Presubmit"
	// FooterLegacyPresubmit is the legacy version of FooterNoPresubmit.
	FooterLegacyPresubmit = "NOPRESUBMIT"
	// FooterCQIncludeTryjobs specifies the additional Tryjobs to launch.
	FooterCQIncludeTryjobs = "Cq-Include-Trybots"
	// FooterLegacyCQIncludeTryjobs is the legacy version of
	// FooterCQIncludeTryjobs.
	FooterLegacyCQIncludeTryjobs = "CQ_INCLUDE_TRYBOTS"
	// FooterOverrideTryjobsForAutomation provides an list of Tryjobs that
	// overrides ALL the Tryjobs supposed to be launched according to the
	// configuration.
	FooterOverrideTryjobsForAutomation = "Override-Tryjobs-For-Automation"
	// FooterCQClTag specifies the additional Tag that will be added to the
	// launched Tryjobs.
	FooterCQClTag = "Cq-Cl-Tag"
)
