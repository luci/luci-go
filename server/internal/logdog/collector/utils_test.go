// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package collector

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logdog/butlerproto"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	cc "github.com/luci/luci-go/server/internal/logdog/collector/coordinator"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
)

var testSecret = bytes.Repeat([]byte{0x55}, types.StreamSecretLength)

// testCoordinator is an implementation of Coordinator that can be used for
// testing.
type testCoordinator struct {
	sync.Mutex

	// errC is a channel that error status will be read from if not nil.
	errC chan error

	// state is the latest tracked stream state.
	state map[string]*cc.LogStreamState
}

var _ cc.Coordinator = (*testCoordinator)(nil)

func (c *testCoordinator) register(s cc.LogStreamState) cc.LogStreamState {
	c.Lock()
	defer c.Unlock()

	// Update our state.
	if c.state == nil {
		c.state = make(map[string]*cc.LogStreamState)
	}
	if sp := c.state[string(s.Path)]; sp != nil {
		return *sp
	}
	c.state[string(s.Path)] = &s
	return s
}

func (c *testCoordinator) RegisterStream(ctx context.Context, s *cc.LogStreamState, d *logpb.LogStreamDescriptor) (
	*cc.LogStreamState, error) {
	if err := c.enter(); err != nil {
		return nil, err
	}

	// Enforce that the submitted stream is not terminal.
	rs := *s
	rs.TerminalIndex = -1

	// Update our state.
	sp := c.register(rs)
	return &sp, nil
}

func (c *testCoordinator) TerminateStream(ctx context.Context, st *cc.LogStreamState) error {
	if err := c.enter(); err != nil {
		return err
	}

	if st.TerminalIndex < 0 {
		return errors.New("submitted stream is not terminal")
	}

	c.Lock()
	defer c.Unlock()

	// Update our state.
	cachedState, ok := c.state[string(st.Path)]
	if !ok {
		return fmt.Errorf("no such stream: %s", st.Path)
	}
	if cachedState.TerminalIndex >= 0 && st.TerminalIndex != cachedState.TerminalIndex {
		return fmt.Errorf("incompatible terminal indexes: %d != %d", st.TerminalIndex, cachedState.TerminalIndex)
	}

	cachedState.TerminalIndex = st.TerminalIndex
	return nil
}

// enter is an entry point for client goroutines. It offers the opportunity
// to trap executing goroutines within client calls.
//
// This must NOT be called while the lock is held, else it could lead to
// deadlock if multiple goroutines are trapped.
func (c *testCoordinator) enter() error {
	if c.errC != nil {
		return <-c.errC
	}
	return nil
}

func (c *testCoordinator) stream(name string) (int, bool) {
	c.Lock()
	defer c.Unlock()

	sp, ok := c.state[name]
	if !ok {
		return 0, false
	}
	return int(sp.TerminalIndex), true
}

// testStorage is a testing storage instance that returns errors.
type testStorage struct {
	storage.Storage
	err func() error
}

func (s *testStorage) Put(r storage.PutRequest) error {
	if s.err != nil {
		if err := s.err(); err != nil {
			return err
		}
	}
	return s.Storage.Put(r)
}

// bundleBuilder is a set of utility functions to help test cases construct
// specific logpb.ButlerLogBundle layouts.
type bundleBuilder struct {
	context.Context

	base    time.Time
	entries []*logpb.ButlerLogBundle_Entry
}

func (b *bundleBuilder) addBundleEntry(be *logpb.ButlerLogBundle_Entry) {
	if b.base.IsZero() {
		b.base = clock.Now(b)
	}

	b.entries = append(b.entries, be)
}

func (b *bundleBuilder) genBundleEntry(name string, tidx int, idxs ...int) *logpb.ButlerLogBundle_Entry {
	p, n := types.StreamPath(name).Split()
	be := logpb.ButlerLogBundle_Entry{
		Secret: testSecret,
		Desc: &logpb.LogStreamDescriptor{
			Prefix:      string(p),
			Name:        string(n),
			ContentType: "application/test-message",
			StreamType:  logpb.StreamType_TEXT,
			Timestamp:   google.NewTimestamp(clock.Now(b)),
		},
	}

	if len(idxs) > 0 {
		be.Logs = make([]*logpb.LogEntry, len(idxs))
		for i, idx := range idxs {
			be.Logs[i] = b.logEntry(idx)
		}
		if tidx >= 0 {
			be.Terminal = true
			be.TerminalIndex = uint64(tidx)
		}
	}

	return &be
}

func (b *bundleBuilder) addStreamEntries(name string, term int, idxs ...int) {
	b.addBundleEntry(b.genBundleEntry(name, term, idxs...))
}

func (b *bundleBuilder) addFullStream(name string, count int) {
	idxs := make([]int, count)
	for i := range idxs {
		idxs[i] = i
	}
	b.addStreamEntries(name, count-1, idxs...)
}

