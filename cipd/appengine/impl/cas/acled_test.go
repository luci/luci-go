// Copyright 2017 The LUCI Authors.
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

package cas

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
)

func TestACLDecorator(t *testing.T) {
	t.Parallel()

	acledSrv := Public(&api.UnimplementedStorageServer{})

	anon := identity.AnonymousIdentity
	someone := identity.Identity("user:someone@example.com")
	admin := identity.Identity("user:admin@example.com")

	state := &authtest.FakeState{
		FakeDB: authtest.NewFakeDB(
			authtest.MockMembership(admin, "administrators"),
		),
	}
	ctx := auth.WithState(context.Background(), state)

	getObjectURL := func() (any, error) {
		return acledSrv.GetObjectURL(ctx, nil)
	}
	beginUpload := func() (any, error) {
		return acledSrv.BeginUpload(ctx, nil)
	}
	noForceHash := func() (any, error) {
		return acledSrv.FinishUpload(ctx, &api.FinishUploadRequest{})
	}
	withForceHash := func() (any, error) {
		return acledSrv.FinishUpload(ctx, &api.FinishUploadRequest{ForceHash: &api.ObjectRef{}})
	}
	cancelReq := func() (any, error) {
		return acledSrv.CancelUpload(ctx, &api.CancelUploadRequest{})
	}

	var cases = []struct {
		method  string
		caller  identity.Identity
		request func() (any, error)
		allowed bool
	}{
		{"GetObjectURL", anon, getObjectURL, false},
		{"GetObjectURL", someone, getObjectURL, false},
		{"GetObjectURL", admin, getObjectURL, true},

		{"BeginUpload", anon, beginUpload, false},
		{"BeginUpload", someone, beginUpload, false},
		{"BeginUpload", admin, beginUpload, true},

		{"FinishUpload", anon, noForceHash, true},
		{"FinishUpload", someone, noForceHash, true},
		{"FinishUpload", admin, noForceHash, true},

		{"FinishUpload", anon, withForceHash, false},
		{"FinishUpload", someone, withForceHash, false},
		{"FinishUpload", admin, withForceHash, false},

		{"CancelUpload", anon, cancelReq, true},
		{"CancelUpload", someone, cancelReq, true},
		{"CancelUpload", admin, cancelReq, true},
	}

	for idx, cs := range cases {
		ftt.Run(fmt.Sprintf("%d - %s by %s", idx, cs.method, cs.caller), t, func(t *ftt.Test) {
			state.Identity = cs.caller
			_, err := cs.request()
			if cs.allowed {
				assert.Loosely(t, status.Code(err), should.Equal(codes.Unimplemented))
			} else {
				assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			}
		})
	}
}
