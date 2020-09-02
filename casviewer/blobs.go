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

package casviewer

import (
	"context"
	"fmt"
	"net/http"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
)

// treeHandler renders a Directory, or renders an error page.
func renderTree(ctx context.Context, w http.ResponseWriter, cl *client.Client, bd *digest.Digest) {
	b, err := readBlob(ctx, w, cl, bd)
	if err != nil {
		// Error response have already been written in readBlob.
		return
	}

	d := &repb.Directory{}
	err = proto.Unmarshal(b, d)
	if err != nil {
		renderBadRequest(ctx, w, "Blob must be Directory")
		return
	}

	// TODO(crbug.com/1121471): render html.

	dirs := d.GetDirectories()
	w.Write([]byte(fmt.Sprintf("dirs: %v\n", dirs)))
	files := d.GetFiles()
	w.Write([]byte(fmt.Sprintf("files: %v\n", files)))
	links := d.GetSymlinks()
	w.Write([]byte(fmt.Sprintf("symlinks: %v\n", links)))
}

// returnBlob writes a blob to response, or renders an error page.
func returnBlob(ctx context.Context, w http.ResponseWriter, cl *client.Client, bd *digest.Digest) {
	b, err := readBlob(ctx, w, cl, bd)
	if err != nil {
		// Error response have already been written in readBlob.
		return
	}

	// TODO(crbug.com/1121471): append appropriate headers.
	w.Write(b)
}

// readBlob returns a blob from CAS, or renders an error page.
func readBlob(ctx context.Context, w http.ResponseWriter, cl *client.Client, bd *digest.Digest) ([]byte, error) {
	b, err := cl.ReadBlob(ctx, *bd)
	if err != nil {
		// render an appropriate error page.
		switch code := status.Code(err); code {
		case codes.NotFound:
			renderNotFound(ctx, w)
		case codes.InvalidArgument:
			renderBadRequest(ctx, w, "Invalid blob digest")
		default:
			logging.Errorf(ctx, "failed to read blob: %s", err)
			renderInternalServerError(ctx, w, code.String())
		}
		return nil, err
	}
	return b, nil
}
