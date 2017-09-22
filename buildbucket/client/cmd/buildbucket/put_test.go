// Copyright 2017 The LUCI Authors.
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

package main

import (
	"testing"

	"go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPut(t *testing.T) {
	t.Parallel()

	Convey("Put", t, func() {
		parse := func(args ...string) ([]*buildbucket.ApiPutRequestMessage, error) {
			return parsePutRequest(args, func() (string, error) {
				return "prefix", nil
			})
		}

		_, err := parse()
		So(err, ShouldErrLike, "missing parameter")

		reqs, err := parse(`{"bucket": "master.tryserver.chromium.linux"}`)
		So(err, ShouldBeNil)
		So(reqs, ShouldResemble, []*buildbucket.ApiPutRequestMessage{
			{
				Bucket:            "master.tryserver.chromium.linux",
				ClientOperationId: "prefix-0",
			},
		})
	})
}
