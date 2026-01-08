// Copyright 2026 The LUCI Authors.
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

package utils

import (
	"slices"
	"sort"
	"strings"

	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	bqpb "go.chromium.org/luci/analysis/proto/bq/legacy"
)

// moduleParameterExcludeKeys lists variant keys flattened from the root invocation level that should be excluded from AnTS module parameters.
var moduleParameterExcludeKeys = []string{"build_target", "atp_test", "run_target", "cluster_id", "suite_version", "suite_plan"}

func ModuleParametersFromVariants(moduleVariants *rdbpb.Variant) []*bqpb.StringPair {
	moduleParameters := []*bqpb.StringPair{}
	for key, val := range moduleVariants.GetDef() {
		if slices.Contains(moduleParameterExcludeKeys, key) {
			continue
		}
		name := key
		if key == "module_abi" || key == "module_param" {
			// AnTS keys are module-abi and module-param (dash instead of underscore).
			name = strings.ReplaceAll(key, "_", "-")
		}
		moduleParameters = append(moduleParameters, &bqpb.StringPair{
			Name:  name,
			Value: val,
		})
	}
	sort.Slice(moduleParameters, func(i, j int) bool {
		return moduleParameters[i].Name < moduleParameters[j].Name
	})
	return moduleParameters
}

func BuildTypeFromBuildID(buildID string) bqpb.BuildType {
	if strings.HasPrefix(buildID, "P") {
		return bqpb.BuildType_PENDING
	} else if strings.HasPrefix(buildID, "L") {
		return bqpb.BuildType_LOCAL
	} else if strings.HasPrefix(buildID, "T") {
		return bqpb.BuildType_TRAIN
	} else if strings.HasPrefix(buildID, "E") {
		return bqpb.BuildType_EXTERNAL
	} else {
		return bqpb.BuildType_SUBMITTED
	}
}

func ConvertToAnTSStringPair(pairs []*rdbpb.StringPair) []*bqpb.StringPair {
	result := make([]*bqpb.StringPair, 0, len(pairs))
	for _, pair := range pairs {
		result = append(result, &bqpb.StringPair{
			Name:  pair.Key,
			Value: pair.Value,
		})
	}
	return result
}

func ConvertMapToAnTSStringPair(pairs map[string]string) []*bqpb.StringPair {
	result := make([]*bqpb.StringPair, 0, len(pairs))
	for k, v := range pairs {
		result = append(result, &bqpb.StringPair{
			Name:  k,
			Value: v,
		})
	}
	return result
}
