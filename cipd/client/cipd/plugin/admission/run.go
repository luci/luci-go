// Copyright 2020 The LUCI Authors.
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

package admission

import (
	"context"
	"io"
	"sync"

	"go.chromium.org/luci/cipd/client/cipd/plugin"
	"go.chromium.org/luci/cipd/client/cipd/plugin/protocol"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// protocolVersion will change if we have backward-incompatible changes.
const protocolVersion = 1

// Handler handles one admission request.
//
// Called in a separate internal goroutine. It should return a grpc status
// error.
type Handler func(ctx context.Context, req *protocol.Admission) error

// RunPlugin executes the run loop of an admission plugin.
//
// It connects to the host and starts handling admission checks (each in an
// individual goroutine) by calling the handler.
//
// Blocks until the stdin closes (which indicates the plugin should terminate).
func RunPlugin(ctx context.Context, stdin io.ReadCloser, version string, handler Handler) error {
	return plugin.Run(ctx, stdin, func(ctx context.Context, conn *grpc.ClientConn) error {
		srv := protocol.NewAdmissionsClient(conn)

		stream, err := srv.ListAdmissions(ctx, &protocol.ListAdmissionsRequest{
			ProtocolVersion: protocolVersion,
			PluginVersion:   version,
		})
		if err != nil {
			return err
		}

		wg := sync.WaitGroup{}
		defer wg.Wait()

		for {
			admission, err := stream.Recv()
			if err == io.EOF || isAborting(status.Code(err)) {
				break
			}

			wg.Add(1)
			go func() {
				defer wg.Done()

				var err error
				defer func() {
					if r := recover(); r != nil {
						err = status.Errorf(codes.Aborted, "panic in the admission handler: %s", r)
					}
					st, _ := status.FromError(err)
					srv.ResolveAdmission(ctx, &protocol.ResolveAdmissionRequest{
						AdmissionId: admission.AdmissionId,
						Status:      st.Proto(),
					})
				}()

				err = handler(ctx, admission)
			}()
		}

		return nil
	})
}

// isAborting recognizes a gRPC status from a closing streaming RPC.
func isAborting(code codes.Code) bool {
	return code == codes.Aborted ||
		code == codes.Canceled ||
		code == codes.Unavailable
}
