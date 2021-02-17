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

package secrets

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFileSource(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Works", t, func(c C) {
		tempDir, err := os.MkdirTemp("", "file_secret_test")
		c.So(err, ShouldBeNil)
		c.Reset(func() { os.RemoveAll(tempDir) })

		read := func(body string) (*Secret, error) {
			path := filepath.Join(tempDir, "secret.json")
			So(os.WriteFile(path, []byte(body), 0600), ShouldBeNil)
			So(err, ShouldBeNil)
			return (&FileSource{Path: path}).ReadSecret(ctx)
		}

		Convey("OK", func() {
			s, err := read(fmt.Sprintf(`{
				"current": "%s",
				"previous": ["%s", "%s"]
			}`,
				base64.StdEncoding.EncodeToString([]byte{1, 2, 3}),
				base64.StdEncoding.EncodeToString([]byte{4, 5, 6}),
				base64.StdEncoding.EncodeToString([]byte{7, 8, 9}),
			))
			So(err, ShouldBeNil)
			So(s, ShouldResemble, &Secret{
				Current: []byte{1, 2, 3},
				Previous: [][]byte{
					{4, 5, 6},
					{7, 8, 9},
				},
			})
		})

		Convey("Missing", func() {
			_, err := (&FileSource{Path: filepath.Join(tempDir, "missing.json")}).ReadSecret(ctx)
			So(err, ShouldNotBeNil)
		})

		Convey("Broken", func() {
			_, err := read("not a json")
			So(err, ShouldNotBeNil)
		})

		Convey("No 'current'", func() {
			_, err := read("{}")
			So(err, ShouldNotBeNil)
		})
	})
}
