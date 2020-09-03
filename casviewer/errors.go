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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
)

// renderErrorPage renders an appropriate error page.
func renderErrorPage(ctx context.Context, w http.ResponseWriter, err error) {
	switch code := status.Code(err); code {
	case codes.NotFound:
		renderNotFound(ctx, w)
	case codes.InvalidArgument:
		renderBadRequest(ctx, w, "Invalid blob digest")
	default:
		logging.Errorf(ctx, "failed to read blob: %s", err)
		renderInternalServerError(ctx, w, code.String())
	}
}

func renderForbidden(ctx context.Context, w http.ResponseWriter) {
	http.Error(w, "Error: Permission Denied", http.StatusForbidden)
}

// renderBadRequest renders 400 BadRequest page.
func renderBadRequest(ctx context.Context, w http.ResponseWriter, errMsg string) {
	// TODO(crbug.com/1121471): render 400 html.
	m := fmt.Sprintf("Error: Bad Request. %s", errMsg)
	http.Error(w, m, http.StatusBadRequest)
}

// renderNotFound renders 404 NotFound page.
func renderNotFound(ctx context.Context, w http.ResponseWriter) {
	// TODO(crbug.com/1121471): render 404 html.
	http.Error(w, "Error: Not Found", http.StatusNotFound)
}

// renderInternalServerError renders 500 InternalServerError page.
func renderInternalServerError(ctx context.Context, w http.ResponseWriter, errMsg string) {
	// TODO(crbug.com/1121471): render 500 html.
	m := fmt.Sprintf("Error: %s", errMsg)
	http.Error(w, m, http.StatusInternalServerError)
}
