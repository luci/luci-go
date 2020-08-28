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
	"fmt"
	"strconv"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/golang/protobuf/proto"
	"github.com/julienschmidt/httprouter"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"
)

// getHandler renders the requested directory, or renders an error page.
func treeHandler(c *router.Context) {
	b, err := readBlob(c)
	if err != nil {
		// Error response/logs have already been written in readBlob.
		return
	}

	d := &repb.Directory{}
	err = proto.Unmarshal(b, d)
	if err != nil {
		renderBadRequest(c, "Blob must be Directory")
		return
	}

	renderDirectory(c, d)
}

// getHandler writes the requested blob to response, or renders an error page.
func getHandler(c *router.Context) {
	b, err := readBlob(c)
	if err != nil {
		// Error response/logs have already been written in readBlob.
		return
	}

	// TODO(crbug.com/1121471): append appropriate headers.
	c.Writer.Write(b)
}

// readBlob returns a blob from CAS, or renders an error page.
func readBlob(c *router.Context) ([]byte, error) {
	cl, err := GetClient(c.Context, fullInstName(c.Params))
	code := status.Code(err)
	if err != nil {
		logging.Errorf(c.Context, "failed to get CAS client: %s", err)
		renderInternalServerError(c, code.String())
		return nil, err
	}

	bd, err := blobDigest(c.Params)
	if err != nil {
		renderBadRequest(c, "Digest size must be number")
		return nil, err
	}
	b, err := cl.ReadBlob(c.Context, *bd)
	if err != nil {
		// render an appropriate page.
		switch code = status.Code(err); code {
		case codes.NotFound:
			renderNotFound(c)
		case codes.InvalidArgument:
			renderBadRequest(c, "Invalid blob digest")
		default:
			logging.Errorf(c.Context, "failed to read blob: %s", err)
			renderInternalServerError(c, code.String())
		}
		return nil, err
	}
	return b, nil
}

// fullInstName constructs full instance name from the URL parameters.
func fullInstName(p httprouter.Params) string {
	return fmt.Sprintf(
		"projects/%s/instances/%s", p.ByName("project"), p.ByName("instance"))
}

// blobDigest constructs a Digest from the URL parameters.
func blobDigest(p httprouter.Params) (*digest.Digest, error) {
	size, err := strconv.ParseInt(p.ByName("size"), 10, 0)
	if err != nil {
		return nil, err
	}

	d := &digest.Digest{
		Hash: p.ByName("hash"),
		Size: size,
	}
	return d, nil
}

// renderDirectory renders a directory page.
func renderDirectory(c *router.Context, d *repb.Directory) {
	// TODO(crbug.com/1121471): render html.

	dirs := d.GetDirectories()
	c.Writer.Write([]byte(fmt.Sprintf("dirs: %v\n", dirs)))
	files := d.GetFiles()
	c.Writer.Write([]byte(fmt.Sprintf("files: %v\n", files)))
	links := d.GetSymlinks()
	c.Writer.Write([]byte(fmt.Sprintf("symlinks %v\n", links)))
}
