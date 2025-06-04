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

package bbfake

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
)

// BuildConstructor provides fluent APIs to reduce the boilerplate
// when constructing test build.
type BuildConstructor struct {
	host                string
	id                  int64
	builderID           *bbpb.BuilderID
	status              bbpb.Status
	createTime          time.Time
	startTime           time.Time
	endTime             time.Time
	updateTime          time.Time
	timeout             bool
	summaryMarkdown     string
	gerritChanges       []*bbpb.GerritChange
	experimental        bool
	requestedProperties *structpb.Struct
	resultdbHost        string
	invocation          string

	template *bbpb.Build
}

// NewBuildConstructor creates a new constructor from scratch.
func NewBuildConstructor() *BuildConstructor {
	return &BuildConstructor{}
}

// NewConstructorFromBuild creates a new constructor with initial value
// populated based on the provided build.
//
// Providing nil build is equivalent to `NewBuildConstructor()`
func NewConstructorFromBuild(build *bbpb.Build) *BuildConstructor {
	if build == nil {
		return NewBuildConstructor()
	}
	bc := &BuildConstructor{
		host:                build.GetInfra().GetBuildbucket().GetHostname(),
		id:                  build.GetId(),
		builderID:           proto.Clone(build.GetBuilder()).(*bbpb.BuilderID),
		status:              build.GetStatus(),
		timeout:             build.GetStatusDetails().GetTimeout() != nil,
		summaryMarkdown:     build.GetSummaryMarkdown(),
		gerritChanges:       make([]*bbpb.GerritChange, len(build.GetInput().GetGerritChanges())),
		experimental:        build.GetInput().GetExperimental(),
		requestedProperties: proto.Clone(build.GetInfra().GetBuildbucket().GetRequestedProperties()).(*structpb.Struct),
		resultdbHost:        build.GetInfra().GetResultdb().GetHostname(),
		invocation:          build.GetInfra().GetResultdb().GetInvocation(),

		template: proto.Clone(build).(*bbpb.Build),
	}

	if createTime := build.GetCreateTime(); createTime != nil {
		bc.createTime = createTime.AsTime()
	}
	if startTime := build.GetStartTime(); startTime != nil {
		bc.startTime = startTime.AsTime()
	}
	if endTime := build.GetEndTime(); endTime != nil {
		bc.endTime = endTime.AsTime()
	}
	if updateTime := build.GetUpdateTime(); updateTime != nil {
		bc.updateTime = updateTime.AsTime()
	}
	for i, gc := range build.GetInput().GetGerritChanges() {
		bc.gerritChanges[i] = proto.Clone(gc).(*bbpb.GerritChange)
	}
	return bc
}

// WithHost specifies the host of this Build. Required.
func (bc *BuildConstructor) WithHost(host string) *BuildConstructor {
	bc.host = host
	return bc
}

// WithID specifies the Build ID. Required.
func (bc *BuildConstructor) WithID(id int64) *BuildConstructor {
	bc.id = id
	return bc
}

// WithBuilderID specifies the Builder. Required.
func (bc *BuildConstructor) WithBuilderID(builderID *bbpb.BuilderID) *BuildConstructor {
	bc.builderID = builderID
	return bc
}

// WithStatus specifies the Build Status. Required.
func (bc *BuildConstructor) WithStatus(status bbpb.Status) *BuildConstructor {
	bc.status = status
	return bc
}

// WithCreateTime specifies the create time. Required.
func (bc *BuildConstructor) WithCreateTime(createTime time.Time) *BuildConstructor {
	bc.createTime = createTime.UTC()
	return bc
}

// WithStartTime specifies the start time. Required if status >= STARTED.
func (bc *BuildConstructor) WithStartTime(startTime time.Time) *BuildConstructor {
	bc.startTime = startTime.UTC()
	return bc
}

// WithEndTime specifies the end time. Required if status is ended.
func (bc *BuildConstructor) WithEndTime(endTime time.Time) *BuildConstructor {
	bc.endTime = endTime.UTC()
	return bc
}

// WithUpdateTime specifies the update time. Optional.
func (bc *BuildConstructor) WithUpdateTime(updateTime time.Time) *BuildConstructor {
	bc.updateTime = updateTime.UTC()
	return bc
}

// WithTimeout sets the timeout bit of this build. Optional.
func (bc *BuildConstructor) WithTimeout(isTimeout bool) *BuildConstructor {
	bc.timeout = isTimeout
	return bc
}

// WithSummaryMarkdown specifies the summary markdown. Optional
func (bc *BuildConstructor) WithSummaryMarkdown(sm string) *BuildConstructor {
	bc.summaryMarkdown = sm
	return bc
}

