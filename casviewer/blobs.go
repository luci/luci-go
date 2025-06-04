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
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/templates"
)

// renderTree renders a Directory.
func renderTree(ctx context.Context, w http.ResponseWriter, cl *client.Client, bd *digest.Digest, instance string) error {
	b, err := readBlob(ctx, cl, bd)
	if err != nil {
		return err
	}

	d := &repb.Directory{}
	err = proto.Unmarshal(b, d)
	if err != nil {
		return grpcutil.InvalidArgumentTag.Apply(errors.Fmt("blob must be directory: %w", err))
	}

	templates.MustRender(ctx, w, "pages/tree.html", templates.Args{
		"Instance":    instance,
		"Digest":      bd.String(),
		"Directories": d.GetDirectories(),
		"Files":       d.GetFiles(),
		"Symlinks":    d.GetSymlinks(),
	})

	return nil
}

// returnBlob writes a blob to response.
func returnBlob(ctx context.Context, w http.ResponseWriter, cl *client.Client, bd *digest.Digest, filename string) error {
	b, err := readBlob(ctx, cl, bd)
	if err != nil {
		return err
	}

	// Set "text/plan" for the browser to detect the file type and display it.
	// - Images will be wrapped in <image>.
	// - Texts will be wrapped in <pre>.
	// - Videos/Audios will be wrapped in <video>.
	// etc.
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")

	// Set the filename for download.
	if filename != "" {
		w.Header().Add("Content-Disposition", fmt.Sprintf("filename=%s", filename))
	}

	_, err = w.Write(b)
	if err != nil {
		return err
	}

	return nil
}

// readBlob reads the blob from CAS.
func readBlob(ctx context.Context, cl *client.Client, bd *digest.Digest) ([]byte, error) {
	b, _, err := cl.ReadBlob(ctx, *bd)
	if err != nil {
		// convert gRPC code to LUCI errors tag.
		t := grpcutil.Tag.WithDefault(status.Code(err))
		return nil, t.Apply(errors.Fmt("failed to read blob: %w", err))
	}
	return b, nil
}

// treeURL renders a URL to the tree page.
func treeURL(instance string, d *repb.DirectoryNode) string {
	return fmt.Sprintf("/%s/blobs/%s/%d/tree", instance, d.Digest.Hash, d.Digest.SizeBytes)
}

// getURL renders a URL to the tree page.
func getURL(instance string, f *repb.FileNode) string {
	return fmt.Sprintf("/%s/blobs/%s/%d?filename=%s", instance, f.Digest.Hash, f.Digest.SizeBytes, f.Name)
}
