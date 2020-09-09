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
	"net/http"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/templates"
)

// renderErrorPage renders an appropriate error page.
func renderErrorPage(ctx context.Context, w http.ResponseWriter, err error) {
	errors.Log(ctx, err)
	grpcCode := grpcutil.Code(err)
	statusCode := grpcutil.CodeStatus(grpcCode)
	w.WriteHeader(statusCode)
	templates.MustRender(ctx, w, "pages/error.html", templates.Args{
		"StatusCode":    statusCode,
		"StatusMessage": grpcCode.String(),
		"ErrorMessage":  err.Error(),
	})
}
