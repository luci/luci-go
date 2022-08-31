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

package model

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSuspectJutification(t *testing.T) {
	t.Parallel()

	Convey("SuspectJutification", t, func() {
		justification := &SuspectJustification{}
		justification.AddItem(10, "a/b", "fileInLog", JustificationType_FAILURELOG)
		So(justification.GetScore(), ShouldEqual, 10)
		justification.AddItem(2, "c/d", "fileInDependency1", JustificationType_DEPENDENCY)
		So(justification.GetScore(), ShouldEqual, 12)
		justification.AddItem(8, "e/f", "fileInDependency2", JustificationType_DEPENDENCY)
		So(justification.GetScore(), ShouldEqual, 19)
	})
}
