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

// Package loggingutil contains utility functions for logging.
package loggingutil

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/bisection/util/datastoreutil"
)

// UpdateLoggingWithAnalysisID returns a context with logging field analysis_id
// and the corresponding analyzed_bbid set.
func UpdateLoggingWithAnalysisID(c context.Context, analysisID int64) (context.Context, error) {
	c = SetAnalysisID(c, analysisID)
	cfa, err := datastoreutil.GetCompileFailureAnalysis(c, analysisID)
	if err != nil {
		return c, errors.Annotate(err, "failed GetCompileFailureAnalysis ID: %d", analysisID).Err()
	}
	if cfa.CompileFailure != nil && cfa.CompileFailure.Parent() != nil {
		c = SetAnalyzedBBID(c, cfa.CompileFailure.Parent().IntID())
	}
	return c, nil
}

// SetAnalyzedBBID returns a context with the logging field analyzed_bbid set
func SetAnalyzedBBID(c context.Context, bbid int64) context.Context {
	return logging.SetField(c, "analyzed_bbid", bbid)
}

// SetAnalysisID returns a context with the logging field analysis_id set
func SetAnalysisID(c context.Context, analysisID int64) context.Context {
	return logging.SetField(c, "analysis_id", analysisID)
}

// SetRerunBBID returns a context with the logging field rerun_bbid set
func SetRerunBBID(c context.Context, rerunBBID int64) context.Context {
	return logging.SetField(c, "rerun_bbid", rerunBBID)
}

// SetQueryBBID returns a context with the logging field query_bbid set
func SetQueryBBID(c context.Context, bbid int64) context.Context {
	return logging.SetField(c, "query_bbid", bbid)
}

// SetProject returns a context with the logging field project set
func SetProject(c context.Context, project string) context.Context {
	return logging.SetField(c, "project", project)
}
