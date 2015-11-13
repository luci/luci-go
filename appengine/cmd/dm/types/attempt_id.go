// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package types

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/luci/gae/service/datastore"
)

// AttemptID is an embeddable structure which represents a parsed version
// of the attempt datastore Key ID. Attempts have an id of the form:
//
//   QUEST_ID|ATTEMPT_NUM
//
// Where QUEST_ID is the raw quest Key ID that this Attempt is for, and
// ATTEMPT_NUM is an inverted 0-padded hex-encoded 32bit unsigned integer. It's
// inverted so that the default __key__ sort-order for Attempts is from
// high-to-low. The lowest-numbered attempt therefore has the suffix
// "|FFFFFFFF".
type AttemptID struct {
	QuestID    string `gae:"-"`
	AttemptNum uint32 `gae:"-"`
}

var _ datastore.PropertyConverter = (*AttemptID)(nil)

// ToProperty implements datastore.PropertyConverter
func (a *AttemptID) ToProperty() (datastore.Property, error) {
	return datastore.MkPropertyNI(a.ID()), nil
}

// FromProperty implements datastore.PropertyConverter
func (a *AttemptID) FromProperty(p datastore.Property) error {
	if p.Type() != datastore.PTString {
		return fmt.Errorf("wrong type for property: %s", p.Type())
	}
	return a.SetEncoded(p.Value().(string))
}

// ID returns an encoded string id for this Attempt.
func (a *AttemptID) ID() string {
	return fmt.Sprintf("%s|%08x", a.QuestID, 0xFFFFFFFF^a.AttemptNum)
}

// Less returnst true iff `a < o`
func (a *AttemptID) Less(o *AttemptID) bool {
	return (a.QuestID < o.QuestID ||
		(a.QuestID == o.QuestID && a.AttemptNum < o.AttemptNum))
}

// SetEncoded decodes val into this AttemptID, returning an error if there's a
// problem.
func (a *AttemptID) SetEncoded(val string) error {
	toks := strings.SplitN(val, "|", 2)
	if len(toks) != 2 {
		return fmt.Errorf("unable to parse Attempt id: %q", val)
	}
	a.QuestID = toks[0]
	an, err := strconv.ParseUint(toks[1], 16, 32)
	if err != nil {
		return err
	}
	a.AttemptNum = 0xFFFFFFFF ^ uint32(an)
	return nil
}

// NewAttemptID parses an encoded AttemptID string into an AttemptID object. It
// panics if `encoded` is malformed. See the docstring on AttemptID for details
// on the format.
func NewAttemptID(encoded string) *AttemptID {
	ret := &AttemptID{}
	if err := ret.SetEncoded(encoded); err != nil {
		panic(err)
	}
	return ret
}

// AttemptIDSlice is a sortable slice of AttemptID objects. Note that its
// default sort order is "natural" (low to high, according to AttemptID.Less),
// which is partially opposite from the sort order based on string-encoding
// the AttemptIDs (e.g. __key__ order). This is because AttemptID intentionally
// stores its AttemptNum in reverse order.
type AttemptIDSlice []*AttemptID

func (s AttemptIDSlice) Len() int           { return len(s) }
func (s AttemptIDSlice) Less(i, j int) bool { return s[i].Less(s[j]) }
func (s AttemptIDSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
