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
	"os"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	dm "go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/tumble"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

const ek = logging.ErrorKey

type deps struct{}

var _ dm.DepsServer = (*deps)(nil)

func depsServerPrelude(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	// Many of the DM request messages can be Normalize'd. This checks them for
	// basic validity and normalizes cases where multiple representations can mean
	// the same thing so that the service handlers only need to check for the
	// canonical representation.
	if norm, ok := req.(interface {
		Normalize() error
	}); ok {
		if err := norm.Normalize(); err != nil {
			return nil, grpcAnnotate(err, codes.InvalidArgument, "invalid request").Err()
		}
	}
	return c, nil
}

const postludeDebugEnvvar = "DUMP_ALL_STACKS"

var postludeOmitCodes = map[codes.Code]struct{}{
	codes.OK:                 {},
	codes.Unauthenticated:    {},
	codes.AlreadyExists:      {},
	codes.FailedPrecondition: {},
	codes.InvalidArgument:    {},
	codes.NotFound:           {},
	codes.OutOfRange:         {},
	codes.Aborted:            {},
}

func depsServerPostlude(c context.Context, methodName string, rsp proto.Message, err error) error {
	retErr := grpcutil.ToGRPCErr(err)
	if err != nil {
		code := codes.OK
		_, printStack := os.LookupEnv(postludeDebugEnvvar)
		if !printStack {
			code = grpc.Code(retErr)
			_, omitStack := postludeOmitCodes[code]
			printStack = !omitStack
		}
		if printStack {
			errors.Log(c, err)
		} else {
			logging.Infof(c, "returning gRPC code: %s", code)
		}
	}
	return retErr
}

func newDecoratedDeps() dm.DepsServer {
	return &dm.DecoratedDeps{
		Service:  &deps{},
		Prelude:  depsServerPrelude,
		Postlude: depsServerPostlude,
	}
}

// RegisterDepsServer registers an implementation of the dm.DepsServer with
// the provided Registrar.
func RegisterDepsServer(svr prpc.Registrar) {
	dm.RegisterDepsServer(svr, newDecoratedDeps())
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
