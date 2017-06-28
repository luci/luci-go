// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package deps

import (
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry/transient"
	"github.com/luci/luci-go/dm/api/acls"
	"github.com/luci/luci-go/grpc/grpcutil"
	"github.com/luci/luci-go/luci_config/server/cfgclient"
	"github.com/luci/luci-go/luci_config/server/cfgclient/textproto"
	"github.com/luci/luci-go/server/auth"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

func loadAcls(c context.Context) (ret *acls.Acls, err error) {
	cSet := cfgclient.CurrentServiceConfigSet(c)
	file := "acls.cfg"

	ret = &acls.Acls{}
	if err := cfgclient.Get(c, cfgclient.AsService, cSet, file, textproto.Message(ret), nil); err != nil {
		return nil, errors.Annotate(err).Tag(transient.Tag).
			D("cSet", cSet).D("file", file).InternalReason("loading config").Err()
	}
	return
}

func inGroups(c context.Context, groups []string) error {
	for _, grp := range groups {
		ok, err := auth.IsMember(c, grp)
		if err != nil {
			return grpcAnnotate(err, codes.Internal).Reason("failed group check").Err()
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
