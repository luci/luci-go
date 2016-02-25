// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deps

import (
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/prpc"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

const ek = logging.ErrorKey

type deps struct{}

var _ dm.DepsServer = (*deps)(nil)

// RegisterDepsServer registers an implementation of the dm.DepsServer with
// the provided Registrar.
func RegisterDepsServer(svr prpc.Registrar) {
	dm.RegisterDepsServer(svr, &deps{})
}

// tumbleNow will run the mutation immediately, converting any non grpc errors
// to codes.Internal.
func tumbleNow(c context.Context, m tumble.Mutation) error {
	err := tumble.RunMutation(c, m)
	if grpc.Code(err) == codes.Unknown {
		logging.WithError(err).Errorf(c, "unknown error while applying mutation %v", m)
		err = grpcutil.Internal
	}
	return err
}
