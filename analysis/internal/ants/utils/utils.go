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
	"sort"
	"strings"

	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	bqpb "go.chromium.org/luci/analysis/proto/bq/legacy"
)

func ModuleParametersFromVariants(moduleVariants *rdbpb.Variant) []*bqpb.StringPair {
	moduleParameters := []*bqpb.StringPair{}
	for key, val := range moduleVariants.GetDef() {
		moduleParameters = append(moduleParameters, &bqpb.StringPair{
			Name:  key,
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

// FindKeyFromTags finds the first occurrence of the key in tags and returns the value.
// Returns empty string if the key is not found.
func FindKeyFromTags(key string, tags []*rdbpb.StringPair) string {
	for _, tag := range tags {
		if tag.Key == key {
			return tag.Value
		}
	}
	return ""
}

// FindKeysFromTags finds all occurrences of the key in tags and returns their values.
func FindKeysFromTags(key string, tags []*rdbpb.StringPair) []string {
	results := []string{}
	for _, tag := range tags {
		if tag.Key == key {
			results = append(results, tag.Value)
		}
	}
	return results
}
