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
	"net/url"
	"regexp"
	"sort"
	"strings"

	v1 "go.chromium.org/luci/cq/api/config/v1"
)

func validateV1(cfg *v1.Config) (response validationResponseMessage) {
	// Required fields.
	if cfg.GetVersion() != 1 {
		response.addError("version is a required field and must be 1")
	}

	if cfg.Verifiers == nil {
		response.addError("verifiers is a required field")
	}

	validateCodereviewV1(cfg, &response)

	if cfg.GitRepoUrl == nil {
		response.addError("git_repo_url is required")
	} else {
		switch parsed, err := url.ParseRequestURI(cfg.GetGitRepoUrl()); {
		case err != nil:
			response.addError("git_repo_url must be a valid url: %s", err)
		case (!strings.HasSuffix(parsed.Host, ".googlesource.com") ||
			parsed.Host == ".googlesource.com"):
			// TODO(tandrii): maybe support other Gerrit hosts.
			response.addError("git_repo_url must match https://*.googlesource.com")
		}
	}

	screenForLegacyOptions(cfg, &response)
	screenForInternalFieldsV1(cfg, &response)

	if cfg.Gerrit != nil &&
		cfg.GetVerifiers().GetGerritCqAbility() != nil &&
		cfg.Verifiers.GerritCqAbility.CommitterList == nil {
		response.addError("verifiers.gerrit_cq_ability requires committer_list to be set")
	}

	// Dangerous fields.
	if cfg.DrainingStartTime != nil {
		response.addWarning("draining_start_time %s is specified. Don't forget to remove!",
			cfg.GetDrainingStartTime())
	}

	validateTryJobVerifierV1(cfg.GetVerifiers().GetTryJob(), &response)
	return
}

func validateCodereviewV1(cfg *v1.Config, v *validationResponseMessage) {
	if cfg.Gerrit == nil {
		v.addError("gerrit is required")
		return
	}
	if cfg.GetVerifiers().GetGerritCqAbility() == nil {
		v.addError("gerrit requires gerrit_cq_ability verifier to be used.")
	}
}

func screenForLegacyOptions(cfg *v1.Config, v *validationResponseMessage) {
	// Legacy fields.
	if cfg.GetCqName() != "" {
		v.addError("cq_name is no longer used and can and should be removed. Please, do so now")
	}
	if cfg.InProduction != nil {
		v.addError("is_production is no longer supported.")
	}
	if cfg.GetGerrit().GetCqVerifiedLabel() != "" {
		v.addError("gerrit.cq_verified_label is no longer supported.")
	}
	if cfg.Gerrit.GetDryRunSetsCqVerifiedLabel() {
		v.addError("gerrit.dry_run_sets_cq_verified_label is no longer supported.")
	}
}

// screenForInternalFieldsV1 ensures no fields for internal CQ use are specified.
func screenForInternalFieldsV1(cfg *v1.Config, v *validationResponseMessage) {
	if cfg.GetVerifiers().GetDeprecator() != nil {
		v.addError("deprecator verifier is not allowed in customer configs. Please, remove.")
	}
	if cfg.GetVerifiers().GetFake() != nil {
		v.addError("fake verifier is not allowed in customer configs. Please, remove.")
	}
}

func checkUniqueBucketNames(tryCfg *v1.Verifiers_TryJobVerifier, v *validationResponseMessage) bool {
	bucket_names := map[string]bool{}
	hasError := false
	for _, bucketCfg := range tryCfg.GetBuckets() {
		if bucketCfg.GetName() == "" {
			v.addError("Bucket name must be given")
			hasError = true
			continue
		}
		if _, ok := bucket_names[*bucketCfg.Name]; !ok {
			bucket_names[*bucketCfg.Name] = true
		} else {
			hasError = true
			v.addError("Bucket '%s' has been defined more than once", *bucketCfg.Name)
		}
	}
	return !hasError
}

