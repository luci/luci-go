// Copyright 2021 The LUCI Authors.
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

package aggrmetrics

import (
	"context"

	"go.chromium.org/luci/server/tq"
)

func NewAggegator(tqd *tq.Dispatcher) *Aggegator {
	return &Aggegator{tqd: tqd}
}

type Aggegator struct {
	tqd *tq.Dispatcher
}

func (a *Aggegator) MinuteCron(ctx context.Context) error {
	// TODO(tandrii): implement.
	return nil
}
