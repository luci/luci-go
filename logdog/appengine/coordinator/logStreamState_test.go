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

package coordinator

import (
	"bytes"
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	ds "go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/logdog/api/config/svcconfig"
	"go.chromium.org/luci/logdog/common/types"
)

func TestLogStreamState(t *testing.T) {
	t.Parallel()

	ftt.Run(`A testing log stream state`, t, func(t *ftt.Test) {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c = memory.Use(c)

		ds.GetTestable(c).Consistent(true)

		c = withProjectConfigs(c, map[string]*svcconfig.ProjectConfig{
			"proj-foo": {},
		})
		if err := WithProjectNamespace(&c, "proj-foo"); err != nil {
			panic(err)
		}

		now := ds.RoundTime(tc.Now().UTC())
		ls := LogStream{ID: LogStreamID("testing/+/log/stream")}
		lst := ls.State(c)

		lst.Schema = CurrentSchemaVersion
		lst.Created = now.UTC()
		lst.Updated = now.UTC()
		lst.ExpireAt = now.Add(LogStreamStateExpiry).UTC()
		lst.Secret = bytes.Repeat([]byte{0x55}, types.PrefixSecretLength)
		lst.TerminalIndex = -1

		t.Run(`Will validate`, func(t *ftt.Test) {
			assert.Loosely(t, lst.Validate(), should.BeNil)
		})

		t.Run(`Will not validate`, func(t *ftt.Test) {
			t.Run(`Without a valid created time`, func(t *ftt.Test) {
				lst.Created = time.Time{}
				assert.Loosely(t, lst.Validate(), should.ErrLike("missing created time"))
			})
			t.Run(`Without a valid updated time`, func(t *ftt.Test) {
				lst.Updated = time.Time{}
				assert.Loosely(t, lst.Validate(), should.ErrLike("missing updated time"))
			})
			t.Run(`Without a valid stream secret`, func(t *ftt.Test) {
				lst.Secret = nil
				assert.Loosely(t, lst.Validate(), should.ErrLike("invalid prefix secret length"))
			})
			t.Run(`With a terminal index, will not validate without a TerminatedTime.`, func(t *ftt.Test) {
				lst.ArchivalKey = []byte{0xd0, 0x65}
				lst.TerminalIndex = 1337
				assert.Loosely(t, lst.Validate(), should.ErrLike("missing terminated time"))

				lst.TerminatedTime = now
				assert.Loosely(t, lst.Validate(), should.BeNil)
			})
		})

		t.Run(`Can write to the Datastore.`, func(t *ftt.Test) {
			assert.Loosely(t, ds.Put(c, lst), should.BeNil)

			t.Run(`Can be queried`, func(t *ftt.Test) {
				q := ds.NewQuery("LogStreamState")

				// runQuery will run the query, panic if it fails, and return whether the
				// single record, "lst", was returned.
				runQuery := func(q *ds.Query) bool {
					var states []*LogStreamState
					if err := ds.GetAll(c, q, &states); err != nil {
						panic(err)
					}
					return len(states) > 0
				}

				t.Run(`A non-terminated stream will satisfy !Terminated, !Archived`, func(t *ftt.Test) {
					assert.Loosely(t, runQuery(q.Eq("_Terminated", false)), should.BeTrue)
					assert.Loosely(t, runQuery(q.Eq("_ArchivalState", NotArchived)), should.BeTrue)
				})

				t.Run(`A terminated stream will satisfy Terminated, but !Archived`, func(t *ftt.Test) {
					lst.TerminalIndex = 1337
					lst.TerminatedTime = now
					assert.Loosely(t, ds.Put(c, lst), should.BeNil)

					assert.Loosely(t, runQuery(q.Eq("_Terminated", true)), should.BeTrue)
					assert.Loosely(t, runQuery(q.Eq("_ArchivalState", NotArchived)), should.BeTrue)
				})

				t.Run(`An incomplete archived stream will satisfy Terminated, and ArchivedPartial`, func(t *ftt.Test) {
					lst.TerminalIndex = 10
					lst.ArchiveLogEntryCount = 9
					lst.ArchivedTime = now
					assert.Loosely(t, ds.Put(c, lst), should.BeNil)

					assert.Loosely(t, runQuery(q.Eq("_Terminated", true)), should.BeTrue)
					assert.Loosely(t, runQuery(q.Eq("_ArchivalState", ArchivedPartial)), should.BeTrue)
				})

				t.Run(`A complete archived stream will satisfy Terminated, and ArchivedComplete`, func(t *ftt.Test) {
					lst.TerminalIndex = 10
					lst.ArchiveLogEntryCount = 11
					lst.ArchivedTime = now
					assert.Loosely(t, ds.Put(c, lst), should.BeNil)

					assert.Loosely(t, runQuery(q.Eq("_Terminated", true)), should.BeTrue)
					assert.Loosely(t, runQuery(q.Eq("_ArchivalState", ArchivedComplete)), should.BeTrue)
				})
			})
		})
	})
}
