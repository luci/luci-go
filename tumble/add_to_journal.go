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

package tumble

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/datastore/serialize"
	"go.chromium.org/luci/common/logging"
)

// AddToJournal records one or more Mutation to the tumble journal, but does not
// execute any of them. This does so by running a transaction on a pseudo-random
// entity group, and journaling the mutations there.
func AddToJournal(c context.Context, m ...Mutation) error {
	(logging.Fields{"count": len(m)}).Infof(c, "tumble.AddToJournal")
	if len(m) == 0 {
		return nil
	}
	return RunMutation(c, addToJournalMutation(m))
}

type addToJournalMutation []Mutation

func (a addToJournalMutation) Root(c context.Context) *ds.Key {
	hsh := sha256.New()
	for _, m := range a {
		hsh.Write(serialize.ToBytesWithContext(m.Root(c)))
	}
	return ds.MakeKey(c, "tumble.temp", hex.EncodeToString(hsh.Sum(nil)))
}

func (a addToJournalMutation) RollForward(c context.Context) ([]Mutation, error) {
	return a, nil
}
