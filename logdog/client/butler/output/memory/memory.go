// Copyright 2019 The LUCI Authors.
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

// Package memory implements an in-memory sink for the logdog Butler.
//
// This is meant for absorbing log data during testing of applications
// which expect a live Butler.
package memory

import (
	"strings"
	"sync"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/bootstrap"
	"go.chromium.org/luci/logdog/client/butler/output"
)

// Output implements the butler output.Output interface, but just
// accumulates the data in memory.
//
// For simplicity, this only retains a subset of the data transmitted
// by SendBundle (e.g. no timestamps, indexes, etc.).
//
// This assumes that SendBundle is called in order.
type Output struct {
	mu      sync.RWMutex
	streams map[StreamKey]*FakeStream
	stats   output.StatsBase
}

// GetStream returns the *FakeStream corresponding to the given prefix and
// stream name.
//
// If no such stream was opened, returns nil.
func (o *Output) GetStream(prefix, stream string) *FakeStream {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.streams[StreamKey{prefix, stream}]
}

var _ output.Output = (*Output)(nil)

type StreamKey struct {
	Prefix string
	Name   string
}

type FakeStream struct {
	stype logpb.StreamType
	tags  map[string]string

	mu          sync.Mutex
	lastIsFinal bool
	data        []*strings.Builder
}

func (fs *FakeStream) StreamType() logpb.StreamType {
	return fs.stype
}

func (fs *FakeStream) Tags() map[string]string {
	ret := make(map[string]string, len(fs.tags))
	for k, v := range fs.tags {
		ret[k] = v
	}
	return ret
}

func (fs *FakeStream) AllData() []string {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	ret := make([]string, len(fs.data))
	for i, dat := range fs.data {
		ret[i] = dat.String()
	}
	return ret
}

func (fs *FakeStream) LastData() string {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	return fs.data[len(fs.data)-1].String()
}

func (fs *FakeStream) AddData(be *logpb.ButlerLogBundle_Entry) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	switch fs.stype {
	case logpb.StreamType_TEXT:
		for _, logEntry := range be.Logs {
			for _, line := range logEntry.GetText().Lines {
				fs.data[0].Write(line.Value)
				fs.data[0].WriteString(line.Delimiter)
			}
		}
	case logpb.StreamType_BINARY:
		for _, logEntry := range be.Logs {
			fs.data[0].Write(logEntry.GetBinary().Data)
		}
	case logpb.StreamType_DATAGRAM:
		for _, logEntry := range be.Logs {
			dg := logEntry.GetDatagram()
			if fs.lastIsFinal {
				fs.data = append(fs.data, &strings.Builder{})
				fs.lastIsFinal = false
			}
			fs.data[len(fs.data)-1].Write(dg.Data)
			if dg.Partial == nil {
				fs.lastIsFinal = true
			}
		}
	default:
		panic(errors.Reason("unknown StreamType: %s", fs.stype).Err())
	}
}

// SendBundle implements output.Output
func (o *Output) SendBundle(b *logpb.ButlerLogBundle) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.stats.F.SentMessages += int64(len(b.Entries))
	o.stats.F.SentBytes += int64(proto.Size(b))

	if o.streams == nil {
		o.streams = map[StreamKey]*FakeStream{}
	}

	for _, bundleEntry := range b.Entries {
		sk := StreamKey{bundleEntry.Desc.Prefix, bundleEntry.Desc.Name}
		cur, ok := o.streams[sk]
		if !ok {
			cur = &FakeStream{
				stype: bundleEntry.Desc.StreamType,
				data:  []*strings.Builder{{}},
			}
			cur.tags = make(map[string]string, len(bundleEntry.Desc.Tags))
			for k, v := range bundleEntry.Desc.Tags {
				cur.tags[k] = v
			}
			o.streams[sk] = cur
		}

		cur.AddData(bundleEntry)
	}

	return nil
}

// MaxSendBundles implements output.Output
func (o *Output) MaxSendBundles() int {
	return 1
}

// Stats implements output.Output
func (o *Output) Stats() output.Stats {
	o.mu.RLock()
	defer o.mu.RUnlock()
	statsCp := o.stats
	return &statsCp
}

// URLConstructionEnv implements output.Output
func (o *Output) URLConstructionEnv() bootstrap.Environment {
	return bootstrap.Environment{
		Project: "memory",
		Prefix:  "memory",
	}
}

// MaxSize returns a large number instead of 0 because butler has bugs.
func (o *Output) MaxSize() int { return 1024 * 1024 * 1024 }

// Close implements output.Output
func (o *Output) Close() {}
