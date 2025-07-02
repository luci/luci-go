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

package archive

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"strconv"
	"testing"
	"time"

	cl "cloud.google.com/go/logging"
	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/recordio"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/api/logpb"
)

func gen(i int) *logpb.LogEntry {
	return &logpb.LogEntry{
		TimeOffset:  durationpb.New(time.Duration(i) * time.Second),
		PrefixIndex: uint64(i) * 2,
		StreamIndex: uint64(i),
		Sequence:    uint64(i),
		Content: &logpb.LogEntry_Text{
			Text: &logpb.Text{
				Lines: []*logpb.Text_Line{
					{Value: []byte(strconv.Itoa(i)), Delimiter: "\n"},
				},
			},
		},
	}
}

type testSource struct {
	logs []*logpb.LogEntry
	err  error
}

func (s *testSource) addLogEntry(le *logpb.LogEntry) {
	s.logs = append(s.logs, le)
}

func (s *testSource) add(indices ...int) {
	for _, i := range indices {
		s.addLogEntry(gen(i))
	}
}

func (s *testSource) addEntries(entries ...*logpb.LogEntry) {
	s.logs = append(s.logs, entries...)
}

func (s *testSource) NextLogEntry() (le *logpb.LogEntry, err error) {
	if err = s.err; err != nil {
		return
	}
	if len(s.logs) == 0 {
		err = io.EOF
		return
	}

	le, s.logs = s.logs[0], s.logs[1:]
	return
}

type errWriter struct {
	io.Writer
	err error
}

func (w *errWriter) Write(d []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	return w.Writer.Write(d)
}

// testCLLogger implements CLLogger for testing and instrumentation.
type testCLLogger struct {
	entry []*cl.Entry
}

func (l *testCLLogger) Log(e cl.Entry) {
	l.entry = append(l.entry, &e)
}

// indexParams umarshals an index from a byte stream and removes any entries.
func indexParams(d []byte) *logpb.LogIndex {
	var index logpb.LogIndex
	if err := proto.Unmarshal(d, &index); err != nil {
		panic(fmt.Errorf("failed to unmarshal index protobuf: %v", err))
	}
	index.Entries = nil
	return &index
}

type indexChecker struct {
	fixedSize int
}

func (ic *indexChecker) size(pb proto.Message) int {
	if ic.fixedSize > 0 {
		return ic.fixedSize
	}
	return proto.Size(pb)
}

// shouldContainIndexFor validates the correctness and completeness of the
// supplied index.
//
// actual should be a *bytes.Buffer that contains a serialized index protobuf.
//
// expected[0] should be the log stream descriptor.
// expected[1] should be a *bytes.Buffer that contains the log RecordIO stream.
//
// If additional expected elements are supplied, they are the specific integers
// that should appear in the index. Otherwise, it is assumed that the log is a
// complete index.
func (ic *indexChecker) shouldContainIndexFor(t testing.TB, indexB *bytes.Buffer, desc *logpb.LogStreamDescriptor, logB *bytes.Buffer, expectedIdx ...uint64) {
	t.Helper()
	// Load our log index.
	index := logpb.LogIndex{}
	err := proto.Unmarshal(indexB.Bytes(), &index)
	assert.That(t, err, should.ErrLike(nil), truth.LineContext())

	// Descriptors must match.
	assert.That(t, index.Desc, should.Match(desc), truth.LineContext())

	// Catalog the log entries in "expected".
	entries := map[uint64]*logpb.LogEntry{}
	offsets := map[uint64]uint64{}
	csizes := map[uint64]uint64{}
	var eidx []uint64

	// Skip the first frame (descriptor).
	cr := iotools.CountingReader{
		Reader: logB,
	}
	csize := uint64(0)
	r := recordio.NewReader(&cr, 1024*1024)
	d, err := r.ReadFrameAll()
	assert.That(t, err, should.ErrLike(nil), truth.LineContext())

	for {
		offset := cr.Count
		d, err = r.ReadFrameAll()
		if err == io.EOF {
			break
		}
		assert.That(t, err, should.ErrLike(nil), truth.LineContext(), truth.Explain("entry #%d", len(entries)))

		le := logpb.LogEntry{}
		err := proto.Unmarshal(d, &le)
		assert.That(t, err, should.ErrLike(nil), truth.LineContext(), truth.Explain("entry #%d", len(entries)))

		entries[le.StreamIndex] = &le
		offsets[le.StreamIndex] = uint64(offset)
		csizes[le.StreamIndex] = csize

		csize += uint64(ic.size(&le))
		eidx = append(eidx, le.StreamIndex)
	}

	// Determine our expected archive indexes.
	if len(expectedIdx) > 0 {
		eidx = expectedIdx
	}

	iidx := make([]uint64, len(index.Entries))
	for i, e := range index.Entries {
		iidx[i] = e.StreamIndex
	}
	assert.That(t, iidx, should.Match(eidx), truth.LineContext())

	for i, cur := range index.Entries {
		cidx := eidx[i]
		le, offset := entries[uint64(cidx)], offsets[cidx]
		assert.Loosely(t, le, should.NotBeNil, truth.LineContext(), truth.Explain("stream index #%d", cidx))

		assert.That(t, cur.StreamIndex, should.Equal(le.StreamIndex), truth.LineContext(), truth.Explain("index entry #%d", i))
		assert.That(t, cur.Offset, should.Equal(offset), truth.LineContext(), truth.Explain("index entry #%d", i))
		assert.That(t, cur.PrefixIndex, should.Equal(le.PrefixIndex), truth.LineContext(), truth.Explain("index entry #%d", i))
		assert.That(t, cur.TimeOffset, should.Match(le.TimeOffset), truth.LineContext(), truth.Explain("index entry #%d", i))
		assert.That(t, cur.Sequence, should.Match(le.Sequence), truth.LineContext(), truth.Explain("index entry #%d", i))
	}
}

