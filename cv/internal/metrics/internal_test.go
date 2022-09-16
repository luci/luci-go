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

package metrics

import (
	"fmt"
	"reflect"
	"testing"

	"go.chromium.org/luci/common/tsmon/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInternalMetrics(t *testing.T) {
	t.Parallel()

	const internalPrefix = "cv/internal"
	Convey(fmt.Sprintf("Internal metrics should start with %s", internalPrefix), t, func() {
		v := reflect.ValueOf(Internal)
		for i := 0; i < v.NumField(); i++ {
			m := v.Field(i).Interface().(types.Metric)
			So(m.Info().Name, ShouldStartWith, internalPrefix)
		}
	})
}