// buildersValidatorHelper stores state of non-trivial validation of builders of a bucket.
type buildersValidatorHelper struct {
	bucketCfg      *v1.Verifiers_TryJobVerifier_Bucket
	v              *validationResponseMessage
	triggersMap    map[string][]string
	eqMap          map[string]bool
	expMap         map[string]bool
	canBeTriggered map[string]bool
}

// checkUniqueNames ensures all builders have a unique name and initializes triggersMap.
func (h *buildersValidatorHelper) checkUniqueNames() bool {
	h.triggersMap = map[string][]string{}
	hasError := false
	for _, builderCfg := range h.bucketCfg.GetBuilders() {
		if builderCfg.GetName() == "" {
			h.v.addError("Bucket '%s' has builder without name", *h.bucketCfg.Name)
			hasError = true
			continue
		}
		if _, ok := h.triggersMap[*builderCfg.Name]; !ok {
			h.triggersMap[*builderCfg.Name] = []string{}
		} else {
			h.v.addError("Bucket '%s' builder '%s' has been defined more than once", *h.bucketCfg.Name,
				*builderCfg.Name)
			hasError = true
		}
	}
	return !hasError
}

// checkLegacyPresubmit ensures all builders with 'presubmit' in their name
// have disable_reuse set to true. See also https://crbug.com/893955.
// Never fails, merely emits warnings.
func (h *buildersValidatorHelper) checkLegacyPresubmit() bool {
	for _, builderCfg := range h.bucketCfg.GetBuilders() {
		if strings.Contains(strings.ToLower(builderCfg.GetName()), "presubmit") &&
			!builderCfg.GetDisableReuse() {
			h.v.addWarning(
				"Bucket '%s' builder '%s' ought to have `disable_reuse: true` set "+
					"because its name contains 'presubmit'. If you intend for this builder "+
					"to be actually re-used, please ignore ths warning. "+
					"See also https://crbug.com/893955",
				*h.bucketCfg.Name,
				*builderCfg.Name)
		}
	}
	return true
}

// checkEquivalentTo validates builders with `equivalent_to` blocks and computes h.eqMap.
func (h *buildersValidatorHelper) checkEquivalentTo() bool {
	h.eqMap = map[string]bool{}
	hasError := false
	for _, builderCfg := range h.bucketCfg.GetBuilders() {
		if eqCfg := builderCfg.GetEquivalentTo(); eqCfg != nil {
			if builderCfg.TriggeredBy != nil {
				h.v.addError("Bucket '%s' builder '%s' has `equivalent_to` and `triggered_by`, which is not allowed",
					*h.bucketCfg.Name, *builderCfg.Name)
				hasError = true
			}
			if builderCfg.ExperimentPercentage != nil {
				h.v.addError("Bucket '%s' builder '%s' has `equivalent_to` and `experiment_percentage`, which is not allowed",
					*h.bucketCfg.Name, *builderCfg.Name)
				hasError = true
			}
			if eqCfg.GetBucket() == "" {
				h.v.addError("Bucket '%s' builder '%s' `equivalent_to` needs a specified `bucket`",
					*h.bucketCfg.Name, *builderCfg.Name)
				hasError = true
			}
			h.eqMap[*builderCfg.Name] = true
		}
	}
	return !hasError
}

