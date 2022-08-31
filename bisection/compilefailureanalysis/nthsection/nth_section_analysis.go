// Copyright 2022 The LUCI Authors.
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

package nthsection

import (
	"context"

	gfim "go.chromium.org/luci/bisection/model"
	gfipb "go.chromium.org/luci/bisection/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"
)

func Analyze(
	c context.Context,
	cfa *gfim.CompileFailureAnalysis,
	rr *gfipb.RegressionRange) (*gfim.CompileNthSectionAnalysis, error) {
	// Create a new CompileNthSectionAnalysis Entity
	nth_section_analysis := &gfim.CompileNthSectionAnalysis{
		ParentAnalysis: datastore.KeyForObj(c, cfa),
		StartTime:      clock.Now(c),
		Status:         gfipb.AnalysisStatus_CREATED,
	}

	if err := datastore.Put(c, nth_section_analysis); err != nil {
		return nil, err
	}

	// TODO (nqmtuan) implement nth section analysis
	return nth_section_analysis, nil
}
