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

package migration

import (
	"context"

	protov1 "github.com/golang/protobuf/proto"

	gaetq "go.chromium.org/luci/appengine/tq"
	servertq "go.chromium.org/luci/server/tq"
)

// TQ is a subset of servertq.Dispatcher.
type TQ interface {
	RegisterTaskClass(cls servertq.TaskClass) servertq.TaskClassRef
	AddTask(ctx context.Context, task *servertq.Task) error
}

var _ TQ = (*servertq.Dispatcher)(nil)

// AppengineTQ implements TQ via appengine/tq package.
type AppengineTQ struct {
	TQ gaetq.Dispatcher
}

func NewAppengineTQ() *AppengineTQ {
	return &AppengineTQ{
		TQ: gaetq.Dispatcher{
			BaseURL: "/internal/tq/",
		},
	}
}

func (tq *AppengineTQ) RegisterTaskClass(cls servertq.TaskClass) servertq.TaskClassRef {
	tq.TQ.RegisterTask(protov1.MessageV1(cls.Prototype), func(ctx context.Context, payload protov1.Message) error {
		return cls.Handler(ctx, protov1.MessageV2(payload))
	}, cls.Queue, nil)
	return nil
}

func (tq *AppengineTQ) AddTask(ctx context.Context, task *servertq.Task) error {
	return tq.TQ.AddTask(ctx, &gaetq.Task{
		Title:   task.Title,
		Payload: protov1.MessageV1(task.Payload),
	})
}
