// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package deps

import (
	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logging"
	dm "github.com/luci/luci-go/dm/api/service/v1"
	"github.com/luci/luci-go/dm/appengine/distributor"
	"github.com/luci/luci-go/server/prpc"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

const ek = logging.ErrorKey

type deps struct{}

var _ dm.DepsServer = (*deps)(nil)

func depsServerPrelude(reg distributor.Registry) func(context.Context, string, proto.Message) (context.Context, error) {
	return func(c context.Context, methodName string, req proto.Message) (context.Context, error) {
		// Many of the DM request messages can be Normalize'd. This checks them for
		// basic validity and normalizes cases where multiple representations can mean
		// the same thing so that the service handlers only need to check for the
		// canonical representation.
		if norm, ok := req.(interface {
			Normalize() error
		}); ok {
			if err := norm.Normalize(); err != nil {
				return nil, grpcutil.MaybeLogErr(c, err, codes.InvalidArgument, "invalid request")
			}
		}
		c = distributor.WithRegistry(c, reg)
		return c, nil
	}
}

func newDecoratedDeps(reg distributor.Registry) dm.DepsServer {
	return &dm.DecoratedDeps{
		Service: &deps{},
		Prelude: depsServerPrelude(reg),
	}
}

// RegisterDepsServer registers an implementation of the dm.DepsServer with
// the provided Registrar.
func RegisterDepsServer(svr prpc.Registrar, reg distributor.Registry) {
	dm.RegisterDepsServer(svr, newDecoratedDeps(reg))
}

// tumbleNow will run the mutation immediately, converting any non grpc errors
// to codes.Internal.
func tumbleNow(c context.Context, m tumble.Mutation) error {
	err := tumble.RunMutation(c, m)
	if grpc.Code(err) == codes.Unknown {
		logging.WithError(err).Errorf(c, "unknown error while applying mutation %v", m)
		err = grpcutil.Internal
	}
	logging.Fields{"root": m.Root(c)}.Infof(c, "tumbleNow success")
	return err
}
