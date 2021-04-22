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
	"google.golang.org/protobuf/proto"

	gaetq "go.chromium.org/luci/appengine/tq"
)

type Handler func(ctx context.Context, payload proto.Message) error

type TQ interface {
	RegisterTask(prototype proto.Message, cb Handler, queue string)
	AddTask(ctx context.Context, title string, payload proto.Message) error
}

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

func (tq *AppengineTQ) RegisterTask(prototype proto.Message, cb Handler, queue string) {
	tq.TQ.RegisterTask(protov1.MessageV1(prototype), func(ctx context.Context, payload protov1.Message) error {
		return cb(ctx, protov1.MessageV2(payload))
	}, queue, nil)
}

func (tq *AppengineTQ) AddTask(ctx context.Context, title string, payload proto.Message) error {
	return tq.TQ.AddTask(ctx, &gaetq.Task{
		Title:   title,
		Payload: protov1.MessageV1(payload),
	})
}
