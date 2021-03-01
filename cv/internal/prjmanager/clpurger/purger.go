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

package clpurger

import (
	"context"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

func init() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        prjpb.PurgeCLTaskClass,
		Prototype: &prjpb.PurgeCLTask{},
		Queue:     "purge-project-cl",
		Quiet:     true,
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*prjpb.PurgeCLTask)
			err := PurgeCL(ctx, task)
			return common.TQifyError(ctx, err)
		},
	})
}

func PurgeCL(ctx context.Context, task *prjpb.PurgeCLTask) error {
	return errors.New("Not implemented")
}
