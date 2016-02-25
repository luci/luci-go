// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock"
	google_pb "github.com/luci/luci-go/common/proto/google"

	"github.com/luci/luci-go/common/api/dm/service/v1"
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

// NormalizeJSONObject is used to take some free-form JSON, validate that:
//   * its unnormalized form is <= maxLen
//   * it contains a valid JSON object (e.g. `{...stuff...}`)
//
// This function will re-use data as the destination buffer, and will return
// a new slice into the same memory (or nil).
func NormalizeJSONObject(maxLen int, data string) (string, error) {
	if len(data) > maxLen {
		return "", fmt.Errorf("quest payload is too large: %d > %d",
			len(data), maxLen)
	}

	dec := json.NewDecoder(bytes.NewBufferString(data))
	dec.UseNumber()
	decoded := map[string]interface{}{}
	err := dec.Decode(&decoded)
	if err != nil {
		return "", err
	}

	buf := &bytes.Buffer{}
	buf.Grow(len(data))
	err = json.NewEncoder(buf).Encode(decoded)

	// the -1 chops off an extraneous newline that the json lib adds on.
	return buf.String()[:buf.Len()-1], err
}

// NewQuest builds a new Quest object with a correct ID given the current
// contents of the Quest_Desc. It returns an error if the Desc is invalid.
//
// This will also compactify the inner json Desc as a side effect.
func NewQuest(c context.Context, desc *dm.Quest_Desc) (ret *Quest, err error) {
	desc.JsonPayload, err = NormalizeJSONObject(payloadMaxLength, desc.JsonPayload)
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

	Desc dm.Quest_Desc `gae:",noindex"`

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
	return &dm.Quest_Data{
		Created: google_pb.NewTimestamp(q.Created),
		Desc:    &q.Desc,
	}
}
