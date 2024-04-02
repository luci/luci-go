// Copyright 2019 The LUCI Authors.
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
	"encoding/json"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// BuildMediaType is a media type for a binary-encoded Build message.
const BuildMediaType = "application/luci+proto; message=buildbucket.v2.Build"

// SummaryMarkdownMaxLength is the maximum size of Build.summary_markdown field in bytes.
// Find more details at https://godoc.org/go.chromium.org/luci/buildbucket/proto#Build
const SummaryMarkdownMaxLength = 4 * 1000

// RunDuration returns duration between build start and end.
func RunDuration(b *pb.Build) (duration time.Duration, ok bool) {
	if b.StartTime == nil || b.EndTime == nil {
		return 0, false
	}

	start := b.StartTime.AsTime()
	end := b.EndTime.AsTime()
	if start.IsZero() || end.IsZero() {
		return 0, false
	}

	return end.Sub(start), true
}

// SchedulingDuration returns duration between build creation and start.
func SchedulingDuration(b *pb.Build) (duration time.Duration, ok bool) {
	if b.CreateTime == nil || b.StartTime == nil {
		return 0, false
	}

	create := b.CreateTime.AsTime()
	start := b.StartTime.AsTime()
	if create.IsZero() || start.IsZero() {
		return 0, false
	}

	return start.Sub(create), true
}

// Tags parses b.Tags as a strpair.Map.
func Tags(b *pb.Build) strpair.Map {
	m := make(strpair.Map, len(b.Tags))
	for _, t := range b.Tags {
		m.Add(t.Key, t.Value)
	}
	return m
}

// SetStatus sets the status field on `b`, and also correctly adjusts
// StartTime and EndTime.
//
// Will set UpdateTime iff `st` is different than `b.Status`.
func SetStatus(now time.Time, b *pb.Build, st pb.Status) {
	if b.Status == st {
		return
	}
	b.Status = st

	nowPb := timestamppb.New(now.UTC())

	b.UpdateTime = nowPb
	switch {
	case st == pb.Status_STARTED:
		if b.StartTime == nil {
			b.StartTime = nowPb
		}

	case IsEnded(st):
		if b.EndTime == nil {
			b.EndTime = nowPb
		}
	}
}

// ExePayloadPath returns the payload path of the build.
func ExePayloadPath(b *pb.Build) string {
	payloadPath := ""
	for p, purpose := range b.GetInfra().GetBuildbucket().GetAgent().GetPurposes() {
		if purpose == pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD {
			payloadPath = p
		}
	}

	if payloadPath == "" {
		payloadPath = b.GetInfra().GetBbagent().GetPayloadPath()
	}
	return payloadPath
}

// CacheDir returns the cache dir of the build.
func CacheDir(b *pb.Build) string {
	return b.GetInfra().GetBbagent().GetCacheDir()
}

// MergeSummary combines the contents of all summary fields.
func MergeSummary(b *pb.Build) string {
	summaries := []string{
		b.Output.GetSummaryMarkdown(),
		b.Infra.GetBackend().GetTask().GetSummaryMarkdown(),
		b.CancellationMarkdown,
	}

	var contents []string
	if strings.TrimSpace(b.SummaryMarkdown) != "" {
		contents = append(contents, b.SummaryMarkdown)
	}

	for _, s := range summaries {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" && !strings.Contains(b.SummaryMarkdown, trimmed) {
			contents = append(contents, s)
		}
	}

	newSummary := strings.Join(contents, "\n")
	if len(newSummary) > SummaryMarkdownMaxLength {
		return newSummary[:SummaryMarkdownMaxLength-3] + "..."
	}
	return newSummary
}

// BotDimensionsFromBackend retrieves bot dimensions from the backend task running the build.
//
// Exclusively for builds running on backend.
func BotDimensionsFromBackend(b *pb.Build) ([]*pb.StringPair, error) {
	details := b.GetInfra().GetBackend().GetTask().GetDetails().GetFields()
	if len(details) == 0 {
		return nil, nil
	}

	botDims := make([]*pb.StringPair, 0)
	var botDimensions map[string][]string
	if bds, ok := details["bot_dimensions"]; ok {
		bdsJSON, err := bds.MarshalJSON()
		if err != nil {
			return nil, errors.Annotate(err, "failed to marshal task details to JSON for build %d", b.Id).Err()
		}
		err = json.Unmarshal(bdsJSON, &botDimensions)
		if err != nil {
			return nil, errors.Annotate(err, "failed to unmarshal task details JSON for build %d", b.Id).Err()
		}

		for k, vs := range botDimensions {
			for _, v := range vs {
				botDims = append(botDims, &pb.StringPair{Key: k, Value: v})
			}
		}
		SortStringPairs(botDims)
	}
	return botDims, nil
}

// BotDimensions retrieves bot dimensions from the backend task running the build.
//
// Supports both builds running on raw Swarming or backend.
func BotDimensions(b *pb.Build) ([]*pb.StringPair, error) {
	if b.GetInfra().GetSwarming().GetBotDimensions() != nil {
		return b.GetInfra().GetSwarming().GetBotDimensions(), nil
	}
	return BotDimensionsFromBackend(b)
}

// MustBotDimensions retrieves bot dimensions from the backend task running the build.
//
// Supports both builds running on raw Swarming or backend.
//
// panic on error.
func MustBotDimensions(b *pb.Build) []*pb.StringPair {
	botDims, err := BotDimensions(b)
	if err != nil {
		panic(err)
	}
	return botDims
}

// AddBotDimensionsToTaskDetails converts bot_dimensions from
// a list of StringPairs to a field in task details struct.
//
// If details is nil, construct a struct with bot_dimensions as the only
// field.
func AddBotDimensionsToTaskDetails(botDims []*pb.StringPair, details *structpb.Struct) (*structpb.Struct, error) {
	botDimensions := make(map[string][]string)
	for _, dim := range botDims {
		if _, ok := botDimensions[dim.Key]; !ok {
			botDimensions[dim.Key] = make([]string, 0)
		}
		botDimensions[dim.Key] = append(botDimensions[dim.Key], dim.Value)
	}

	detailsMap := make(map[string]any)
	if details != nil {
		detailsMap = details.AsMap()
	}
	detailsMap["bot_dimensions"] = botDimensions

	// Use json as an intermediate format to convert.
	j, err := json.Marshal(detailsMap)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err = json.Unmarshal(j, &m); err != nil {
		return nil, err
	}
	return structpb.NewStruct(m)
}
