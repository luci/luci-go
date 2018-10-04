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

package deps

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/dm/appengine/mutate"
)

func (d *deps) FinishAttempt(c context.Context, req *dm.FinishAttemptReq) (_ *empty.Empty, err error) {
	logging.Fields{"execution": req.Auth.Id}.Infof(c, "finishing")
	return &empty.Empty{}, tumbleNow(c, &mutate.FinishAttempt{
		FinishAttemptReq: *req})
}
