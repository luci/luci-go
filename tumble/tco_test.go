// Copyright 2015 The LUCI Authors.
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
	"testing"

	ds "go.chromium.org/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
)

type BigObjectGroup struct {
	_id int64 `gae:"$id,1"`

	Count int64
}

type BigObject struct {
	ID    int64   `gae:"$id"`
	Group *ds.Key `gae:"$parent"`

	Data []byte `gae:",noindex"`
}

type SlowMutation struct {
	Count int64
}

func (s *SlowMutation) Root(c context.Context) *ds.Key {
	return ds.MakeKey(c, "BigObjectGroup", 1)
}

var JunkBytes = make([]byte, 512*1024)

func init() {
	for i := 0; i < 32768; i += 16 {
		copy(JunkBytes[i:i+16], []byte("BuhNerfCatBlarty"))
	}
}

func (s *SlowMutation) RollForward(c context.Context) (muts []Mutation, err error) {
	grp := &BigObjectGroup{}
	err = ds.Get(c, grp)
	if err == ds.ErrNoSuchEntity {
		err = ds.Put(c, grp)
	}
	if err != nil {
		return
	}

	grp.Count++
	bo := &BigObject{ID: grp.Count, Group: ds.KeyForObj(c, grp), Data: JunkBytes}
	err = ds.Put(c, grp, bo)

	retMut := *s
	retMut.Count--
	if retMut.Count > 0 {
		muts = append(muts, &retMut)
	}
	return
}

func init() {
	Register((*SlowMutation)(nil))
}

func TestTailCallOptimization(t *testing.T) {
	if testing.Short() {
		t.Skip("TCO test skipped in -short mode")
	}

	t.Parallel()

	Convey("Tail Call optimization test", t, func() {
		testing := &Testing{}
		c := testing.Context()

		// This will start a chain.
		So(RunMutation(c, &SlowMutation{40}), ShouldBeNil)

		// 2 processed shards shows that we did 1 transaction and 1 empty tail call.
		So(testing.Drain(c), ShouldEqual, 2)

		bog := &BigObjectGroup{}
		So(ds.Get(c, bog), ShouldBeNil)

		// Hey look at that, bog shows all 40 :)
		So(bog.Count, ShouldEqual, 40)
	})
}