func TestArchive(t *testing.T) {
	ftt.Run(`A Manifest connected to Buffer Writers`, t, func(t *ftt.Test) {
		var logB, indexB bytes.Buffer
		desc := &logpb.LogStreamDescriptor{
			Prefix:    "test",
			Name:      "foo",
			Timestamp: timestamppb.New(testclock.TestTimeUTC),
		}
		ic := indexChecker{}
		ts := testSource{}
		m := Manifest{
			LUCIProject: "chromium",
			Desc:        desc,
			Source:      &ts,
			LogWriter:   &logB,
			IndexWriter: &indexB,
			CloudLogger: &testCLLogger{},
		}

		t.Run(`A sequence of logs will build a complete index.`, func(t *ftt.Test) {
			ts.add(0, 1, 2, 3, 4, 5, 6)
			_, err := Archive(m)
			assert.Loosely(t, err, should.BeNil)

			ic.shouldContainIndexFor(t, &indexB, desc, &logB)
			assert.Loosely(t, indexParams(indexB.Bytes()), should.Match(&logpb.LogIndex{
				Desc:            desc,
				LastPrefixIndex: 12,
				LastStreamIndex: 6,
				LogEntryCount:   7,
			}))
		})

		t.Run(`A sequence of non-contiguous logs will build a complete index.`, func(t *ftt.Test) {
			ts.add(0, 1, 3, 6)
			_, err := Archive(m)
			assert.Loosely(t, err, should.BeNil)

			ic.shouldContainIndexFor(t, &indexB, desc, &logB, 0, 1, 3, 6)
			assert.Loosely(t, indexParams(indexB.Bytes()), should.Match(&logpb.LogIndex{
				Desc:            desc,
				LastPrefixIndex: 12,
				LastStreamIndex: 6,
				LogEntryCount:   4,
			}))
		})

		t.Run(`Out of order logs are ignored`, func(t *ftt.Test) {
			t.Run(`When StreamIndex is out of order.`, func(t *ftt.Test) {
				ts.add(0, 2, 1, 3)
				_, err := Archive(m)
				assert.Loosely(t, err, should.BeNil)

				ic.shouldContainIndexFor(t, &indexB, desc, &logB, 0, 2, 3)
				assert.Loosely(t, indexParams(indexB.Bytes()), should.Match(&logpb.LogIndex{
					Desc:            desc,
					LastPrefixIndex: 6,
					LastStreamIndex: 3,
					LogEntryCount:   3,
				}))
			})

			t.Run(`When PrefixIndex is out of order.`, func(t *ftt.Test) {
				ts.add(0, 1)
				le := gen(2)
				le.PrefixIndex = 1
				ts.addEntries(le)
				ts.add(3, 4)
				_, err := Archive(m)
				assert.Loosely(t, err, should.BeNil)

				ic.shouldContainIndexFor(t, &indexB, desc, &logB, 0, 1, 3, 4)
				assert.Loosely(t, indexParams(indexB.Bytes()), should.Match(&logpb.LogIndex{
					Desc:            desc,
					LastPrefixIndex: 8,
					LastStreamIndex: 4,
					LogEntryCount:   4,
				}))
			})

			t.Run(`When Sequence is out of order.`, func(t *ftt.Test) {
				ts.add(0, 1)
				le := gen(2)
				le.Sequence = 0
				ts.addEntries(le)
				ts.add(3, 4)
				_, err := Archive(m)
				assert.Loosely(t, err, should.BeNil)

				ic.shouldContainIndexFor(t, &indexB, desc, &logB, 0, 1, 3, 4)
				assert.Loosely(t, indexParams(indexB.Bytes()), should.Match(&logpb.LogIndex{
					Desc:            desc,
					LastPrefixIndex: 8,
					LastStreamIndex: 4,
					LogEntryCount:   4,
				}))
			})
		})

		t.Run(`When TimeOffset is out of order, it is bumped to max.`, func(t *ftt.Test) {
			ts.add(0, 1)
			le := gen(2)
			le.TimeOffset = nil // 0
			ts.addEntries(le)
			ts.add(3, 4)
			_, err := Archive(m)
			assert.Loosely(t, err, should.BeNil)

			ic.shouldContainIndexFor(t, &indexB, desc, &logB, 0, 1, 2, 3, 4)
			assert.Loosely(t, indexParams(indexB.Bytes()), should.Match(&logpb.LogIndex{
				Desc:            desc,
				LastPrefixIndex: 8,
				LastStreamIndex: 4,
				LogEntryCount:   5,
			}))
		})

		t.Run(`Source errors will be returned`, func(t *ftt.Test) {
			t.Run(`nil LogEntry`, func(t *ftt.Test) {
				ts.addLogEntry(nil)
				_, err := Archive(m)
				assert.Loosely(t, err, should.ErrLike("nil LogEntry"))
			})

			t.Run(`Error returned`, func(t *ftt.Test) {
				ts.add(0, 1, 2, 3, 4, 5)
				ts.err = errors.New("test error")
				_, err := Archive(m)
				assert.Loosely(t, err, should.ErrLike("test error"))
			})
		})

		t.Run(`Writer errors will be returned`, func(t *ftt.Test) {
			ts.add(0, 1, 2, 3, 4, 5)

			t.Run(`For log writer errors.`, func(t *ftt.Test) {
				m.LogWriter = &errWriter{m.LogWriter, errors.New("test error")}
				_, err := Archive(m)
				assert.Loosely(t, errors.SingleError(err), should.ErrLike("test error"))
			})

			t.Run(`For index writer errors.`, func(t *ftt.Test) {
				m.IndexWriter = &errWriter{m.IndexWriter, errors.New("test error")}
				_, err := Archive(m)
				assert.Loosely(t, errors.SingleError(err), should.ErrLike("test error"))
			})

			t.Run(`When all Writers fail.`, func(t *ftt.Test) {
				m.LogWriter = &errWriter{m.LogWriter, errors.New("test error")}
				m.IndexWriter = &errWriter{m.IndexWriter, errors.New("test error")}
				_, err := Archive(m)
				assert.Loosely(t, err, should.NotBeNil)
			})
		})

		t.Run(`When building sparse index`, func(t *ftt.Test) {
			ts.add(0, 1, 2, 3, 4, 5)

			t.Run(`Can build an index for every 3 StreamIndex.`, func(t *ftt.Test) {
				m.StreamIndexRange = 3
				_, err := Archive(m)
				assert.Loosely(t, err, should.BeNil)

				ic.shouldContainIndexFor(t, &indexB, desc, &logB, 0, 3, 5)
				assert.Loosely(t, indexParams(indexB.Bytes()), should.Match(&logpb.LogIndex{
					Desc:            desc,
					LastPrefixIndex: 10,
					LastStreamIndex: 5,
					LogEntryCount:   6,
				}))
			})

			t.Run(`Can build an index for every 3 PrefixIndex.`, func(t *ftt.Test) {
				m.PrefixIndexRange = 3
				_, err := Archive(m)
				assert.Loosely(t, err, should.BeNil)

				// Note that in our generated logs, PrefixIndex = 2*StreamIndex.
				ic.shouldContainIndexFor(t, &indexB, desc, &logB, 0, 2, 4, 5)
				assert.Loosely(t, indexParams(indexB.Bytes()), should.Match(&logpb.LogIndex{
					Desc:            desc,
					LastPrefixIndex: 10,
					LastStreamIndex: 5,
					LogEntryCount:   6,
				}))
			})

			t.Run(`Can build an index for every 13 bytes.`, func(t *ftt.Test) {
				ic.fixedSize = 5
				m.ByteRange = 13
				m.sizeFunc = func(pb proto.Message) int {
					// Stub all LogEntry to be 5 bytes.
					return 5
				}
				_, err := Archive(m)
				assert.Loosely(t, err, should.BeNil)

				ic.shouldContainIndexFor(t, &indexB, desc, &logB, 0, 2, 5)
				assert.Loosely(t, indexParams(indexB.Bytes()), should.Match(&logpb.LogIndex{
					Desc:            desc,
					LastPrefixIndex: 10,
					LastStreamIndex: 5,
					LogEntryCount:   6,
				}))
			})
		})

		t.Run(`Exports entries to Cloud Logs`, func(t *ftt.Test) {
			clogger := m.CloudLogger.(*testCLLogger)
			line := func(msg, del string) *logpb.Text_Line {
				return &logpb.Text_Line{Value: []byte(msg), Delimiter: del}
			}
			sha := sha256.New()
			sha.Write([]byte(m.LUCIProject))
			sha.Write([]byte(desc.Prefix))
			sha.Write([]byte(desc.Name))
			streamIDHash := sha.Sum(nil)

			t.Run(`With nil LogEntry`, func(t *ftt.Test) {
				ts.addLogEntry(nil)
				_, err := Archive(m)
				assert.Loosely(t, err, should.ErrLike("nil LogEntry"))
				assert.Loosely(t, clogger.entry, should.HaveLength(0))
			})

			t.Run(`With multiple LogEntry(s)`, func(t *ftt.Test) {
				ts.add(123, 456, 789)
				ts.logs[0].GetText().Lines = []*logpb.Text_Line{
					line("this", "\n"),
					line("is", "\n"),
				}
				ts.logs[1].GetText().Lines = []*logpb.Text_Line{line("a complete", "\n")}
				ts.logs[2].GetText().Lines = []*logpb.Text_Line{
					line("line.", "\n"),
				}

				_, err := Archive(m)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, clogger.entry, should.HaveLength(3))
				assert.Loosely(t, clogger.entry[0].Payload, should.Equal("this\nis"))
				assert.Loosely(t, clogger.entry[1].Payload, should.Equal("a complete"))
				assert.Loosely(t, clogger.entry[2].Payload, should.Equal("line."))

				assert.Loosely(t, clogger.entry[0].Trace, should.Equal(fmt.Sprintf("%x", streamIDHash)))
				assert.Loosely(t, clogger.entry[1].Trace, should.Equal(fmt.Sprintf("%x", streamIDHash)))
				assert.Loosely(t, clogger.entry[2].Trace, should.Equal(fmt.Sprintf("%x", streamIDHash)))
			})

			t.Run("Skip, if sum(tags) is too big", func(t *ftt.Test) {
				ts.add(123)
				str := make([]byte, maxTagSum/2+100)
				for i := range str {
					str[i] = "1234567890"[i%10]
				}
				desc.Tags = map[string]string{
					string(str): string(str),
				}
				_, err := Archive(m)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, clogger.entry, should.HaveLength(0))
			})
		})
	})
}
