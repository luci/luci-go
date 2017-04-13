// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package filesystem

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/luci/luci-go/common/errors"

	. "github.com/smartystreets/goconvey/convey"
)

func withTempDir(t *testing.T, fn func(string)) {
	tdir, err := ioutil.TempDir("", "vpython-filesystem")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %s", err)
	}
	defer func() {
		// Mark all files writable.
		rmErr := recursiveChmod(tdir, nil, func(mode os.FileMode) os.FileMode {
			return mode | 0222
		})
		if rmErr != nil {
			t.Errorf("failed to mark temporary directory as writable [%s]: %s", tdir, rmErr)
		}
		if rmErr := os.RemoveAll(tdir); rmErr != nil {
			t.Errorf("failed to remove temporary directory [%s]: %s", tdir, rmErr)
		}
	}()
	fn(tdir)
}

func TestIsNotExist(t *testing.T) {
	t.Parallel()

	// Create a temporary directory so we control its contents. This will let us
	// safely create an IsNotExist error.
	withTempDir(t, func(tdir string) {
		dnePath := filepath.Join(tdir, "anything")
		_, err := os.Open(dnePath)
		if !os.IsNotExist(err) {
			t.Fatalf("failed to get IsNotExist error: %s", err)
		}

		Convey(`IsNotExist works`, t, func() {
			So(IsNotExist(nil), ShouldBeFalse)
			So(IsNotExist(errors.New("something")), ShouldBeFalse)

			So(IsNotExist(err), ShouldBeTrue)
			So(IsNotExist(errors.Annotate(err).Reason("annotated").Err()), ShouldBeTrue)
		})
	})
}

func TestAbsPath(t *testing.T) {
	t.Parallel()

	// Create a temporary directory so we control its contents. This will let us
	// safely create an IsNotExist error.
	withTempDir(t, func(tdir string) {
		tdirName := filepath.Base(tdir)

		Convey(`AbsPath works`, t, func() {
			base := filepath.Join(tdir, "..", tdirName, "file.txt")
			So(AbsPath(&base), ShouldBeNil)
			So(base, ShouldEqual, filepath.Join(tdir, "file.txt"))
		})
	})
}

func TestTouch(t *testing.T) {
	t.Parallel()

	Convey(`Testing Touch`, t, func() {
		withTempDir(t, func(tdir string) {
			thePast := time.Now().Add(-10 * time.Second)

			Convey(`Can update a directory timestamp`, func() {
				path := filepath.Join(tdir, "subdir")

				So(os.Mkdir(path, 0755), ShouldBeNil)
				st, err := os.Lstat(path)
				So(err, ShouldBeNil)
				initialModTime := st.ModTime()

				So(Touch(path, thePast, 0), ShouldBeNil)
				st, err = os.Lstat(path)
				So(err, ShouldBeNil)

				So(st.ModTime(), ShouldHappenBefore, initialModTime)
			})

			Convey(`Can update an empty file timestamp`, func() {
				path := filepath.Join(tdir, "touch")

				So(Touch(path, time.Time{}, 0644), ShouldBeNil)
				st, err := os.Lstat(path)
				So(err, ShouldBeNil)
				initialModTime := st.ModTime()

				So(Touch(path, thePast, 0), ShouldBeNil)
				st, err = os.Lstat(path)
				So(err, ShouldBeNil)
				pastModTime := st.ModTime()
				So(pastModTime, ShouldHappenBefore, initialModTime)

				// Touch back to "now".
				So(Touch(path, time.Time{}, 0644), ShouldBeNil)
				st, err = os.Lstat(path)
				So(err, ShouldBeNil)
				So(st.ModTime(), ShouldHappenOnOrAfter, pastModTime)
			})

			Convey(`Can update a populated file timestamp`, func() {
				path := filepath.Join(tdir, "touch")

				So(ioutil.WriteFile(path, []byte("sup"), 0644), ShouldBeNil)
				st, err := os.Lstat(path)
				So(err, ShouldBeNil)
				initialModTime := st.ModTime()

				So(Touch(path, thePast, 0), ShouldBeNil)
				st, err = os.Lstat(path)
				So(err, ShouldBeNil)

				So(st.ModTime(), ShouldHappenBefore, initialModTime)

				content, err := ioutil.ReadFile(path)
				So(err, ShouldBeNil)
				So(content, ShouldResemble, []byte("sup"))
			})
		})
	})
}

