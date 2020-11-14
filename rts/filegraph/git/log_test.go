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

package git

import (
	"bufio"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLogReader(t *testing.T) {
	Convey(`LogReader`, t, func() {

		read := func(log string) []commit {
			r := &logReader{r: bufio.NewReader(strings.NewReader(log))}
			r.sep = '|'
			var commits []commit
			err := r.ReadCommits(func(c commit) error {
				commits = append(commits, c)
				return nil
			})
			So(err, ShouldBeNil)
			return commits
		}

		Convey(`one commit`, func() {
			parseOneCommit := func(log string) commit {
				commits := read(log)
				So(commits, ShouldHaveLength, 1)
				return commits[0]
			}

			Convey(`M`, func() {
				actual := parseOneCommit(`a3dcd10d73c46ea826785d03b7aa35e294d0f12a 91b7cf4d4e8b259f7657b6149e3393b166a7aaee
:100644 100644 8150c0c9b 8f04adce2 M|path/to/file||`)
				So(actual, ShouldResemble, commit{
					Hash:         "a3dcd10d73c46ea826785d03b7aa35e294d0f12a",
					ParentHashes: []string{"91b7cf4d4e8b259f7657b6149e3393b166a7aaee"},
					Files: []fileChange{
						{
							Status: 'M',
							Path:   "path/to/file",
						},
					},
				})
			})

			Convey(`M, M`, func() {
				actual := parseOneCommit(`a3dcd10d73c46ea826785d03b7aa35e294d0f12a 91b7cf4d4e8b259f7657b6149e3393b166a7aaee
:100644 100644 8150c0c9b 8f04adce2 M|path/to/file|:100644 100644 8150c0c9b 8f04adce2 M|path/to/file2||`)

				So(actual, ShouldResemble, commit{
					Hash:         "a3dcd10d73c46ea826785d03b7aa35e294d0f12a",
					ParentHashes: []string{"91b7cf4d4e8b259f7657b6149e3393b166a7aaee"},
					Files: []fileChange{
						{
							Status: 'M',
							Path:   "path/to/file",
						},
						{
							Status: 'M',
							Path:   "path/to/file2",
						},
					},
				})
			})

			Convey(`C`, func() {
				actual := parseOneCommit(`a3dcd10d73c46ea826785d03b7aa35e294d0f12a 91b7cf4d4e8b259f7657b6149e3393b166a7aaee
:100644 100644 8150c0c9b 8f04adce2 C|path/to/file|path/to/file2||`)
				So(actual, ShouldResemble, commit{
					Hash:         "a3dcd10d73c46ea826785d03b7aa35e294d0f12a",
					ParentHashes: []string{"91b7cf4d4e8b259f7657b6149e3393b166a7aaee"},
					Files: []fileChange{
						{
							Status: 'C',
							Path:   "path/to/file",
							Path2:  "path/to/file2",
						},
					},
				})
			})

			Convey(`R50`, func() {
				actual := parseOneCommit(`a3dcd10d73c46ea826785d03b7aa35e294d0f12a 91b7cf4d4e8b259f7657b6149e3393b166a7aaee
:100644 100644 8150c0c9b 8f04adce2 R50|path/to/file|path/to/file2||`)
				So(actual, ShouldResemble, commit{
					Hash:         "a3dcd10d73c46ea826785d03b7aa35e294d0f12a",
					ParentHashes: []string{"91b7cf4d4e8b259f7657b6149e3393b166a7aaee"},
					Files: []fileChange{
						{
							Status: 'R',
							Path:   "path/to/file",
							Path2:  "path/to/file2",
						},
					},
				})
			})

			Convey(`two parents`, func() {
				actual := parseOneCommit(`a3dcd10d73c46ea826785d03b7aa35e294d0f12a 91b7cf4d4e8b259f7657b6149e3393b166a7aaee 3a89a841f9d213cc273af75f085f52c2597a63d2
:100644 100644 8150c0c9b 8f04adce2 M|path/to/file||`)
				So(actual, ShouldResemble, commit{
					Hash:         "a3dcd10d73c46ea826785d03b7aa35e294d0f12a",
					ParentHashes: []string{"91b7cf4d4e8b259f7657b6149e3393b166a7aaee", "3a89a841f9d213cc273af75f085f52c2597a63d2"},
					Files: []fileChange{
						{
							Status: 'M',
							Path:   "path/to/file",
						},
					},
				})
			})

		})

		Convey(`two commits`, func() {
			Convey(`M; M`, func() {
				actual := read(`a3dcd10d73c46ea826785d03b7aa35e294d0f12a 91b7cf4d4e8b259f7657b6149e3393b166a7aaee
:100644 100644 8150c0c9b 8f04adce2 M|path/to/file||3a89a841f9d213cc273af75f085f52c2597a63d2 5376d1941a55c6481fc568004d4836ffc2c332b9
:100644 100644 8150c0c9b 8f04adce2 M|path/to/file2||`)
				So(actual, ShouldResemble, []commit{
					{
						Hash:         "a3dcd10d73c46ea826785d03b7aa35e294d0f12a",
						ParentHashes: []string{"91b7cf4d4e8b259f7657b6149e3393b166a7aaee"},
						Files: []fileChange{
							{
								Status: 'M',
								Path:   "path/to/file",
							},
						},
					},
					{
						Hash:         "3a89a841f9d213cc273af75f085f52c2597a63d2",
						ParentHashes: []string{"5376d1941a55c6481fc568004d4836ffc2c332b9"},
						Files: []fileChange{
							{
								Status: 'M',
								Path:   "path/to/file2",
							},
						},
					},
				})
			})
		})

		Convey(`empty commit`, func() {
			actual := read(`f71d633d037422e101edb573038b8e281f283d16 |2f1870f85324ed520077653a2c8a881e9052d96c f71d633d037422e101edb573038b8e281f283d16
:100644 100644 8150c0c9b 8f04adce2 M|path/to/file2||`)
			So(actual, ShouldResemble, []commit{
				{
					Hash: "f71d633d037422e101edb573038b8e281f283d16",
				},
				{
					Hash:         "2f1870f85324ed520077653a2c8a881e9052d96c",
					ParentHashes: []string{"f71d633d037422e101edb573038b8e281f283d16"},
					Files: []fileChange{
						{
							Status: 'M',
							Path:   "path/to/file2",
						},
					},
				},
			})
		})

		Convey(`orphan commit`, func() {
			// This string is split into two because some editors trim trailing
			// whitespace, even if it is a part of a string literal.
			actual := read("f71d633d037422e101edb573038b8e281f283d16 " + `
:100644 100644 8150c0c9b 8f04adce2 M|path/to/file||2f1870f85324ed520077653a2c8a881e9052d96c f71d633d037422e101edb573038b8e281f283d16
:100644 100644 8150c0c9b 8f04adce2 M|path/to/file2||`)
			So(actual, ShouldResemble, []commit{
				{
					Hash: "f71d633d037422e101edb573038b8e281f283d16",
					Files: []fileChange{
						{
							Status: 'M',
							Path:   "path/to/file",
						},
					},
				},
				{
					Hash:         "2f1870f85324ed520077653a2c8a881e9052d96c",
					ParentHashes: []string{"f71d633d037422e101edb573038b8e281f283d16"},
					Files: []fileChange{
						{
							Status: 'M',
							Path:   "path/to/file2",
						},
					},
				},
			})
		})

	})
}
