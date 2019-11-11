package main

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLogReader(t *testing.T) {
	Convey(`LogReader`, t, func() {

		read := func(log string) []commit {
			r := newLogReader(strings.NewReader(log))
			r.delim = '|'
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
				actual := parseOneCommit(`a3dcd10d73c46ea826785d03b7aa35e294d0f12a
:100644 100644 8150c0c9b 8f04adce2 M|path/to/file|1	2	path/to/file|`)
				So(actual, ShouldResemble, commit{
					Hash: "a3dcd10d73c46ea826785d03b7aa35e294d0f12a",
					Files: []*fileChange{
						{
							Status:  'M',
							Src:     Path{"path", "to", "file"},
							Added:   1,
							Deleted: 2,
						},
					},
				})
			})

			Convey(`M, M`, func() {
				actual := parseOneCommit(`a3dcd10d73c46ea826785d03b7aa35e294d0f12a
:100644 100644 8150c0c9b 8f04adce2 M|path/to/file|:100644 100644 8150c0c9b 8f04adce2 M|path/to/file2|1	2	path/to/file|3	4	path/to/file2|`)

				So(actual, ShouldResemble, commit{
					Hash: "a3dcd10d73c46ea826785d03b7aa35e294d0f12a",
					Files: []*fileChange{
						{
							Status:  'M',
							Src:     Path{"path", "to", "file"},
							Added:   1,
							Deleted: 2,
						},
						{
							Status:  'M',
							Src:     Path{"path", "to", "file2"},
							Added:   3,
							Deleted: 4,
						},
					},
				})
			})

			Convey(`C`, func() {
				actual := parseOneCommit(`a3dcd10d73c46ea826785d03b7aa35e294d0f12a
:100644 100644 8150c0c9b 8f04adce2 C|path/to/file|path/to/file2|1	2	|path/to/file|path/to/file2|`)
				So(actual, ShouldResemble, commit{
					Hash: "a3dcd10d73c46ea826785d03b7aa35e294d0f12a",
					Files: []*fileChange{
						{
							Status:  'C',
							Src:     Path{"path", "to", "file"},
							Dst:     Path{"path", "to", "file2"},
							Added:   1,
							Deleted: 2,
						},
					},
				})
			})

			Convey(`R50`, func() {
				actual := parseOneCommit(`a3dcd10d73c46ea826785d03b7aa35e294d0f12a
:100644 100644 8150c0c9b 8f04adce2 R50|path/to/file|path/to/file2|1	2	|path/to/file|path/to/file2|`)
				So(actual, ShouldResemble, commit{
					Hash: "a3dcd10d73c46ea826785d03b7aa35e294d0f12a",
					Files: []*fileChange{
						{
							Status:  'R',
							Score:   50,
							Src:     Path{"path", "to", "file"},
							Dst:     Path{"path", "to", "file2"},
							Added:   1,
							Deleted: 2,
						},
					},
				})
			})

		})

		Convey(`two commits`, func() {
			Convey(`M; M`, func() {
				actual := read(`a3dcd10d73c46ea826785d03b7aa35e294d0f12a
:100644 100644 8150c0c9b 8f04adce2 M|path/to/file|1	2	path/to/file||3a89a841f9d213cc273af75f085f52c2597a63d2
:100644 100644 8150c0c9b 8f04adce2 M|path/to/file2|3	4	path/to/file2|`)
				So(actual, ShouldResemble, []commit{
					{
						Hash: "a3dcd10d73c46ea826785d03b7aa35e294d0f12a",
						Files: []*fileChange{
							{
								Status:  'M',
								Src:     Path{"path", "to", "file"},
								Added:   1,
								Deleted: 2,
							},
						},
					},
					{
						Hash: "3a89a841f9d213cc273af75f085f52c2597a63d2",
						Files: []*fileChange{
							{
								Status:  'M',
								Src:     Path{"path", "to", "file2"},
								Added:   3,
								Deleted: 4,
							},
						},
					},
				})
			})
		})

		Convey(`empty commit`, func() {
			actual := read(`a3dcd10d73c46ea826785d03b7aa35e294d0f12a|3a89a841f9d213cc273af75f085f52c2597a63d2
:100644 100644 8150c0c9b 8f04adce2 M|path/to/file2|3	4	path/to/file2|`)
			So(actual, ShouldResemble, []commit{
				{
					Hash: "a3dcd10d73c46ea826785d03b7aa35e294d0f12a",
				},
				{
					Hash: "3a89a841f9d213cc273af75f085f52c2597a63d2",
					Files: []*fileChange{
						{
							Status:  'M',
							Src:     Path{"path", "to", "file2"},
							Added:   3,
							Deleted: 4,
						},
					},
				},
			})
		})
	})
}
