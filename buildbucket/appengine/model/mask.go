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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/common/proto/structmask"
)

// The default field mask to use for read requests.
var defaultFieldMask = fieldmaskpb.FieldMask{
	Paths: []string{
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
	},
}

// Used just for their type information.
var (
	buildPrototype       = pb.Build{}
	searchBuildPrototype = pb.SearchBuildsResponse{}
)

// NoopBuildMask selects all fields.
var NoopBuildMask = &BuildMask{m: mask.All(&buildPrototype)}

// DefaultBuildMask is the default mask to use for read requests.
var DefaultBuildMask = HardcodedBuildMask(defaultFieldMask.Paths...)

// BuildMask knows how to filter pb.Build proto messages.
type BuildMask struct {
	m         *mask.Mask         // the overall field mask
	in        *structmask.Filter // "input.properties" filter
	out       *structmask.Filter // "output.properties" filter
	req       *structmask.Filter // "infra.buildbucket.requested_properties" filter
	allFields bool               // Flag for including all fields.
}

// NewBuildMask constructs a build mask either using a legacy `fields` FieldMask
// or new `mask` BuildMask (but not both at the same time, pick one).
//
// legacyPrefix is usually "", but can be "builds" to trim "builds." from
// the legacy field mask (used by SearchBuilds API).
//
// If the mask is empty, returns DefaultBuildMask.
// if mask.AllFields==true, returns BuildMask{allFields:true}.
func NewBuildMask(legacyPrefix string, legacy *fieldmaskpb.FieldMask, bm *pb.BuildMask) (*BuildMask, error) {
	switch {
	case legacy == nil && bm == nil:
		return DefaultBuildMask, nil
	case legacy != nil && bm != nil:
		return nil, errors.Reason("`mask` and `fields` can't be used together, prefer `mask` since `fields` is deprecated").Err()
	case legacy != nil:
		return newLegacyBuildMask(legacyPrefix, legacy)
	}

	if bm.GetAllFields() {
		// All fields should be included.
		if len(bm.GetFields().GetPaths()) > 0 || len(bm.GetInputProperties()) > 0 || len(bm.GetOutputProperties()) > 0 || len(bm.GetRequestedProperties()) > 0 {
			return nil, errors.New("mask.AllFields is mutually exclusive with other mask fields")
		}
		return &BuildMask{allFields: true}, nil
	}

	fm := bm.Fields
	if len(fm.GetPaths()) == 0 {
		fm = &defaultFieldMask
	}

	var cloned bool
	structFilter := func(path string, structMask []*structmask.StructMask) (*structmask.Filter, error) {
		if len(structMask) == 0 {
			return nil, nil
		}
		// Implicitly include struct-valued fields when their masks are present.
		// Make sure not to accidentally override the original FieldMask
		// (in particular when it is &defaultFieldMask).
		if !cloned {
			fm = proto.Clone(fm).(*fieldmaskpb.FieldMask)
			cloned = true
		}
		fm.Paths = append(fm.Paths, path)
		return structmask.NewFilter(structMask)
	}

	// Parse struct masks. This also mutates `fm` to include corresponding fields.
	in, err := structFilter("input.properties", bm.InputProperties)
	if err != nil {
		return nil, errors.Annotate(err, `bad "input_properties" struct mask`).Err()
	}
	out, err := structFilter("output.properties", bm.OutputProperties)
	if err != nil {
		return nil, errors.Annotate(err, `bad "output_properties" struct mask`).Err()
	}
	req, err := structFilter("infra.buildbucket.requested_properties", bm.RequestedProperties)
	if err != nil {
		return nil, errors.Annotate(err, `bad "requested_properties" struct mask`).Err()
	}

	// Construct the overall pb.Build mask.
	var m *mask.Mask
	if fm == &defaultFieldMask {
		// An optimization for the common case, to avoid constructing mask.Mask all
		// the time.
		m = DefaultBuildMask.m
	} else {
		var err error
		if m, err = mask.FromFieldMask(fm, &buildPrototype, false, false); err != nil {
			return nil, err
		}
	}

	// We want to support only field masks compatible with Go protobuf library.
	// Note that "go.chromium.org/luci/common/proto/mask" implements a superset
	// of this functionality. It also returns detailed errors. So we used it first
	// to reject obviously invalid masks (e.g. referring to unknown fields) with
	// nice error messages, and use a blunt IsValid check below to reject no
	// longer supported non-protobuf compatible masks.
	if !fm.IsValid(&buildPrototype) {
		return nil, errors.Reason(
			"the extended field mask syntax is no longer supported, " +
				"use the standard one: " +
				"https://pkg.go.dev/google.golang.org/protobuf/types/known/fieldmaskpb#FieldMask",
		).Err()
	}

	return &BuildMask{
		m:   m,
		in:  in,
		out: out,
		req: req,
	}, nil
}

// newLegacyBuildMask constructs BuildMask from legacy `fields` field.
func newLegacyBuildMask(legacyPrefix string, fields *fieldmaskpb.FieldMask) (*BuildMask, error) {
	if len(fields.GetPaths()) == 0 {
		return DefaultBuildMask, nil
	}
	var m *mask.Mask
	var err error
	switch legacyPrefix {
	case "":
		m, err = mask.FromFieldMask(fields, &buildPrototype, false, false)
	case "builds":
		m, err = mask.FromFieldMask(fields, &searchBuildPrototype, false, false)
		if err == nil {
			m, err = m.Submask("builds.*")
		}
	default:
		panic(fmt.Sprintf("unsupported legacy prefix %q", legacyPrefix))
	}
	if err != nil {
		return nil, err
	}
	return &BuildMask{m: m}, nil
}

// HardcodedBuildMask returns a build mask with given fields.
//
// Panics if some of them are invalid. Intended to be used to initialize
// constants or in tests.
func HardcodedBuildMask(fields ...string) *BuildMask {
	return &BuildMask{m: mask.MustFromReadMask(&buildPrototype, fields...)}
}

// Includes returns true if the given field path is included in the mask
// (either partially or entirely), or the mask includes all fields.
//
// Panics if the fieldPath is invalid.
func (m *BuildMask) Includes(fieldPath string) bool {
	if m.allFields {
		return true
	}
	inc, err := m.m.Includes(fieldPath)
	if err != nil {
		panic(errors.Annotate(err, "bad field path %q", fieldPath).Err())
	}
	return inc != mask.Exclude
}

// Trim applies the mask to the build in-place.
func (m *BuildMask) Trim(b *pb.Build) error {
	if err := m.m.Trim(b); err != nil {
		return err
	}
	if m.in != nil && b.Input != nil {
		b.Input.Properties = m.in.Apply(b.Input.Properties)
	}
	if m.out != nil && b.Output != nil {
		b.Output.Properties = m.out.Apply(b.Output.Properties)
	}
	if m.req != nil && b.Infra != nil && b.Infra.Buildbucket != nil {
		b.Infra.Buildbucket.RequestedProperties = m.req.Apply(b.Infra.Buildbucket.RequestedProperties)
	}
	return nil
}
