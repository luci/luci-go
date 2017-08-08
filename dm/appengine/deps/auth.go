// Copyright 2016 The LUCI Authors.
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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/dm/api/acls"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/luci_config/server/cfgclient"
	"go.chromium.org/luci/luci_config/server/cfgclient/textproto"
	"go.chromium.org/luci/server/auth"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

func loadAcls(c context.Context) (ret *acls.Acls, err error) {
	cSet := cfgclient.CurrentServiceConfigSet(c)
	file := "acls.cfg"

	ret = &acls.Acls{}
	if err := cfgclient.Get(c, cfgclient.AsService, cSet, file, textproto.Message(ret), nil); err != nil {
		return nil, errors.Annotate(err, "").Tag(transient.Tag).
			InternalReason("loading config :: cSet(%v)/file(%v)", cSet, file).Err()
	}
	return
}

func inGroups(c context.Context, groups []string) error {
	for _, grp := range groups {
		ok, err := auth.IsMember(c, grp)
		if err != nil {
			return grpcAnnotate(err, codes.Internal, "failed group check").Err()
		}
		if ok {
			return nil
		}
	}
	logging.Fields{
		"ident":  auth.CurrentIdentity(c),
		"groups": groups,
	}.Infof(c, "not authorized")
	return grpcutil.Errf(codes.PermissionDenied, "not authorized")
}

func canRead(c context.Context) (err error) {
	acl, err := loadAcls(c)
	if err != nil {
		return
	}
	if err = inGroups(c, acl.Readers); grpcutil.Code(err) == codes.PermissionDenied {
		err = inGroups(c, acl.Writers)
	}
	return
}

func canWrite(c context.Context) (err error) {
	acl, err := loadAcls(c)
	if err != nil {
		return
	}
	return inGroups(c, acl.Writers)
}