func TestMakeReadOnly(t *testing.T) {
	t.Parallel()

	Convey(`Can RemoveAll`, t, func() {
		withTempDir(t, func(tdir string) {

			for _, path := range []string{
				filepath.Join(tdir, "foo", "bar"),
				filepath.Join(tdir, "foo", "baz"),
				filepath.Join(tdir, "qux"),
			} {
				base := filepath.Dir(path)
				if err := os.MkdirAll(base, 0755); err != nil {
					t.Fatalf("failed to populate directory [%s]: %s", base, err)
				}
				if err := ioutil.WriteFile(path, []byte("junk"), 0644); err != nil {
					t.Fatalf("failed to create file [%s]: %s", path, err)
				}
			}

			Convey(`Can mark the entire subdirectory read-only`, func() {
				So(MakeReadOnly(tdir, nil), ShouldBeNil)
				filepath.Walk(tdir, func(path string, info os.FileInfo, err error) error {
					So(err, ShouldBeNil)
					So(info.Mode()&0222, ShouldEqual, 0)
					return nil
				})
			})

			Convey(`Can selectively mark files read-only`, func() {
				// Don't mark <TMP>/foo read-only.
				fooPath := filepath.Join(tdir, "foo")

				So(MakeReadOnly(tdir, func(path string) bool {
					return path != fooPath
				}), ShouldBeNil)

				filepath.Walk(tdir, func(path string, info os.FileInfo, err error) error {
					So(err, ShouldBeNil)
					if path == fooPath {
						So(info.Mode()&0222, ShouldNotEqual, 0)
					} else {
						So(info.Mode()&0222, ShouldEqual, 0)
					}
					return nil
				})
			})
		})
	})
}

func TestRemoveAll(t *testing.T) {
	t.Parallel()

	isGone := func(path string) bool {
		if _, err := os.Lstat(path); err != nil {
			return os.IsNotExist(err)
		}
		return false
	}

	Convey(`Can RemoveAll`, t, func() {
		withTempDir(t, func(tdir string) {

			subDir := filepath.Join(tdir, "sub")

			Convey(`With directory contents...`, func() {
				for _, path := range []string{
					filepath.Join(subDir, "foo", "bar"),
					filepath.Join(subDir, "foo", "baz"),
					filepath.Join(subDir, "qux"),
				} {
					base := filepath.Dir(path)
					if err := os.MkdirAll(base, 0755); err != nil {
						t.Fatalf("failed to populate directory [%s]: %s", base, err)
					}
					if err := ioutil.WriteFile(path, []byte("junk"), 0644); err != nil {
						t.Fatalf("failed to create file [%s]: %s", path, err)
					}
				}

				Convey(`Can remove the directory`, func() {
					So(RemoveAll(subDir), ShouldBeNil)
					So(isGone(subDir), ShouldBeTrue)
				})

				Convey(`Can remove the directory when it is read-only`, func() {
					// Make the directory read-only, and assert that classic os.RemoveAll
					// fails.
					So(MakeReadOnly(subDir, nil), ShouldBeNil)
					So(os.RemoveAll(subDir), ShouldNotBeNil)

					So(RemoveAll(subDir), ShouldBeNil)
					So(isGone(subDir), ShouldBeTrue)
				})
			})

			Convey(`Will return nil if the target does not exist`, func() {
				So(RemoveAll(filepath.Join(tdir, "dne")), ShouldBeNil)
			})
		})
	})
}
