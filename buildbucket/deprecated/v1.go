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

package deprecated

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"

	structpb "github.com/golang/protobuf/ptypes/struct"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	pb "go.chromium.org/luci/buildbucket/proto"
	v1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
)

// This file implements v1<->v2 interoperation.

// MalformedBuild tag is present in an error if the build was malformed.
var MalformedBuild = errors.BoolTag{Key: errors.NewTagKey("malformed buildbucket v1 build")}

// StatusToV2 converts v1 build's Status, Result, FailureReason and
// CancelationReason to v2 Status enum.
//
// If build.Status is "", returns (Status_STATUS_UNSPECIFIED, nil).
// Useful with partial buildbucket responses.
func StatusToV2(build *v1.LegacyApiCommonBuildMessage) (pb.Status, error) {
	switch build.Status {
	case "":
		return pb.Status_STATUS_UNSPECIFIED, nil

	case "SCHEDULED":
		return pb.Status_SCHEDULED, nil

	case "STARTED":
		return pb.Status_STARTED, nil

	case "COMPLETED":
		switch build.Result {
		case "SUCCESS":
			return pb.Status_SUCCESS, nil

		case "FAILURE":
			switch build.FailureReason {
			case "", "BUILD_FAILURE":
				return pb.Status_FAILURE, nil
			case "INFRA_FAILURE", "BUILDBUCKET_FAILURE", "INVALID_BUILD_DEFINITION":
				return pb.Status_INFRA_FAILURE, nil
			default:
				return 0, errors.Reason("unexpected failure reason %q", build.FailureReason).Tag(MalformedBuild).Err()
			}

		case "CANCELED":
			switch build.CancelationReason {
			case "", "CANCELED_EXPLICITLY":
				return pb.Status_CANCELED, nil
			case "TIMEOUT":
				return pb.Status_INFRA_FAILURE, nil
			default:
				return 0, errors.Reason("unexpected cancellation reason %q", build.CancelationReason).Tag(MalformedBuild).Err()
			}

		default:
			return 0, errors.Reason("unexpected result %q", build.Result).Tag(MalformedBuild).Err()
		}

	default:
		return 0, errors.Reason("unexpected status %q", build.Status).Tag(MalformedBuild).Err()
	}
}

type v1Params struct {
	Builder    string          `json:"builder_name"`
	Properties json.RawMessage `json:"properties"`
}

// BuildToV2 converts a v1 build message to v2.
//
// The returned build may be incomplete if msg is incomplete.
// For example, if msg is a partial response and does not have builder name,
// the returned build won't have it either.
//
// The returned build does not include steps.
// Returns an error if msg is malformed.
func BuildToV2(msg *v1.LegacyApiCommonBuildMessage) (b *pb.Build, err error) {
	// This implementation is a port of
	// https://chromium.googlesource.com/infra/infra/+/d55f587c0f30b0297e4d134c698e7458baa39b7f/appengine/cr-buildbucket/v2/builds.py#21

	params := &v1Params{}
	if msg.ParametersJson != "" {
		if err = json.NewDecoder(strings.NewReader(msg.ParametersJson)).Decode(params); err != nil {
			return nil, errors.Annotate(err, "ParametersJson is invalid").Tag(MalformedBuild).Err()
		}
	}

	resultDetails := &struct {
		Properties json.RawMessage                 `json:"properties"`
		TaskResult swarming.SwarmingRpcsTaskResult `json:"task_result"`
		UI         struct {
			Info string `json:"info"`
		} `json:"ui"`
	}{}
	if msg.ResultDetailsJson != "" {
		if err = json.NewDecoder(strings.NewReader(msg.ResultDetailsJson)).Decode(resultDetails); err != nil {
			return nil, errors.Annotate(err, "ResultDetailsJson is invalid").Tag(MalformedBuild).Err()
		}
	}

	tags := strpair.ParseMap(msg.Tags)
	address := tags.Get(v1.TagBuildAddress)
	var number int
	if address != "" {
		_, _, _, _, number, err = v1.ParseBuildAddress(address)
		if err != nil {
			return nil, errors.Annotate(err, "invalid build address %q", address).Tag(MalformedBuild).Err()
		}
	}

	status, err := StatusToV2(msg)
	if err != nil {
		return nil, err
	}

	builder, err := builderToV2(msg, tags, params)
	if err != nil {
		return nil, err
	}

	b = &pb.Build{
		Id:        msg.Id,
		Builder:   builder,
		Number:    int32(number),
		CreatedBy: msg.CreatedBy,

		CreateTime: timestampToV2(msg.CreatedTs),
		StartTime:  timestampToV2(msg.StartedTs),
		EndTime:    timestampToV2(msg.CompletedTs),
		UpdateTime: timestampToV2(msg.UpdatedTs),

		Status:          status,
		SummaryMarkdown: resultDetails.UI.Info,
		Canary:          msg.Canary,

		Input: &pb.Build_Input{
			Experimental: msg.Experimental,
		},
		Output: &pb.Build_Output{},

		Infra: &pb.BuildInfra{
			Buildbucket: &pb.BuildInfra_Buildbucket{},
			Swarming: &pb.BuildInfra_Swarming{
				Hostname:           tags.Get("swarming_hostname"),
				TaskId:             tags.Get("swarming_task_id"),
				TaskServiceAccount: msg.ServiceAccount,
			},
		},
	}

	if b.Input.Properties, err = propertiesToV2(params.Properties); err != nil {
		return nil, errors.Annotate(err, "invalid input properties").Tag(MalformedBuild).Err()
	}
	if b.Output.Properties, err = propertiesToV2(resultDetails.Properties); err != nil {
		return nil, errors.Annotate(err, "invalid output properties").Tag(MalformedBuild).Err()
	}

	b.Infra.Swarming.BotDimensions = make([]*pb.StringPair, 0, len(resultDetails.TaskResult.BotDimensions))
	for _, d := range resultDetails.TaskResult.BotDimensions {
		for _, v := range d.Value {
			b.Infra.Swarming.BotDimensions = append(b.Infra.Swarming.BotDimensions, &pb.StringPair{
				Key:   d.Key,
				Value: v,
			})
		}
	}

	if err := tagsToV2(b, msg.Tags); err != nil {
		return nil, err
	}
	return b, nil
}

