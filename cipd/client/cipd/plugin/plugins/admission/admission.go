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

// Package admission contains API for writing admission plugins.
package admission

import (
	"context"
	"io"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/client/cipd/plugin/plugins"
	"go.chromium.org/luci/cipd/client/cipd/plugin/protocol"
)

// ProtocolVersion will change if we have backward-incompatible changes.
const ProtocolVersion = 1

// Handler handles one admission request.
//
// Called in a separate internal goroutine. It should return a grpc status
// error. To decide it can use the given `info` to fetch additional data about
// the package instance.
type Handler func(ctx context.Context, req *protocol.Admission, info InstanceInfo) error

// InstanceInfo fetches additional information about a package instance being
// checked for admission by the handler.
type InstanceInfo interface {
	// VisitMetadata visits metadata entries attached to the package instance.
	//
	// Either visits all metadata or only entries with requested keys. Visits
	// entries in order of their registration time (the most recent first).
	// Fetches them in pages of `pageSize`. If `pageSize` is negative or zero,
	// uses some default size.
	//
	// Calls `cb` for each visited entry until all entries are successfully
	// visited or the callback returns false. Returns an error if the RPC to
	// the CIPD backend fails.
	VisitMetadata(ctx context.Context, keys []string, pageSize int, cb func(md *repopb.InstanceMetadata) bool) error
}

// Run executes the run loop of an admission plugin.
//
// It connects to the host and starts handling admission checks (each in an
// individual goroutine) by calling the handler.
//
// Blocks until the stdin closes (which indicates the plugin should terminate).
func Run(ctx context.Context, stdin io.ReadCloser, version string, handler Handler) error {
	return plugins.Run(ctx, stdin, func(ctx context.Context, conn *grpc.ClientConn) error {
		srv := protocol.NewAdmissionsClient(conn)

		stream, err := srv.ListAdmissions(ctx, &protocol.ListAdmissionsRequest{
			ProtocolVersion: ProtocolVersion,
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

				err = handler(ctx, admission, &infoImpl{
					host:      protocol.NewHostClient(conn),
					admission: admission,
				})
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

// infoImpl implements InstanceInfo.
type infoImpl struct {
	host      protocol.HostClient
	admission *protocol.Admission
}

func (v *infoImpl) VisitMetadata(ctx context.Context, keys []string, pageSize int, cb func(md *repopb.InstanceMetadata) bool) error {
	if pageSize <= 0 {
		pageSize = 20
	}
	pageToken := ""

	for {
		resp, err := v.host.ListMetadata(ctx, &protocol.ListMetadataRequest{
			ServiceUrl: v.admission.ServiceUrl,
			Package:    v.admission.Package,
			Instance:   v.admission.Instance,
			Keys:       keys,
			PageSize:   int32(pageSize),
			PageToken:  pageToken,
		})
		if err != nil {
			return err
		}
		for _, md := range resp.Metadata {
			if !cb(md) {
				return nil
			}
		}
		if resp.NextPageToken == "" {
			return nil
		}
		pageToken = resp.NextPageToken
	}
}
