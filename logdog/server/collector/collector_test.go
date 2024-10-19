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

package collector

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/pubsubprotocol"
	"go.chromium.org/luci/logdog/common/storage/memory"
	"go.chromium.org/luci/logdog/common/types"
	cc "go.chromium.org/luci/logdog/server/collector/coordinator"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

// TestCollector runs through a series of end-to-end Collector workflows and
// ensures that the Collector behaves appropriately.
func testCollectorImpl(t *testing.T, caching bool) {
	ftt.Run(fmt.Sprintf(`Using a test configuration with caching == %v`, caching), t, func(t *ftt.Test) {
		c, _ := testclock.UseTime(context.Background(), testclock.TestTimeLocal)

		tcc := &testCoordinator{}
		st := &testStorage{Storage: &memory.Storage{}}

		coll := &Collector{
			Coordinator: tcc,
			Storage:     st,
		}
		defer coll.Close()

		bb := bundleBuilder{
			Context: c,
		}

		if caching {
			coll.Coordinator = cc.NewCache(coll.Coordinator, 0, 0)
		}

		t.Run(`Can process multiple single full streams from a Butler bundle.`, func(t *ftt.Test) {
			bb.addFullStream("foo/+/bar", 128)
			bb.addFullStream("foo/+/baz", 256)

			assert.Loosely(t, coll.Process(c, bb.bundle()), should.BeNil)

			shouldHaveRegisteredStream(t, tcc, "test-project", "foo/+/bar", 127)
			shouldHaveStoredStream(t, st, "test-project", "foo/+/bar", indexRange{0, 127})

			shouldHaveRegisteredStream(t, tcc, "test-project", "foo/+/baz", 255)
			shouldHaveStoredStream(t, st, "test-project", "foo/+/baz", indexRange{0, 255})
		})

		t.Run(`Will return a transient error if a transient error happened while registering.`, func(t *ftt.Test) {
			tcc.registerCallback = func(cc.LogStreamState) error { return errors.New("test error", transient.Tag) }

			bb.addFullStream("foo/+/bar", 128)
			err := coll.Process(c, bb.bundle())
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
		})

		t.Run(`Will return an error if a non-transient error happened while registering.`, func(t *ftt.Test) {
			tcc.registerCallback = func(cc.LogStreamState) error { return errors.New("test error") }

			bb.addFullStream("foo/+/bar", 128)
			err := coll.Process(c, bb.bundle())
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
		})

		// This will happen when one registration request registers non-terminal,
		// and a follow-on registration request registers with a terminal index. The
		// latter registration request will idempotently succeed, but not accept the
		// terminal index, so termination is still required.
		t.Run(`Will terminate a stream if a terminal registration returns a non-terminal response.`, func(t *ftt.Test) {
			terminateCalled := false
			tcc.terminateCallback = func(cc.TerminateRequest) error {
				terminateCalled = true
				return nil
			}

			bb.addStreamEntries("foo/+/bar", -1, 0, 1)
			assert.Loosely(t, coll.Process(c, bb.bundle()), should.BeNil)

			bb.addStreamEntries("foo/+/bar", 3, 2, 3)
			assert.Loosely(t, coll.Process(c, bb.bundle()), should.BeNil)
			assert.Loosely(t, terminateCalled, should.BeTrue)
		})

		t.Run(`Will return a transient error if a transient error happened while terminating.`, func(t *ftt.Test) {
			tcc.terminateCallback = func(cc.TerminateRequest) error { return errors.New("test error", transient.Tag) }

			// Register independently from terminate so we don't bundle RPC.
			bb.addStreamEntries("foo/+/bar", -1, 0, 1, 2, 3, 4)
			assert.Loosely(t, coll.Process(c, bb.bundle()), should.BeNil)

			// Add terminal index.
			bb.addStreamEntries("foo/+/bar", 5, 5)
			err := coll.Process(c, bb.bundle())
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
		})

		t.Run(`Will return an error if a non-transient error happened while terminating.`, func(t *ftt.Test) {
			tcc.terminateCallback = func(cc.TerminateRequest) error { return errors.New("test error") }

			// Register independently from terminate so we don't bundle RPC.
			bb.addStreamEntries("foo/+/bar", -1, 0, 1, 2, 3, 4)
			assert.Loosely(t, coll.Process(c, bb.bundle()), should.BeNil)

			// Add terminal index.
			bb.addStreamEntries("foo/+/bar", 5, 5)
			err := coll.Process(c, bb.bundle())
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
		})

		t.Run(`Will return a transient error if a transient error happened on storage.`, func(t *ftt.Test) {
			// Single transient error.
			count := int32(0)
			st.err = func() error {
				if atomic.AddInt32(&count, 1) == 1 {
					return errors.New("test error", transient.Tag)
				}
				return nil
			}

			bb.addFullStream("foo/+/bar", 128)
			err := coll.Process(c, bb.bundle())
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
		})

		t.Run(`Will drop invalid LogStreamDescriptor bundle entries and process the valid ones.`, func(t *ftt.Test) {
			be := bb.genBundleEntry("foo/+/trash", 1337, 4, 5, 6, 7, 8)
			bb.addBundleEntry(be)

			bb.addStreamEntries("foo/+/trash", 0, 1, 3) // Invalid: non-contiguous
			bb.addFullStream("foo/+/bar", 32)

			err := coll.Process(c, bb.bundle())
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)

			shouldHaveRegisteredStream(t, tcc, "test-project", "foo/+/bar", 32)
			shouldHaveStoredStream(t, st, "test-project", "foo/+/bar", indexRange{0, 31})

			shouldHaveRegisteredStream(t, tcc, "test-project", "foo/+/trash", 1337)
			shouldHaveStoredStream(t, st, "test-project", "foo/+/trash", 4, 5, 6, 7, 8)
		})

		t.Run(`Will drop streams with missing (invalid) secrets.`, func(t *ftt.Test) {
			b := bb.genBase()
			b.Secret = nil
			bb.addFullStream("foo/+/bar", 4)

			err := coll.Process(c, bb.bundle())
			assert.Loosely(t, err, should.ErrLike("invalid prefix secret"))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
		})

		t.Run(`Will drop messages with mismatching secrets.`, func(t *ftt.Test) {
			bb.addStreamEntries("foo/+/bar", -1, 0, 1, 2)
			assert.Loosely(t, coll.Process(c, bb.bundle()), should.BeNil)

			// Push another bundle with a different secret.
			b := bb.genBase()
			b.Secret = bytes.Repeat([]byte{0xAA}, types.PrefixSecretLength)
			be := bb.genBundleEntry("foo/+/bar", 4, 3, 4)
			be.TerminalIndex = 1337
			bb.addBundleEntry(be)
			bb.addFullStream("foo/+/baz", 3)
			assert.Loosely(t, coll.Process(c, bb.bundle()), should.BeNil)

			shouldHaveRegisteredStream(t, tcc, "test-project", "foo/+/bar", -1)
			shouldHaveStoredStream(t, st, "test-project", "foo/+/bar", indexRange{0, 2})

			shouldHaveRegisteredStream(t, tcc, "test-project", "foo/+/baz", 2)
			shouldHaveStoredStream(t, st, "test-project", "foo/+/baz", indexRange{0, 2})
		})

		t.Run(`With an empty project name, will drop the stream.`, func(t *ftt.Test) {
			b := bb.genBase()
			b.Project = ""
			bb.addFullStream("foo/+/baz", 3)

			err := coll.Process(c, bb.bundle())
			assert.Loosely(t, err, should.ErrLike("invalid bundle project name"))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
		})

		t.Run(`Will drop streams with invalid project names.`, func(t *ftt.Test) {
			b := bb.genBase()
			b.Project = "!!!invalid name!!!"
			assert.Loosely(t, config.ValidateProjectName(b.Project), should.NotBeNil)

			err := coll.Process(c, bb.bundle())
			assert.Loosely(t, err, should.ErrLike("invalid bundle project name"))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
		})

		t.Run(`Will drop streams with empty bundle prefixes.`, func(t *ftt.Test) {
			b := bb.genBase()
			b.Prefix = ""

			err := coll.Process(c, bb.bundle())
			assert.Loosely(t, err, should.ErrLike("invalid bundle prefix"))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
		})

		t.Run(`Will drop streams with invalid bundle prefixes.`, func(t *ftt.Test) {
			b := bb.genBase()
			b.Prefix = "!!!invalid prefix!!!"
			assert.Loosely(t, types.StreamName(b.Prefix).Validate(), should.NotBeNil)

			err := coll.Process(c, bb.bundle())
			assert.Loosely(t, err, should.ErrLike("invalid bundle prefix"))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
		})

		t.Run(`Will drop streams whose descriptor prefix doesn't match its bundle's prefix.`, func(t *ftt.Test) {
			bb.addStreamEntries("baz/+/bar", 3, 0, 1, 2, 3, 4)

			err := coll.Process(c, bb.bundle())
			assert.Loosely(t, err, should.ErrLike("mismatched bundle and entry prefixes"))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
		})

		t.Run(`Will return no error if the data has a corrupt bundle header.`, func(t *ftt.Test) {
			assert.Loosely(t, coll.Process(c, []byte{0x00}), should.BeNil)
			shouldNotHaveRegisteredStream(t, tcc, "test-project", "foo/+/bar")
		})

		t.Run(`Will drop bundles with unknown ProtoVersion string.`, func(t *ftt.Test) {
			buf := bytes.Buffer{}
			w := pubsubprotocol.Writer{ProtoVersion: "!!!invalid!!!"}
			w.Write(&buf, &logpb.ButlerLogBundle{})

			assert.Loosely(t, coll.Process(c, buf.Bytes()), should.BeNil)

			shouldNotHaveRegisteredStream(t, tcc, "test-project", "foo/+/bar")
		})

		t.Run(`Will not ingest records if the stream is archived.`, func(t *ftt.Test) {
			tcc.register(cc.LogStreamState{
				Project:       "test-project",
				Path:          "foo/+/bar",
				Secret:        testSecret,
				TerminalIndex: -1,
				Archived:      true,
			})

			bb.addStreamEntries("foo/+/bar", 3, 0, 1, 2, 3, 4)
			assert.Loosely(t, coll.Process(c, bb.bundle()), should.BeNil)

			shouldHaveRegisteredStream(t, tcc, "test-project", "foo/+/bar", -1)
			shouldHaveStoredStream(t, st, "test-project", "foo/+/bar")
		})

		t.Run(`Will not ingest records if the stream is purged.`, func(t *ftt.Test) {
			tcc.register(cc.LogStreamState{
				Project:       "test-project",
				Path:          "foo/+/bar",
				Secret:        testSecret,
				TerminalIndex: -1,
				Purged:        true,
			})

			assert.Loosely(t, coll.Process(c, bb.bundle()), should.BeNil)

			shouldHaveRegisteredStream(t, tcc, "test-project", "foo/+/bar", -1)
			shouldHaveStoredStream(t, st, "test-project", "foo/+/bar")
		})

		t.Run(`Will not ingest a bundle with no bundle entries.`, func(t *ftt.Test) {
			assert.Loosely(t, coll.Process(c, bb.bundle()), should.BeNil)
		})

		t.Run(`Will not ingest a bundle whose log entries don't match their descriptor.`, func(t *ftt.Test) {
			be := bb.genBundleEntry("foo/+/bar", 4, 0, 1, 2, 3, 4)

			// Add a binary log entry. This does NOT match the text descriptor, and
			// should fail validation.
			be.Logs = append(be.Logs, &logpb.LogEntry{
				StreamIndex: 2,
				Sequence:    2,
				Content: &logpb.LogEntry_Binary{
					&logpb.Binary{
						Data: []byte{0xd0, 0x6f, 0x00, 0xd5},
					},
				},
			})
			bb.addBundleEntry(be)
			assert.Loosely(t, coll.Process(c, bb.bundle()), should.ErrLike("invalid log entry"))

			shouldNotHaveRegisteredStream(t, tcc, "test-project", "foo/+/bar")
		})
	})
}

// TestCollector runs through a series of end-to-end Collector workflows and
// ensures that the Collector behaves appropriately.
func TestCollector(t *testing.T) {
	t.Parallel()

	testCollectorImpl(t, false)
}

// TestCollectorWithCaching runs through a series of end-to-end Collector
// workflows and ensures that the Collector behaves appropriately.
func TestCollectorWithCaching(t *testing.T) {
	t.Parallel()

	testCollectorImpl(t, true)
}
