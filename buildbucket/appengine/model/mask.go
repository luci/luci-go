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

package model

import (
	"fmt"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
)

// NoopBuildMask selects all fields.
var NoopBuildMask = &BuildMask{mask.All(&pb.Build{})}

// DefaultBuildMask is the default mask to use for read requests.
var DefaultBuildMask = HardcodedBuildMask(
	"builder",
	"canary",
	"create_time",
	"created_by",
	"critical",
	"end_time",
	"id",
	"input.experimental",
	"input.gerrit_changes",
	"input.gitiles_commit",
	"number",
	"start_time",
	"status",
	"status_details",
	"update_time",
)

// BuildMask knows how to filter pb.Build proto messages.
type BuildMask struct {
	m *mask.Mask
}

// NewBuildMask constructs a build mask either using a legacy `fields` FieldMask
// or new `mask` BuildMask.
//
// legacyPrefix is usually "", but can be "builds" to trim "builds." from
// the legacy field mask (used by SearchBuilds API).
//
// If the mask is empty, returns DefaultBuildMask.
//
// TODO(crbug.com/1219676): Recognize and use `buildMask`.
func NewBuildMask(legacyPrefix string, legacy *fieldmaskpb.FieldMask, buildMask *pb.BuildMask) (*BuildMask, error) {
	if buildMask != nil {
		return nil, errors.Reason("`mask` field is not implemented yet").Err()
	}
	if len(legacy.GetPaths()) == 0 {
		return DefaultBuildMask, nil
	}
	var m *mask.Mask
	var err error
	switch legacyPrefix {
	case "":
		m, err = mask.FromFieldMask(legacy, &pb.Build{}, false, false)
	case "builds":
		m, err = mask.FromFieldMask(legacy, &pb.SearchBuildsResponse{}, false, false)
		if err == nil {
			m, err = m.Submask("builds.*")
		}
	default:
		panic(fmt.Sprintf("unsupported legacy prefix %q", legacyPrefix))
	}
	if err != nil {
		return nil, err
	}
	return &BuildMask{m}, nil
}

// HardcodedBuildMask returns a build mask with given fields.
//
// Panics if some of them are invalid. Intended to be used to initialize
// constants or in tests.
func HardcodedBuildMask(fields ...string) *BuildMask {
	return &BuildMask{mask.MustFromReadMask(&pb.Build{}, fields...)}
}

// Includes returns true if the given field path is included in the mask
// (either partially or entirely).
//
// Panics if the fieldPath is invalid.
func (m *BuildMask) Includes(fieldPath string) bool {
	inc, err := m.m.Includes(fieldPath)
	if err != nil {
		panic(errors.Annotate(err, "bad field path %q", fieldPath).Err())
	}
	return inc != mask.Exclude
}

// Trim applies the mask to the build in-place.
func (m *BuildMask) Trim(b *pb.Build) error {
	return m.m.Trim(b)
}
