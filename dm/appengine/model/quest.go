// Copyright 2016 The LUCI Authors.
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
	"time"

	"golang.org/x/net/context"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock"
	google_pb "github.com/luci/luci-go/common/proto/google"

	dm "github.com/luci/luci-go/dm/api/service/v1"
)

// NewQuest builds a new Quest object with a correct ID given the current
// contents of the Quest_Desc. It returns an error if the Desc is invalid.
//
// Desc must already be Normalize()'d
func NewQuest(c context.Context, desc *dm.Quest_Desc) *Quest {
	return &Quest{ID: desc.QuestID(), Desc: *desc, Created: clock.Now(c).UTC()}
}

// Quest is the model for a job-to-run. Its questPayload should fully
// describe the job to be done.
type Quest struct {
	// ID is the base64 sha256 of questPayload
	ID string `gae:"$id"`

	Desc    dm.Quest_Desc `gae:",noindex"`
	BuiltBy TemplateInfo  `gae:",noindex"`

	Created time.Time `gae:",noindex"`
}

// QueryAttemptsForQuest returns all Attempt objects that exist for this Quest.
func QueryAttemptsForQuest(c context.Context, qid string) *ds.Query {
	from := ds.MakeKey(c, "Attempt", qid+"|")
	to := ds.MakeKey(c, "Attempt", qid+"~")
	return ds.NewQuery("Attempt").Gt("__key__", from).Lt("__key__", to)
}

// ToProto converts this Quest into its display equivalent.
func (q *Quest) ToProto() *dm.Quest {
	return &dm.Quest{
		Id:   dm.NewQuestID(q.ID),
		Data: q.DataProto(),
	}
}

// DataProto gets the Quest.Data proto message for this Quest.
func (q *Quest) DataProto() *dm.Quest_Data {
	spec := make([]*dm.Quest_TemplateSpec, len(q.BuiltBy))
	for i := range q.BuiltBy {
		spec[i] = &q.BuiltBy[i]
	}

	return &dm.Quest_Data{
		Created: google_pb.NewTimestamp(q.Created),
		Desc:    &q.Desc,
		BuiltBy: spec,
	}
}
