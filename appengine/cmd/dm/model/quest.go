// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/display"
	"github.com/luci/luci-go/common/clock"
	"golang.org/x/net/context"
)

// QuestDescriptor is the data associated with a Quest that's hashed into the
// Quest's ID.
type QuestDescriptor struct {
	Distributor string
	Payload     []byte
}

var (
	// QuestIDLength is the number of encoded bytes to use. It removes the
	// single padding character.
	QuestIDLength = base64.URLEncoding.EncodedLen(sha256.Size) - 1

	// distributorRE is the regex that Quest.Distributor must match.
	distributorRE = regexp.MustCompile("^[a-zA-Z][a-zA-Z0-9._-]*$")
)

const (
	// payloadMaxLength is the maximum size of the Quest Payload
	payloadMaxLength = 256 * 1024

	// distributorNameMaxLength is the maximum distributor Name length.
	distributorNameMaxLength = 64
)

func (desc *QuestDescriptor) compactPayload() error {
	if len(desc.Payload) > payloadMaxLength {
		return fmt.Errorf("quest payload is too large: %d > %d",
			len(desc.Payload), payloadMaxLength)
	}

	dec := json.NewDecoder(bytes.NewBuffer(desc.Payload))
	dec.UseNumber()
	decoded := map[string]interface{}{}
	err := dec.Decode(&decoded)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer(desc.Payload[:0])
	err = json.NewEncoder(buf).Encode(decoded)

	desc.Payload = buf.Bytes()[:buf.Len()-1]

	return err
}

func (desc *QuestDescriptor) validDistributor() error {
	if len(desc.Distributor) > distributorNameMaxLength {
		return fmt.Errorf("quest distributor name is too long: %d > %d",
			len(desc.Distributor), distributorNameMaxLength)
	}

	if !distributorRE.MatchString(desc.Distributor) {
		return fmt.Errorf("quest distributor name is invalid: %s", desc.Distributor)
	}
	return nil
}

// NewQuest builds a new Quest object with a correct ID given the current
// contents of the QuestDescriptor. It returns an error if the Payload or
// Distributor are invalid.
//
// This will also compactify the json Payload as a side effect.
func (desc *QuestDescriptor) NewQuest(c context.Context) (*Quest, error) {
	if err := desc.validDistributor(); err != nil {
		return nil, err
	}
	if err := desc.compactPayload(); err != nil {
		return nil, err
	}

	h := sha256.New()
	data := []struct {
		field string
		data  string
	}{
		{"distributor", desc.Distributor},
		{"payload", string(desc.Payload)},
	}

	for _, d := range data {
		fmt.Fprintf(h, "%s %d\x00%s", d.field, len(d.data), d.data)
	}

	return &Quest{
		ID:              base64.URLEncoding.EncodeToString(h.Sum(nil))[:QuestIDLength],
		QuestDescriptor: *desc,
		Created:         clock.Now(c).UTC(),
	}, nil
}

// Quest is the model for a job-to-run. Its QuestDescriptor should fully
// describe the job to be done.
type Quest struct {
	// ID is the base64 sha256 of QuestDescriptor
	ID string `gae:"$id"`

	QuestDescriptor `gae:",noindex"`

	Created time.Time `gae:",noindex"`
}

// GetAttempts returns all Attempt objects that exist for this Quest.
func (q *Quest) GetAttempts(c context.Context) (s []*Attempt, err error) {
	ds := datastore.Get(c)
	from := ds.MakeKey("Attempt", q.ID+"|")
	to := ds.MakeKey("Attempt", q.ID+"~")

	// TODO(iannucci): page this
	qry := datastore.NewQuery("Attempt").Gt("__key__", from).Lt("__key__", to)
	err = ds.GetAll(qry, &s)
	return
}

// ToDisplay converts this Quest into its display equivalent.
func (q *Quest) ToDisplay() *display.Quest {
	return &display.Quest{
		ID:          q.ID,
		Payload:     string(q.Payload),
		Distributor: q.Distributor,
		Created:     q.Created,
	}
}
