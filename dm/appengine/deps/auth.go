// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package deps

import (
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"

	"github.com/golang/protobuf/proto"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/dm/api/acls"
	"github.com/luci/luci-go/grpc/grpcutil"
	"github.com/luci/luci-go/server/auth"
	"golang.org/x/net/context"
)

func getTrimmedAppID(c context.Context) string {
	// custom domains show up as "foo.com:appid"
	toks := strings.Split(info.Get(c).AppID(), ":")
	return toks[len(toks)-1]
}

func loadAcls(c context.Context) (ret *acls.Acls, err error) {
	aid := getTrimmedAppID(c)
	aclCfg, err := config.GetConfig(c, fmt.Sprintf("services/%s", aid), "acls.cfg", false)
	if err != nil {
		return nil, errors.WrapTransient(err)
	}

	ret = &acls.Acls{}
	err = proto.UnmarshalText(aclCfg.Content, ret)
	return
}

func inGroups(c context.Context, groups []string) error {
	for _, grp := range groups {
		ok, err := auth.IsMember(c, grp)
		if err != nil {
			return grpcutil.MaybeLogErr(c, err, codes.Internal, "failed group check")
		}
		if ok {
			return nil
		}
	}
	logging.Fields{
		"ident":  auth.CurrentIdentity(c),
		"groups": groups,
	}.Infof(c, "not authorized")
	return grpcutil.Errf(codes.Unauthenticated, "not authorized")
}

func canRead(c context.Context) (err error) {
	acl, err := loadAcls(c)
	if err != nil {
		return
	}
	return inGroups(c, acl.Readers)
}

func canWrite(c context.Context) (err error) {
	acl, err := loadAcls(c)
	if err != nil {
		return
	}
	return inGroups(c, acl.Writers)
}
