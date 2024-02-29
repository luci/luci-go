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

package teams

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/server/span"
)

type TeamBuilder struct {
	team Team
}

func NewTeamBuilder() *TeamBuilder {
	id, err := GenerateID()
	if err != nil {
		panic(err)
	}
	return &TeamBuilder{team: Team{
		ID:         id,
		CreateTime: time.Date(2021, 1, 2, 3, 4, 5, 6, time.UTC),
	}}
}

func (b *TeamBuilder) WithID(id string) *TeamBuilder {
	b.team.ID = id
	return b
}

func (b *TeamBuilder) WithCreateTime(createTime time.Time) *TeamBuilder {
	b.team.CreateTime = createTime
	return b
}

func (b *TeamBuilder) Build() *Team {
	t := b.team
	return &t
}

func (b *TeamBuilder) CreateInDB(ctx context.Context) (*Team, error) {
	t := b.Build()
	row := map[string]any{
		"Id":         t.ID,
		"CreateTime": t.CreateTime,
	}
	m := spanner.InsertOrUpdateMap("Teams", row)
	ts, err := span.Apply(ctx, []*spanner.Mutation{m})
	if err != nil {
		return nil, err
	}
	if t.CreateTime == spanner.CommitTimestamp {
		t.CreateTime = ts.UTC()
	}
	return t, nil
}
