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

// Package compilelogs handles downloading logs for compile failures
package compilelog

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/logdog"
	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/util"
)

// GetCompileLogs gets the compile log for a build bucket build
// Returns the ninja log and stdout log
func GetCompileLogs(c context.Context, bbid int64) (*model.CompileLogs, error) {
	build, err := buildbucket.GetBuild(c, bbid, &buildbucketpb.BuildMask{
		Fields: &fieldmaskpb.FieldMask{
			Paths: []string{"steps"},
		},
	})
	if err != nil {
		return nil, err
	}
	failureSummaryUrl := ""
	ninjaUrl := ""
	stdoutUrl := ""
	for _, step := range build.Steps {
		if util.IsCompileStep(step) {
			for _, log := range step.Logs {
				if log.Name == "json.output[failure_summary]" || log.Name == "raw_io.output_text[failure_summary]" {
					failureSummaryUrl = log.ViewUrl
				}
				if log.Name == "json.output[ninja_info]" {
					ninjaUrl = log.ViewUrl
				}
				if log.Name == "stdout" {
					stdoutUrl = log.ViewUrl
				}
			}
			break
		}
	}

	failureSummaryLog := ""
	ninjaLog := &model.NinjaLog{}
	stdoutLog := ""

	// TODO(crbug.com/1295566): Parallelize downloading ninja & stdout logs
	if failureSummaryUrl != "" {
		failureSummaryLog, err = logdog.GetLogFromViewUrl(c, failureSummaryUrl)
		if err != nil {
			logging.Errorf(c, "Failed to get failure summary log: %v", err)
		}
	}

	if ninjaUrl != "" {
		log, err := logdog.GetLogFromViewUrl(c, ninjaUrl)
		if err != nil {
			logging.Errorf(c, "Failed to get ninja log: %v", err)
		}
		if err = json.Unmarshal([]byte(log), ninjaLog); err != nil {
			return nil, fmt.Errorf("Failed to unmarshal ninja log %w. Log: %s", err, log)
		}
	}

	if stdoutUrl != "" {
		stdoutLog, err = logdog.GetLogFromViewUrl(c, stdoutUrl)
		if err != nil {
			logging.Errorf(c, "Failed to get stdout log: %v", err)
		}
	}

	if len(ninjaLog.Failures) > 0 || stdoutLog != "" || failureSummaryLog != "" {
		return &model.CompileLogs{
			FailureSummaryLog: failureSummaryLog,
			NinjaLog:          ninjaLog,
			StdOutLog:         stdoutLog,
		}, nil
	}

	return nil, fmt.Errorf("Could not get compile log from build %d", bbid)
}

func GetFailedTargets(compileLogs *model.CompileLogs) []string {
	if compileLogs.NinjaLog == nil {
		return []string{}
	}
	results := []string{}
	for _, failure := range compileLogs.NinjaLog.Failures {
		results = append(results, failure.OutputNodes...)
	}
	return results
}