// checkPathBased validates builders with `path_regexp` and
// `path_regexp_exclude` blocks.
func (h *buildersValidatorHelper) checkPathBased() bool {
	hasError := false
	for _, builderCfg := range h.bucketCfg.GetBuilders() {
		regexps := builderCfg.GetPathRegexp()
		regexpsExclude := builderCfg.GetPathRegexpExclude()
		if len(regexps)+len(regexpsExclude) == 0 {
			continue
		}
		definedField := "path_regexp"
		if len(regexps) == 0 {
			definedField = "path_regexp_exclude"
		}
		if builderCfg.TriggeredBy != nil {
			h.v.addError("Bucket '%s' builder '%s' has `%s` and `triggered_by`, which is not allowed",
				*h.bucketCfg.Name, *builderCfg.Name, definedField)
			hasError = true
		}
		if builderCfg.ExperimentPercentage != nil {
			h.v.addError("Bucket '%s' builder '%s' has `%s` and `experiment_percentage`, which is not allowed",
				*h.bucketCfg.Name, *builderCfg.Name, definedField)
			hasError = true
		}
		if builderCfg.GetEquivalentTo() != nil {
			h.v.addError("Bucket '%s' builder '%s' has `%s` and `equivalent_to`, which is not allowed",
				*h.bucketCfg.Name, *builderCfg.Name, definedField)
			hasError = true
		}
		// Validate regexes.
		for _, expr := range regexps {
			if _, err := regexp.Compile(expr); err != nil {
				h.v.addError("Bucket '%s' builder '%s' path_regexp=%q is invalid regexp: %s",
					*h.bucketCfg.Name, *builderCfg.Name, expr, err)
				hasError = true
			}
		}
		for _, expr := range regexpsExclude {
			if _, err := regexp.Compile(expr); err != nil {
				h.v.addError("Bucket '%s' builder '%s' path_regexp_exclude=%q is invalid regexp: %s",
					*h.bucketCfg.Name, *builderCfg.Name, expr, err)
				hasError = true
			}
		}
	}
	return !hasError
}

// checkExperimental validates builders with `experiment_percentage` and computes h.expMap.
func (h *buildersValidatorHelper) checkExperiment() bool {
	h.expMap = map[string]bool{}
	hasError := false
	for _, builderCfg := range h.bucketCfg.GetBuilders() {
		if builderCfg.ExperimentPercentage != nil {
			h.expMap[*builderCfg.Name] = true
			if (*builderCfg.ExperimentPercentage < 0) || (*builderCfg.ExperimentPercentage > 100) {
				h.v.addError("Bucket '%s' builder '%s' `experimental_percentage` %f must be within 0..100",
					*h.bucketCfg.Name, *builderCfg.Name, *builderCfg.ExperimentPercentage)
				hasError = true
			}
			if builderCfg.TriggeredBy != nil {
				h.v.addError("Bucket '%s' builder '%s' has `experiment_percentage` and `triggered_by`, which is not allowed",
					*h.bucketCfg.Name, *builderCfg.Name)
				hasError = true
			}
		}
	}
	return !hasError
}

// checkTriggerByReferences ensures triggerred_by reference is to the existing builder.
// This function also completes triggersMap to be a map from builder name to all builders it triggers.
func (h *buildersValidatorHelper) checkTriggerByReferences() bool {
	hasError := false
	for _, builderCfg := range h.bucketCfg.GetBuilders() {
		if builderCfg.TriggeredBy != nil {
			if _, ok := h.eqMap[*builderCfg.TriggeredBy]; ok {
				h.v.addError("Bucket '%s' builder '%s' is triggered by '%s', "+
					"which has an `equivalent_to` and this is not allowed",
					*h.bucketCfg.Name, *builderCfg.Name, *builderCfg.TriggeredBy)
				hasError = true
			}
			if _, ok := h.expMap[*builderCfg.TriggeredBy]; ok {
				h.v.addError("Bucket '%s' builder '%s' is triggered by '%s', "+
					"which has an `experiment_percentage` and this is not allowed",
					*h.bucketCfg.Name, *builderCfg.Name, *builderCfg.TriggeredBy)
				hasError = true
			}
			if triggers, ok := h.triggersMap[*builderCfg.TriggeredBy]; ok {
				h.triggersMap[*builderCfg.TriggeredBy] = append(triggers, *builderCfg.Name)
			} else {
				h.v.addError("Bucket '%s' builder '%s' triggered_by non-existent builder '%s'",
					*h.bucketCfg.Name, *builderCfg.Name, *builderCfg.TriggeredBy)
				hasError = true
			}
		}
	}
	return !hasError
}

