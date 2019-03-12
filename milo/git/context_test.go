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

package git

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestProjectContext(t *testing.T) {
	t.Parallel()

	Convey("Context annotation works", t, func() {
		ctx := context.Background()
		So(func() { ProjectFromContext(ctx) }, ShouldPanic)
		WithProject(ctx, "")
		So(func() { ProjectFromContext(ctx) }, ShouldPanic)
		ctx = WithProject(ctx, "luci-project")
		So(ProjectFromContext(ctx), ShouldResemble, "luci-project")
	})
}
