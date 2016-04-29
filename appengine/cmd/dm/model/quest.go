// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock"
	google_pb "github.com/luci/luci-go/common/proto/google"

	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/api/template"
)

var (
	// QuestIDLength is the number of encoded bytes to use. It removes the
	// single padding character.
	QuestIDLength = base64.URLEncoding.EncodedLen(sha256.Size) - 1
)

const (
	// payloadMaxLength is the maximum size of the Quest Desc
	payloadMaxLength = 256 * 1024
)

// NewQuest builds a new Quest object with a correct ID given the current
// contents of the Quest_Desc. It returns an error if the Desc is invalid.
//
// This will also compactify the inner json Desc as a side effect.
func NewQuest(c context.Context, desc *dm.Quest_Desc) (ret *Quest, err error) {
	if len(desc.JsonPayload) > payloadMaxLength {
		return nil, fmt.Errorf("quest payload is too large: %d > %d",
			len(desc.JsonPayload), payloadMaxLength)
	}
	desc.JsonPayload, err = template.NormalizeJSON(desc.JsonPayload, true)
	if err != nil {
		return
	}

	data, err := proto.Marshal(desc)
	if err != nil {
		panic(err)
	}
	h := sha256.Sum256(data)

	ret = &Quest{
		ID:      base64.URLEncoding.EncodeToString(h[:])[:QuestIDLength],
		Desc:    *desc,
		Created: clock.Now(c).UTC(),
	}
	return
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
func QueryAttemptsForQuest(c context.Context, qid string) *datastore.Query {
	ds := datastore.Get(c)
	from := ds.MakeKey("Attempt", qid+"|")
	to := ds.MakeKey("Attempt", qid+"~")
	return datastore.NewQuery("Attempt").Gt("__key__", from).Lt("__key__", to)
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