// checkIfCanTriggerAll checks whether CQ will be able to trigger all builds either directly or indirectly.
func (h *buildersValidatorHelper) checkIfCanTriggerAll() bool {
	// DFS from all builders which aren't triggered by any other builder.
	h.canBeTriggered = map[string]bool{}
	for _, builderCfg := range h.bucketCfg.GetBuilders() {
		if builderCfg.TriggeredBy == nil {
			h.dfs(*builderCfg.Name)
		}
	}
	if len(h.canBeTriggered) == len(h.triggersMap) {
		return true
	}

	bad := []string{}
	for name, _ := range h.triggersMap {
		if _, ok := h.canBeTriggered[name]; !ok {
			bad = append(bad, "'"+name+"'")
		}
	}
	sort.Strings(bad)
	h.v.addError(
		"Bucket '%s' builders [%s] are triggered_by each other and neither can be triggered by CQ directly",
		*h.bucketCfg.Name, strings.Join(bad, ", "))
	return false
}

func (h *buildersValidatorHelper) dfs(name string) {
	h.canBeTriggered[name] = true
	for _, triggered := range h.triggersMap[name] {
		h.dfs(triggered)
	}
}

// checkBuilderNumber ensures there are not too many CQ builders.
func (h *buildersValidatorHelper) checkBuilderNumber() bool {
	// CQ wasn't coded with 1k builders per bucket in mind, abort now to avoid bad UX for this and other
	// projects.
	if len(h.bucketCfg.GetBuilders()) > 193 {
		h.v.addError("CQ allows at most 193 builders (%d specified); contact CQ team with your use case",
			len(h.bucketCfg.GetBuilders()))
		return false
	}
	return true
}

// checkNoAliasingInEquivalentTo ensures that equivalent_to alternative builders are all unique and don't
// overlap the rest (main) builders.
//
// Assumes that config passed has verified names of buckets and builders and their uniqueness.
func checkNoAliasingInEquivalentTo(tryCfg *v1.Verifiers_TryJobVerifier, v *validationResponseMessage) {
	type key struct{ bucket, builder string }
	main := map[key]bool{}
	equi := map[key]bool{}
	for _, bucketCfg := range tryCfg.GetBuckets() {
		for _, builderCfg := range bucketCfg.GetBuilders() {
			k := key{*bucketCfg.Name, *builderCfg.Name}
			main[k] = true
			if _, ok := equi[k]; ok {
				v.addError("Bucket '%s' builder '%s' should not be in main and equivalent_to places at the same time",
					k.bucket, k.builder)
			}

			if eq := builderCfg.GetEquivalentTo(); eq != nil {
				if eq.Builder == nil {
					k = key{*eq.Bucket, *builderCfg.Name}
				} else {
					k = key{*eq.Bucket, *eq.Builder}
				}
				if _, aliases := main[k]; aliases {
					v.addError("Bucket '%s' builder '%s' should not be in main and equivalent_to places at the same time",
						k.bucket, k.builder)
				}

				if _, ok := equi[k]; ok {
					v.addError("Bucket '%s' builder '%s' should not be in more than one equivalent_to sections",
						k.bucket, k.builder)
				}
				equi[k] = true
			}
		}
	}
}

func validateTryJobVerifierV1(tryCfg *v1.Verifiers_TryJobVerifier, v *validationResponseMessage) {
	if tryCfg.GetBuckets() == nil {
		return
	}
	if !checkUniqueBucketNames(tryCfg, v) {
		return
	}

	hasError := false
	for _, bucketCfg := range tryCfg.GetBuckets() {
		h := buildersValidatorHelper{bucketCfg, v, nil, nil, nil, nil}
		if !(h.checkBuilderNumber() &&
			h.checkUniqueNames() &&
			h.checkLegacyPresubmit() &&
			h.checkExperiment() &&
			h.checkEquivalentTo() &&
			h.checkPathBased() &&
			h.checkTriggerByReferences() &&
			h.checkIfCanTriggerAll()) {
			hasError = true
		}
	}
	if hasError {
		return
	}

	checkNoAliasingInEquivalentTo(tryCfg, v)
}
