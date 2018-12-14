import "testing"

// Copyright 2018 The LUCI Authors.
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

package serviceaccounts

import (
	"context"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectscope"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLookupProjectScopedServiceAccount(t *testing.T) {

}