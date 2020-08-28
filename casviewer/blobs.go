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

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"
)

func treeHandler(c *router.Context) {
	bd, err := blobDigest(c.Params)
	if err != nil {
		http.Error(c.Writer, "Invalid digest size", http.StatusUnprocessableEntity)
		return
	}
	cl, err := GetClient(c.Context, fullInstName(c.Params))
	if err != nil {
		errMsg := "failed to initialize CAS client"
		logging.Errorf(c.Context, "%s: %s", errMsg, err)
		http.Error(c.Writer, errMsg, http.StatusInternalServerError)
		return
	}
	b, err := cl.ReadBlob(c.Context, *bd)
	if err != nil {
		errMsg := "failed to read blob"
		logging.Errorf(c.Context, "%s: %s", errMsg, err)
		http.Error(c.Writer, errMsg, http.StatusInternalServerError)
		return
	}
	if b == nil {
		http.Error(c.Writer, "Not Found", http.StatusNotFound)
		return
	}

	d := &repb.Directory{}
	if err = proto.Unmarshal(b, d); err != nil {
		http.Error(c.Writer, "Please specify directory node.", http.StatusBadRequest)
		return
	}

	renderDirectory(c, d)
}

func getHandler(c *router.Context) {
	bd, err := blobDigest(c.Params)
	if err != nil {
		http.Error(c.Writer, "Invalid digest size", http.StatusUnprocessableEntity)
		return
	}
	cl, err := GetClient(c.Context, fullInstName(c.Params))
	if err != nil {
		errMsg := "failed to initialize CAS client"
		logging.Errorf(c.Context, "%s: %s", errMsg, err)
		http.Error(c.Writer, errMsg, http.StatusInternalServerError)
		return
	}
	b, err := cl.ReadBlob(c.Context, *bd)
	if err != nil {
		errMsg := "failed to read blob"
		logging.Errorf(c.Context, "%s: %s", errMsg, err)
		http.Error(c.Writer, errMsg, http.StatusInternalServerError)
		return
	}

	c.Writer.Write(b)
}

func renderDirectory(c *router.Context, d *repb.Directory) {
	// TODO(crbug.com/1121471): render html.
	dirs := d.GetDirectories()
	c.Writer.Write([]byte(fmt.Sprintf("dirs: %v", dirs)))
	files := d.GetFiles()
	c.Writer.Write([]byte(fmt.Sprintf("files: %v", files)))

	links := d.GetSymlinks()
	c.Writer.Write([]byte(fmt.Sprintf("symlinks %v", links)))
}
