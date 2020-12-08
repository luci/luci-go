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

package impl

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/internal"
)

func init() {
	prjmanager.PokePMTaskRef.AttachHandler(
		func(ctx context.Context, payload proto.Message) error {
			task := payload.(*internal.PokePMTask)
			switch err := pokePMTask(ctx, task.GetLuciProject()); {
			case err == nil:
				return nil
			case !transient.Tag.In(err):
				return tq.Fatal.Apply(err)
			default:
				// TODO(tandrii): avoid retries iff we know a new task was already
				// scheduled for the next second.
				return err
			}
		},
	)
}

// maxEventsToProcess. Since events are currently deleted transactionally,
// this must be well below max # of entities a transaction can touch.
const maxEventsToProcess = 256

func pokePMTask(ctx context.Context, luciProject string) error {
	ctx = logging.SetField(ctx, "project", luciProject)
	// Read state outside a transaction.
	eg, ectx := errgroup.WithContext(ctx)
	var triaged *triageResult
	eg.Go(func() (err error) {
		triaged, err = triage(ectx, luciProject)
		return
	})
	var sm *stateMachine
	eg.Go(func() (err error) {
		sm, err = readState(ectx, luciProject)
		return
	})
	if err := eg.Wait(); err != nil {
		return err
	}
	return sm.mutate(ctx, triaged)
}

// triage triages incoming events.
func triage(ctx context.Context, luciProject string) (*triageResult, error) {
	events, err := internal.Peek(ctx, luciProject, maxEventsToProcess)
	if err != nil {
		return nil, err
	}
	ret := &triageResult{}
	for _, e := range events {
		ret.triage(e)
	}
	return ret, nil
}

// triageResult is the result of the triage of the incoming events.
type triageResult struct {
	updateConfig []*internal.Event
}

func (r *triageResult) triage(e *internal.Event) {
	switch p := e.Payload; p.GetEvent().(type) {
	case *internal.Payload_UpdateConfig:
		r.updateConfig = append(r.updateConfig, e)
	default:
		panic(fmt.Errorf("unknown event: %T", p.GetEvent()))
	}
}
