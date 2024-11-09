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

package log

import (
	"context"
	"encoding/hex"
	"strconv"
	"sync"

	"github.com/golang/protobuf/proto"

	log "go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/bootstrap"
	"go.chromium.org/luci/logdog/client/butler/output"
	"go.chromium.org/luci/logdog/common/types"
)

// logOutput is an Output implementation that logs messages to its contexts'
// Logger instance.
type logOutput struct {
	sync.Mutex
	ctx context.Context

	// bundleSize is the maximum size of the Butler bundle to use.
	bundleSize int

	stats output.StatsBase
}

// logOutput implements output.Output.
var _ output.Output = (*logOutput)(nil)

// New instantes a new log output instance.
func New(ctx context.Context, bundleSize int) output.Output {
	o := logOutput{
		ctx:        ctx,
		bundleSize: bundleSize,
	}
	o.ctx = log.SetFields(o.ctx, log.Fields{
		"output": &o,
	})
	return &o
}

func (o *logOutput) String() string { return "log" }

func (o *logOutput) SendBundle(bundle *logpb.ButlerLogBundle) error {
	o.Lock()
	defer o.Unlock()

	for _, e := range bundle.Entries {
		path := types.StreamName(bundle.Prefix).Join(types.StreamName(e.Desc.Name))
		ctx := log.SetField(o.ctx, "streamPath", path)

		log.Fields{
			"count":      len(e.Logs),
			"descriptor": e.Desc.String(),
		}.Infof(ctx, "Received stream logs.")

		for _, le := range e.Logs {
			log.Fields{
				"timeOffset":  le.TimeOffset.AsDuration(),
				"prefixIndex": le.PrefixIndex,
				"streamIndex": le.StreamIndex,
				"sequence":    le.Sequence,
			}.Infof(ctx, "Received message.")
			if c := le.GetText(); c != nil {
				for idx, l := range c.Lines {
					log.Infof(ctx, "Line %d) %s (%s)", idx, l.Value, strconv.Quote(l.Delimiter))
				}
			}
			if c := le.GetBinary(); c != nil {
				log.Infof(ctx, "Binary) %s", hex.EncodeToString(c.Data))
			}
			if c := le.GetDatagram(); c != nil {
				if cp := c.Partial; cp != nil {
					log.Infof(ctx, "Datagram (%#v) (%d bytes): %s", cp, cp.Size, hex.EncodeToString(c.Data))
				} else {
					log.Infof(ctx, "Datagram (%d bytes): %s", len(c.Data), hex.EncodeToString(c.Data))
				}
			}

			o.stats.F.SentMessages++
		}
	}
	o.stats.F.SentBytes += int64(proto.Size(bundle))

	return nil
}

func (o *logOutput) MaxSendBundles() int {
	return 1
}

func (o *logOutput) MaxSize() int {
	return o.bundleSize
}

func (o *logOutput) Stats() output.Stats {
	o.Lock()
	defer o.Unlock()

	st := o.stats
	return &st
}

func (o *logOutput) URLConstructionEnv() bootstrap.Environment {
	return bootstrap.Environment{}
}

func (o *logOutput) Close() {}
