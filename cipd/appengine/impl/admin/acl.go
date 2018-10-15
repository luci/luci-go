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

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"

	api "go.chromium.org/luci/cipd/api/admin/v1"
)

// AdminGroup is a name of a group with accounts that can use Admin API.
const AdminGroup = "administrators"

// AdminAPI returns an ACL-protected implementation of cipd.AdminServer that can
// be exposed as a public API (i.e. admins can use it via external RPCs).
func AdminAPI(d *tq.Dispatcher) api.AdminServer {
	impl := &adminImpl{tq: d}
	impl.init()
	return &api.DecoratedAdmin{
		Service: impl,
		Prelude: checkAdminPrelude,
	}
}

// checkAdminPrelude is called before each RPC to check the caller is in
// "administrators" group.
func checkAdminPrelude(c context.Context, method string, req proto.Message) (context.Context, error) {
	switch yep, err := auth.IsMember(c, AdminGroup); {
	case err != nil:
		logging.WithError(err).Errorf(c, "IsMember(%q) failed", AdminGroup)
		return nil, status.Errorf(codes.Internal, "failed to check ACL")
	case !yep:
		logging.Warningf(c, "Denying access to %q for %q, not in %q group", method, auth.CurrentIdentity(c), AdminGroup)
		return nil, status.Errorf(codes.PermissionDenied, "not allowed")
	}
	logging.Infof(c, "Admin %q is calling %q", auth.CurrentIdentity(c), method)
	return c, nil
}
