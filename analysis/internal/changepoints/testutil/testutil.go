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

// Package testutil contains utility functions for tests.
package testutil

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func SamplePayload() *taskspb.IngestTestVerdicts {
	return &taskspb.IngestTestVerdicts{
		Project: "chromium",
		Invocation: &controlpb.InvocationResult{
			InvocationId: "build-1234",
			ResultdbHost: "rdbHost",
		},
		PresubmitRun: &controlpb.PresubmitResult{
			Status: pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_SUCCEEDED,
			Mode:   pb.PresubmitRunMode_FULL_RUN,
		},
		PartitionTime: timestamppb.New(time.Unix(3600*10, 0)),
	}
}

func SampleSourcesMap(commitPosition int) map[string]*pb.Sources {
	return map[string]*pb.Sources{
		"sources_id": {
			GitilesCommit: &pb.GitilesCommit{
				Host:     "host",
				Project:  "proj",
				Ref:      "ref",
				Position: int64(commitPosition),
			},
		},
	}
}

func SampleSourcesWithChangelistsMap(commitPosition int) map[string]*pb.Sources {
	return map[string]*pb.Sources{
		"sources_id": {
			GitilesCommit: &pb.GitilesCommit{
				Host:     "host",
				Project:  "proj",
				Ref:      "ref",
				Position: int64(commitPosition),
			},
			Changelists: []*pb.GerritChange{
				{
					Host:     "gerrit-host",
					Project:  "gerrit-proj",
					Change:   15,
					Patchset: 16,
				},
			},
		},
	}
}

func TestConfig() *configpb.Config {
	return &configpb.Config{
		TestVariantAnalysis: &configpb.TestVariantAnalysis{
			Enabled:               true,
			BigqueryExportEnabled: true,
		},
	}
}
