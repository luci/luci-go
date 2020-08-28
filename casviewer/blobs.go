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
	"net/http"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"
)

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

func getHandler(c *router.Context) {
	b, err := readBlob(c)
	if err != nil {
		// Error response/logs have already been written in readBlob.
		return
	}

	// TODO(crbug.com/1121471): append appropriate headers.
	c.Writer.Write(b)
}

func readBlob(c *router.Context) ([]byte, error) {
	bd, err := blobDigest(c.Params)
	if err != nil {
		renderBadRequest(c, "Digest size must be number")
		return nil, err
	}
	cl, err := GetClient(c.Context, fullInstName(c.Params))
	code := status.Code(err)
	if err != nil {
		logging.Errorf(c.Context, "failed to get CAS client: %s", err)
		renderInternalServerError(c, code.String())
		return nil, err
	}
	b, err := cl.ReadBlob(c.Context, *bd)
	code = status.Code(err)
	if code == codes.NotFound {
		renderNotFound(c)
		return nil, err
	}
	if code == codes.InvalidArgument {
		renderBadRequest(c, "Invalid blob digest")
		return nil, err
	}
	if err != nil {
		logging.Errorf(c.Context, "failed to read blob: %s", err)
		renderInternalServerError(c, code.String())
		return nil, err
	}
	return b, nil
}

func renderDirectory(c *router.Context, d *repb.Directory) {
	// TODO(crbug.com/1121471): render html.

	dirs := d.GetDirectories()
	c.Writer.Write([]byte(fmt.Sprintf("dirs: %v\n", dirs)))
	files := d.GetFiles()
	c.Writer.Write([]byte(fmt.Sprintf("files: %v\n", files)))
	links := d.GetSymlinks()
	c.Writer.Write([]byte(fmt.Sprintf("symlinks %v\n", links)))
}

func renderNotFound(c *router.Context) {
	// TODO(crbug.com/1121471): render 400 html.
	http.Error(c.Writer, "Error: Not Found", http.StatusNotFound)
}

func renderBadRequest(c *router.Context, errMsg string) {
	// TODO(crbug.com/1121471): render 404 html.
	m := fmt.Sprintf("Error: Bad Request. %s", errMsg)
	http.Error(c.Writer, m, http.StatusBadRequest)
}

func renderInternalServerError(c *router.Context, errMsg string) {
	// TODO(crbug.com/1121471): render 500 html.
	m := fmt.Sprintf("Error: %s", errMsg)
	http.Error(c.Writer, m, http.StatusInternalServerError)
}
