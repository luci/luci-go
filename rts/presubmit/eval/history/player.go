// Copyright 2020 The LUCI Authors.
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

package history

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

// Player can playback a history from a reader.
// It takes care of reconstructing rejections from fragments.
type Player struct {
	RejectionC   chan *evalpb.Rejection
	DurationC    chan *evalpb.TestDuration
	r            *Reader
	totalRecords int64
}

// TotalRecords returns the number of records observed thus far.
func (p *Player) TotalRecords() int {
	return int(atomic.LoadInt64(&p.totalRecords))
}

// NewPlayer returns a new Player.
func NewPlayer(r *Reader) *Player {
	return &Player{
		r:          r,
		RejectionC: make(chan *evalpb.Rejection),
		DurationC:  make(chan *evalpb.TestDuration),
	}
}

// Playback reads historical records and dispatches them to p.RejectionC and
// p.DurationC.
// Before exiting, closes RejectionC, DurationC and the underlying reader.
func (p *Player) Playback(ctx context.Context) error {
	return p.playback(ctx, true)
}

// PlaybackIgnoreDurations is like Playback, except it ignores all duration
// data.
func (p *Player) PlaybackIgnoreDurations(ctx context.Context) error {
	return p.playback(ctx, false)
}

func (p *Player) playback(ctx context.Context, withDurations bool) error {
	defer func() {
		close(p.RejectionC)
		close(p.DurationC)
		p.r.Close()
	}()

	curRej := &evalpb.Rejection{}
	for {
		rec, err := p.r.Read()
		switch {
		case err == io.EOF:
			return p.r.Close()
		case err != nil:
			return errors.Annotate(err, "failed to read history").Err()
		}

		atomic.AddInt64(&p.totalRecords, 1)

		// Send the record to the appropriate channel.
		switch data := rec.Data.(type) {

		case *evalpb.Record_RejectionFragment:
			proto.Merge(curRej, data.RejectionFragment.Rejection)
			if data.RejectionFragment.Terminal {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case p.RejectionC <- curRej:
					// Start a new rejection.
					curRej = &evalpb.Rejection{}
				}
			}

		case *evalpb.Record_TestDuration:
			if withDurations {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case p.DurationC <- data.TestDuration:
				}
			}

		default:
			panic(fmt.Sprintf("unexpected record %s", rec))
		}
	}
}
