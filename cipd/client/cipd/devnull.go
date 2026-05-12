// Copyright 2026 The LUCI Authors.
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

package cipd

import (
	"context"
	"hash"
	"io"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/common/cipderr"
)

var ErrNetworkDisabled = cipderr.OfflineClient.Apply(errors.New("network is disabled"))

func mkDevNullErr(service, method string) error {
	return errors.Fmt("%s/%s: %w", service, method, ErrNetworkDisabled)
}

type devNullConn string

var _ grpc.ClientConnInterface = devNullConn("")

// Invoke performs a unary RPC and returns after the response is received
// into reply.
func (d devNullConn) Invoke(ctx context.Context, method string, args any, reply any, opts ...grpc.CallOption) error {
	return mkDevNullErr(string(d), method)
}

// NewStream begins a streaming RPC.
func (d devNullConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, mkDevNullErr(string(d), method)
}

type storageDevNull struct{}

var _ storage = storageDevNull{}

func (storageDevNull) upload(ctx context.Context, url string, data io.ReadSeeker) error {
	return mkDevNullErr("storage", "upload")
}

func (storageDevNull) download(ctx context.Context, url string, output io.WriteSeeker, h hash.Hash) error {
	return mkDevNullErr("storage", "download")
}
