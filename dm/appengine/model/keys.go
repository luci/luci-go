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
	"context"
	"fmt"
	"math"

	ds "go.chromium.org/gae/service/datastore"
	dm "go.chromium.org/luci/dm/api/service/v1"
)

// QuestKeyFromID makes a datastore.Key given the QuestID.
func QuestKeyFromID(c context.Context, qid string) *ds.Key {
	return ds.MakeKey(c, "Quest", qid)
}

// QuestFromID produces an empty Quest model from the QuestID.
func QuestFromID(qid string) *Quest {
	return &Quest{ID: qid}
}

// QuestIDFromKey makes a QuestID from the given datastore.Key. It panics if the
// Key does not point to a Quest.
func QuestIDFromKey(k *ds.Key) string {
	if k.Kind() != "Quest" || k.Parent() != nil {
		panic(fmt.Errorf("invalid Quest key: %s", k))
	}
	return k.StringID()
}

// AttemptKeyFromID makes a datastore.Key given the AttemptID.
func AttemptKeyFromID(c context.Context, aid *dm.Attempt_ID) *ds.Key {
	return ds.MakeKey(c, "Attempt", aid.DMEncoded())
}

// AttemptFromID produces an empty Attempt model from the AttemptID.
func AttemptFromID(aid *dm.Attempt_ID) *Attempt {
	ret := &Attempt{}
	ret.ID = *aid
	return ret
}

// AttemptIDFromKey makes a AttemptID from the given datastore.Key. It panics if the
// Key does not point to a Attempt.
func AttemptIDFromKey(k *ds.Key) *dm.Attempt_ID {
	if k.Kind() != "Attempt" || k.Parent() != nil {
		panic(fmt.Errorf("invalid Attempt key: %s", k))
	}
	ret := &dm.Attempt_ID{}
	if err := ret.SetDMEncoded(k.StringID()); err != nil {
		panic(fmt.Errorf("invalid Attempt key: %s: %s", k, err))
	}
	return ret
}

// ExecutionKeyFromID makes a datastore.Key given the ExecutionID.
func ExecutionKeyFromID(c context.Context, eid *dm.Execution_ID) *ds.Key {
	return ds.MakeKey(c, "Attempt", eid.AttemptID().DMEncoded(), "Execution", eid.Id)
}

// ExecutionFromID produces an empty Execution model from the ExecutionID.
func ExecutionFromID(c context.Context, eid *dm.Execution_ID) *Execution {
	ret := &Execution{}
	ret.ID = invertedHexUint32(eid.Id)
	ret.Attempt = AttemptKeyFromID(c, eid.AttemptID())
	return ret
}

// ExecutionIDFromKey makes a ExecutionID from the given datastore.Key. It panics if the
// Key does not point to a Execution.
func ExecutionIDFromKey(k *ds.Key) *dm.Execution_ID {
	if k.Kind() != "Execution" || k.Parent() == nil {
		panic(fmt.Errorf("invalid Execution key: %s", k))
	}
	id := k.IntID()
	if id <= 0 || id > math.MaxUint32 {
		panic(fmt.Errorf("invalid Execution key: %s", k))
	}
	atmpt := AttemptIDFromKey(k.Parent())
	return &dm.Execution_ID{Quest: atmpt.Quest, Attempt: atmpt.Id, Id: uint32(id)}
}
