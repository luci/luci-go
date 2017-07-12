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
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/dm/api/service/v1"
	"github.com/luci/luci-go/dm/appengine/mutate"
	"golang.org/x/net/context"
)

func (d *deps) FinishAttempt(c context.Context, req *dm.FinishAttemptReq) (_ *empty.Empty, err error) {
	logging.Fields{"execution": req.Auth.Id}.Infof(c, "finishing")
	return &empty.Empty{}, tumbleNow(c, &mutate.FinishAttempt{
		FinishAttemptReq: *req})
}
