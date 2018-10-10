// Copyright 2018 The LUCI Authors.
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

package admin

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/logging"
)

// adminImpl implements cipd.AdminServer, with an assumption that auth check has
// already been done by the decorator setup in AdminAPI(), see acl.go.
type adminImpl struct {
	tq *tq.Dispatcher
}

// registerTasks adds tasks to the tq Dispatcher.
func (impl *adminImpl) registerTasks() {
	// TODO(vadimsh): Nothing here for now.
}

// SayHello implements the corresponding RPC method, see the proto doc.
func (impl *adminImpl) SayHello(c context.Context, _ *empty.Empty) (*empty.Empty, error) {
	logging.Infof(c, "Hello!")
	return &empty.Empty{}, nil
}