func tagsToV2(dest *pb.Build, tags []string) error {
	dest.Input.GitilesCommit = nil
	for _, t := range toStringPairs(tags) {
		switch t.Key {
		case v1.TagBuilder, v1.TagBuildAddress:
			// We've already parsed these tags.

		case v1.TagBuildSet:
			switch bs := protoutil.ParseBuildSet(t.Value).(type) {
			case *pb.GerritChange:
				dest.Input.GerritChanges = append(dest.Input.GerritChanges, bs)
			case *pb.GitilesCommit:
				if dest.Input.GitilesCommit != nil {
					return errors.Reason("more than one gitiles commit buildset").Tag(MalformedBuild).Err()
				}
				dest.Input.GitilesCommit = bs
			default:
				dest.Tags = append(dest.Tags, t)
			}

		case "swarming_dimension":
			if d := toStringPair(t.Value); d != nil {
				dest.Infra.Swarming.TaskDimensions = append(dest.Infra.Swarming.TaskDimensions, &pb.RequestedDimension{
					Key:   d.Key,
					Value: d.Value,
				})
			}

		case "swarming_tag":
			if st := toStringPair(t.Value); st != nil {
				switch st.Key {
				case "priority":
					pri, _ := strconv.ParseInt(st.Value, 10, 32)
					dest.Infra.Swarming.Priority = int32(pri)
				case "buildbucket_template_revision":
					dest.Infra.Buildbucket.ServiceConfigRevision = st.Value
				}
			}

		default:
			dest.Tags = append(dest.Tags, t)
		}
	}
	return nil
}

// BucketNameToV2 converts a v1 Bucket name to the v2 constituent parts.
// An error is returned if the bucketname does not match the expected format.
// The difference between the bucket name is that v2 uses short names, for example:
// v1: luci.chromium.try
// v2: try
// "luci" is dropped, "chromium" is recorded as the project, "try" is the name.
// If the bucket does not conform to this convention, or if it is not a luci bucket,
// then this return and empty string for both project and bucket.
func BucketNameToV2(v1Bucket string) (project string, bucket string) {
	p := strings.SplitN(v1Bucket, ".", 3)
	if len(p) != 3 || p[0] != "luci" {
		return "", ""
	}
	return p[1], p[2]
}

// builderToV2 attempts to parse as many fields into bucket and project as possible,
// and do project name validation if the project is available.
func builderToV2(msg *v1.LegacyApiCommonBuildMessage, tags strpair.Map, params *v1Params) (ret *pb.BuilderID, err error) {
	ret = &pb.BuilderID{Builder: params.Builder}
	if ret.Builder == "" {
		ret.Builder = tags.Get(v1.TagBuilder) // Fallback: Grab builder name from tags.
	}

	ret.Project, ret.Bucket = BucketNameToV2(msg.Bucket)
	if msg.Project != "" && ret.Project != "" && ret.Project != msg.Project {
		err = errors.Reason(
			"message project %q does not match bucket project %q", msg.Project, ret.Project).Tag(MalformedBuild).Err()
	}
	return
}

func timestampToV2(ts int64) *tspb.Timestamp {
	if ts == 0 {
		return nil
	}
	ret, _ := ptypes.TimestampProto(v1.ParseTimestamp(ts))
	return ret
}

func propertiesToV2(v1 json.RawMessage) (*structpb.Struct, error) {
	if len(v1) == 0 {
		return nil, nil
	}
	ret := &structpb.Struct{}
	return ret, jsonpb.UnmarshalString(string(v1), ret)
}

func toStringPair(s string) *pb.StringPair {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return nil
	}
	return &pb.StringPair{Key: parts[0], Value: parts[1]}
}

func toStringPairs(tags []string) []*pb.StringPair {
	ret := make([]*pb.StringPair, 0, len(tags))
	for _, t := range tags {
		if p := toStringPair(t); p != nil {
			ret = append(ret, p)
		}
	}
	return ret
}