func (b *bundleBuilder) logEntry(idx int) *logpb.LogEntry {
	return &logpb.LogEntry{
		StreamIndex: uint64(idx),
		Sequence:    uint64(idx),
		Content: &logpb.LogEntry_Text{
			Text: &logpb.Text{
				Lines: []*logpb.Text_Line{
					{
						Value:     fmt.Sprintf("Line #%d", idx),
						Delimiter: "\n",
					},
				},
			},
		},
	}
}

func (b *bundleBuilder) bundle() []byte {
	bytes := b.bundleWithEntries(b.entries...)
	b.entries = nil

	return bytes
}

func (b *bundleBuilder) bundleWithEntries(e ...*logpb.ButlerLogBundle_Entry) []byte {
	bundle := logpb.ButlerLogBundle{
		Source:    "test stream",
		Timestamp: google.NewTimestamp(clock.Now(b)),
		Entries:   e,
	}

	buf := bytes.Buffer{}
	w := butlerproto.Writer{Compress: true}
	if err := w.Write(&buf, &bundle); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

type indexRange struct {
	start int
	end   int
}

func (r *indexRange) String() string { return fmt.Sprintf("[%d..%d]", r.start, r.end) }

// shouldHaveRegisteredStream asserts that a testCoordinator has
// registered a stream (string) and its terminal index (int).
func shouldHaveRegisteredStream(actual interface{}, expected ...interface{}) string {
	tcc := actual.(*testCoordinator)
	name := expected[0].(string)
	tidx := expected[1].(int)

	cur, ok := tcc.stream(name)
	if !ok {
		return fmt.Sprintf("stream %q is not registered", name)
	}
	if tidx >= 0 && cur < 0 {
		return fmt.Sprintf("stream %q is expected to be terminated, but isn't.", name)
	}
	if cur >= 0 && tidx < 0 {
		return fmt.Sprintf("stream %q is NOT expected to be terminated, but it is.", name)
	}
	return ""
}

// shoudNotHaveRegisteredStream asserts that a testCoordinator has not
// registered a stream (string).
func shouldNotHaveRegisteredStream(actual interface{}, expected ...interface{}) string {
	tcc := actual.(*testCoordinator)
	name := expected[0].(string)

	if _, ok := tcc.stream(name); ok {
		return fmt.Sprintf("stream %q is registered, but it should NOT be.", name)
	}
	return ""
}

// shouldHaveStoredStream asserts that a storage.Storage instance has contiguous
// stream records in it.
//
// actual is the storage.Storage instance. expected is a stream name (string)
// followed by a a series of records to assert. This can either be a specific
// integer index or an intexRange marking a closed range of indices.
func shouldHaveStoredStream(actual interface{}, expected ...interface{}) string {
	st := actual.(storage.Storage)
	name := expected[0].(string)

	// Load all entries for this stream.
	req := storage.GetRequest{
		Path: types.StreamPath(name),
	}

	entries := make(map[int]*logpb.LogEntry)
	var ierr error
	err := st.Get(req, func(idx types.MessageIndex, d []byte) bool {
		le := logpb.LogEntry{}
		if ierr = proto.Unmarshal(d, &le); ierr != nil {
			return false
		}
		entries[int(idx)] = &le
		return true
	})
	if ierr != nil {
		err = ierr
	}
	if err != nil && err != storage.ErrDoesNotExist {
		return fmt.Sprintf("error: %v", err)
	}

	assertLogEntry := func(i int) string {
		le := entries[i]
		if le == nil {
			return fmt.Sprintf("%d", i)
		}
		delete(entries, i)

		if le.StreamIndex != uint64(i) {
			return fmt.Sprintf("*%d", i)
		}
		return ""
	}

	var failed []string
	for _, exp := range expected[1:] {
		switch e := exp.(type) {
		case int:
			if err := assertLogEntry(e); err != "" {
				failed = append(failed, fmt.Sprintf("missing{%s}", err))
			}

		case indexRange:
			var errs []string
			for i := e.start; i <= e.end; i++ {
				if err := assertLogEntry(i); err != "" {
					errs = append(errs, err)
				}
			}
			if len(errs) > 0 {
				failed = append(failed, fmt.Sprintf("%s{%s}", e.String(), strings.Join(errs, ",")))
			}

		default:
			panic(fmt.Errorf("unknown expected type %T", e))
		}
	}

	// Extras?
	if len(entries) > 0 {
		idxs := make([]int, 0, len(entries))
		for i := range entries {
			idxs = append(idxs, i)
		}
		sort.Ints(idxs)

		extra := make([]string, len(idxs))
		for i, idx := range idxs {
			extra[i] = fmt.Sprintf("%d", idx)
		}
		failed = append(failed, fmt.Sprintf("extra{%s}", strings.Join(extra, ",")))
	}

	if len(failed) > 0 {
		return strings.Join(failed, ", ")
	}
	return ""
}
