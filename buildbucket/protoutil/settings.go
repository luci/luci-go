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

package protoutil

import (
	"regexp"

	"go.chromium.org/luci/common/data/stringset"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// ExperimentNameRE is the regular expression that a valid Buildbucket
// experiment name must match.
var ExperimentNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)*$`)

// WellKnownExperiments computes all known 'global' experiments from the global
// SettingsCfg.
func WellKnownExperiments(globalCfg *pb.SettingsCfg) stringset.Set {
	experiments := globalCfg.GetExperiment().GetExperiments()
	ret := stringset.New(len(experiments))
	for _, exp := range experiments {
		ret.Add(exp.Name)
	}
	return ret
}
