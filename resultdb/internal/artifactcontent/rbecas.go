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

package artifactcontent

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

// RBEConn creates a gRPC connection to RBE authenticated as self.
func RBEConn(ctx context.Context) (*grpc.ClientConn, error) {
	creds, err := auth.GetPerRPCCredentials(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, err
	}

	return grpc.Dial(
		"remotebuildexecution.googleapis.com:443",
		grpc.WithTransportCredentials(credentials.NewTLS(nil)),
		grpc.WithPerRPCCredentials(creds),
	)
}

// RegisterRBEInstanceFlag registers -artifact-rbe-instance flag.
func RegisterRBEInstanceFlag(fs *flag.FlagSet, target *string) {
	fs.StringVar(
		target,
		"artifact-rbe-instance",
		"",
		"Name of the RBE instance to use for artifact storage",
	)
}

// handleRBECASContent serves artifact content stored in RBE-CAS.
func (r *contentRequest) handleRBECASContent(c *router.Context, hash string) {
	// Protocol:
	// https://github.com/bazelbuild/remote-apis/blob/7802003e00901b4e740fe0ebec1243c221e02ae2/build/bazel/remote/execution/v2/remote_execution.proto#L229-L233
	// https://github.com/googleapis/googleapis/blob/c8e291e6a4d60771219205b653715d5aeec3e96b/google/bytestream/bytestream.proto#L50-L53

	// Start a reading stream.
	stream, err := r.ReadCASBlob(c.Context, &bytestream.ReadRequest{
		ResourceName: fmt.Sprintf("%s/blobs/%s/%d", r.RBECASInstanceName, strings.TrimPrefix(hash, "sha256:"), r.size.Int64),
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// Do not lose the original error message.
			logging.Warningf(c.Context, "RBE-CAS responded: %s", err)
			err = appstatus.Errorf(codes.NotFound, "artifact content no longer exists")
		}
		r.sendError(c.Context, err)
		return
	}

	// Forward the blob to the client.
	wroteHeader := false
	for {
		_, readSpan := trace.StartSpan(c.Context, "resultdb.readChunk")
		chunk, err := stream.Recv()
		if err == nil {
			readSpan.Attribute("size", len(chunk.Data))
		}
		readSpan.End(err)

		switch {
		case err == io.EOF:
			// We are done.
			return

		case err != nil:
			if wroteHeader {
				// The response was already partially written, so it is too late to
				// write headers. Write at least something indicating the incomplete
				// response.
				fmt.Fprintf(c.Writer, "\nResultDB: internal error while writing the response!\n")
				logging.Errorf(c.Context, "Failed to read from RBE-CAS in the middle of response: %s", err)
			} else {
				r.sendError(c.Context, err)
			}
			return

		default:
			// Forward the chunk.
			if !wroteHeader {
				r.writeContentHeaders()
				wroteHeader = true
			}

			_, writeSpan := trace.StartSpan(c.Context, "resultdb.writeChunk")
			writeSpan.Attribute("size", len(chunk.Data))
			_, err := c.Writer.Write(chunk.Data)
			writeSpan.End(err)
			if err != nil {
				logging.Warningf(c.Context, "Failed to write a response chunk: %s", err)
				return
			}
		}
	}
}
