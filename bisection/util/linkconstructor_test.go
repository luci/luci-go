// Copyright 2022 The LUCI Authors.
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

package util

import (
	"context"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/info"
)

func TestConstructAnalysisURL(t *testing.T) {
	ctx := memory.Use(context.Background())
	appID := info.AppID(ctx)

	Convey("construct analysis URL", t, func() {
		So(ConstructAnalysisURL(ctx, 123456789876543), ShouldEqual,
			fmt.Sprintf("https://%s.appspot.com/analysis/b/123456789876543", appID))
	})
}

func TestConstructBuildURL(t *testing.T) {
	ctx := context.Background()

	Convey("construct build URL", t, func() {
		So(ConstructBuildURL(ctx, 123456789876543), ShouldEqual,
			"https://ci.chromium.org/b/123456789876543")
	})
}