func (bc *BuildConstructor) WithInvocation(resultdbHost, inv string) *BuildConstructor {
	bc.resultdbHost = resultdbHost
	bc.invocation = inv
	return bc
}

// AppendGerritChanges appends Gerrit changes to this build. Optional.
func (bc *BuildConstructor) AppendGerritChanges(gcs ...*bbpb.GerritChange) *BuildConstructor {
	if len(gcs) == 0 {
		panic("must provide at least one GerritChange")
	}
	for _, gc := range gcs {
		switch {
		case gc.GetHost() == "":
			panic(fmt.Errorf("empty gerrit host"))
		case gc.GetProject() == "":
			panic(fmt.Errorf("empty gerrit repo"))
		case gc.GetChange() == 0:
			panic(fmt.Errorf("zero gerrit CL number"))
		case gc.GetPatchset() == 0:
			panic(fmt.Errorf("zero gerrit CL patchset"))
		}
		bc.gerritChanges = append(bc.gerritChanges, proto.Clone(gc).(*bbpb.GerritChange))
	}
	return bc
}

// ResetGerritChanges clears all existing Gerrit Changes of this build.
func (bc *BuildConstructor) ResetGerritChanges() *BuildConstructor {
	bc.gerritChanges = nil
	return bc
}

// WithExperimental marks this build as experimental build. Optional.
func (bc *BuildConstructor) WithExperimental(exp bool) *BuildConstructor {
	bc.experimental = exp
	return bc
}

// WithRequestedProperties specifies the requested properties. Optional.
//
// The data will be transformed to proto struct format.
func (bc *BuildConstructor) WithRequestedProperties(data map[string]any) *BuildConstructor {
	var err error
	bc.requestedProperties, err = structpb.NewStruct(data)
	if err != nil {
		panic(errors.Fmt("failed to convert to proto struct: %w", err))
	}
	return bc
}

// Construct creates a new build based on supplied inputs.
func (bc *BuildConstructor) Construct() *bbpb.Build {
	switch {
	case bc.host == "":
		panic(fmt.Errorf("empty host"))
	case bc.id == 0:
		panic(fmt.Errorf("zero build ID"))
	case bc.builderID == nil:
		panic(fmt.Errorf("empty builder ID"))
	case bc.status == bbpb.Status_STATUS_UNSPECIFIED:
		panic(fmt.Errorf("unspecified status"))
	case bc.createTime.IsZero():
		panic(fmt.Errorf("zero create time"))
	case bc.status >= bbpb.Status_STARTED && bc.startTime.IsZero():
		panic(fmt.Errorf("zero start time"))
	case protoutil.IsEnded(bc.status) && bc.endTime.IsZero():
		panic(fmt.Errorf("zero end time"))
	}
	ret := bc.template
	if ret == nil {
		ret = &bbpb.Build{}
	}
	ret.Id = bc.id
	ret.Builder = bc.builderID
	ret.Status = bc.status
	ret.CreateTime = timestamppb.New(bc.createTime)
	if !bc.startTime.IsZero() {
		ret.StartTime = timestamppb.New(bc.startTime)
	}
	if !bc.endTime.IsZero() {
		ret.EndTime = timestamppb.New(bc.endTime)
	}
	if !bc.updateTime.IsZero() {
		ret.UpdateTime = timestamppb.New(bc.updateTime)
	}
	if bc.timeout {
		if ret.GetStatusDetails() == nil {
			ret.StatusDetails = &bbpb.StatusDetails{}
		}
		ret.GetStatusDetails().Timeout = &bbpb.StatusDetails_Timeout{}
	}
	ret.SummaryMarkdown = bc.summaryMarkdown
	// Input
	if ret.GetInput() == nil {
		ret.Input = &bbpb.Build_Input{}
	}
	ret.Input.GerritChanges = bc.gerritChanges
	if bc.experimental {
		ret.Input.Experimental = true
		ret.Input.Experiments = append(ret.Input.Experiments, "luci.non_production")
	}
	// Infra
	if ret.GetInfra() == nil {
		ret.Infra = &bbpb.BuildInfra{}
	}
	if ret.GetInfra().GetBuildbucket() == nil {
		ret.Infra.Buildbucket = &bbpb.BuildInfra_Buildbucket{}
	}
	ret.Infra.Buildbucket.Hostname = bc.host
	ret.Infra.Buildbucket.RequestedProperties = bc.requestedProperties

	if ret.GetInfra().GetResultdb() == nil {
		ret.Infra.Resultdb = &bbpb.BuildInfra_ResultDB{}
	}
	ret.Infra.Resultdb.Hostname = bc.resultdbHost
	ret.Infra.Resultdb.Invocation = bc.invocation
	return ret
}
