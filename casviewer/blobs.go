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
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
)

// renderTree renders a Directory.
func renderTree(ctx context.Context, w http.ResponseWriter, cl *client.Client, bd *digest.Digest) error {
	b, err := readBlob(ctx, cl, bd)
	if err != nil {
		return err
	}

	d := &repb.Directory{}
	err = proto.Unmarshal(b, d)
	if err != nil {
		return errors.Annotate(err, "blob must be directory").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	// TODO(crbug.com/1121471): render html.

	dirs := d.GetDirectories()
	_, err = w.Write([]byte(fmt.Sprintf("dirs: %v\n", dirs)))
	if err != nil {
		return err
	}
	files := d.GetFiles()
	_, err = w.Write([]byte(fmt.Sprintf("files: %v\n", files)))
	if err != nil {
		return err
	}
	links := d.GetSymlinks()
	_, err = w.Write([]byte(fmt.Sprintf("symlinks: %v\n", links)))
	if err != nil {
		return err
	}

	return nil
}

// returnBlob writes a blob to response.
func returnBlob(ctx context.Context, w http.ResponseWriter, cl *client.Client, bd *digest.Digest) error {
	b, err := readBlob(ctx, cl, bd)
	if err != nil {
		return err
	}

	// TODO(crbug.com/1121471): append appropriate headers.

	_, err = w.Write(b)
	if err != nil {
		return err
	}

	return nil
}

// readBlob reads the blob from CAS.
func readBlob(ctx context.Context, cl *client.Client, bd *digest.Digest) ([]byte, error) {
	b, err := cl.ReadBlob(ctx, *bd)
	if err != nil {
		// convert gRPC code to LUCI errors tag.
		t := grpcutil.Tag.With(status.Code(err))
		return nil, errors.Annotate(err, "failed to read blob").Tag(t).Err()
	}
	return b, nil
}
