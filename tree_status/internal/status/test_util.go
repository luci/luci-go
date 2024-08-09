// Copyright 2024 The LUCI Authors.
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

// Package status manages status values.
package status

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/server/span"

	pb "go.chromium.org/luci/tree_status/proto/v1"
)

type StatusBuilder struct {
	status Status
}

func NewStatusBuilder() *StatusBuilder {
	id, err := GenerateID()
	if err != nil {
		panic(err)
	}
	return &StatusBuilder{status: Status{
		TreeName:      "chromium",
		StatusID:      id,
		GeneralStatus: pb.GeneralState_OPEN,
		Message:       "Tree is open!",
		CreateUser:    "user1",
		CreateTime:    spanner.CommitTimestamp,
	}}
}

func (b *StatusBuilder) WithTreeName(treeName string) *StatusBuilder {
	b.status.TreeName = treeName
	return b
}

func (b *StatusBuilder) WithStatusID(id string) *StatusBuilder {
	b.status.StatusID = id
	return b
}

func (b *StatusBuilder) WithGeneralStatus(state pb.GeneralState) *StatusBuilder {
	b.status.GeneralStatus = state
	return b
}

func (b *StatusBuilder) WithMessage(message string) *StatusBuilder {
	b.status.Message = message
	return b
}

func (b *StatusBuilder) WithCreateTime(createTime time.Time) *StatusBuilder {
	b.status.CreateTime = createTime
	return b
}

func (b *StatusBuilder) WithCreateUser(user string) *StatusBuilder {
	b.status.CreateUser = user
	return b
}

func (b *StatusBuilder) WithClosingBuilderName(closingBuilderName string) *StatusBuilder {
	b.status.ClosingBuilderName = closingBuilderName
	return b
}

func (b *StatusBuilder) Build() *Status {
	s := b.status
	return &s
}

func (b *StatusBuilder) CreateInDB(ctx context.Context) *Status {
	s := b.Build()
	row := map[string]any{
		"TreeName":           s.TreeName,
		"StatusId":           s.StatusID,
		"GeneralStatus":      int64(s.GeneralStatus),
		"Message":            s.Message,
		"CreateUser":         s.CreateUser,
		"CreateTime":         s.CreateTime,
		"ClosingBuilderName": s.ClosingBuilderName,
	}
	m := spanner.InsertOrUpdateMap("Status", row)
	ts, err := span.Apply(ctx, []*spanner.Mutation{m})
	if err != nil {
		panic(err)
	}
	if s.CreateTime == spanner.CommitTimestamp {
		s.CreateTime = ts.UTC()
	}
	return s
}
