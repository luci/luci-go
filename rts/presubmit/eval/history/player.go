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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

// Player can playback a history from a reader.
// It takes care of reconstructing rejections from fragments.
type Player struct {
	RejectionC chan *evalpb.Rejection
	DurationC  chan *evalpb.TestDuration
	r          *Reader
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
//
// TODO(nodir): refactor this package and potentially the file format.
// This function is never used.
func (p *Player) Playback(ctx context.Context) error {
	return p.playback(ctx, true, true)
}

// PlaybackRejections is like Playback, except reports only rejections.
func (p *Player) PlaybackRejections(ctx context.Context) error {
	return p.playback(ctx, false, true)
}

// PlaybackDurations is like Playback, except reports only durations.
func (p *Player) PlaybackDurations(ctx context.Context) error {
	return p.playback(ctx, true, false)
}

func (p *Player) playback(ctx context.Context, withDurations, withRejections bool) error {
	defer func() {
		close(p.RejectionC)
		close(p.DurationC)
		p.r.Close()
	}()

	var rejSynth RejectionSynth
	for {
		rec, err := p.r.Read()
		switch {
		case err == io.EOF:
			return p.r.Close()
		case err != nil:
			return errors.Annotate(err, "failed to read history").Err()
		}

		// Send the record to the appropriate channel.
		switch data := rec.Data.(type) {

		case *evalpb.Record_RejectionFragment:
			if withRejections {
				if rej := rejSynth.Next(data.RejectionFragment); rej != nil {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case p.RejectionC <- rej:
					}
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

type RejectionSynth struct {
	curRej *evalpb.Rejection
}

func (s *RejectionSynth) Next(frag *evalpb.RejectionFragment) *evalpb.Rejection {
	if s.curRej == nil {
		s.curRej = &evalpb.Rejection{}
	}
	proto.Merge(s.curRej, frag.Rejection)
	if !frag.Terminal {
		return nil
	}
	ret := s.curRej
	s.curRej = nil
	return ret
}
