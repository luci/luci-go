// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package lucictx

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/luci/luci-go/common/system/environ"
)

func rawctx(content string) func() {
	tf, err := ioutil.TempFile(os.TempDir(), "lucictx_test.")
	if err != nil {
		panic(err)
	}
	_, err = tf.WriteString(content)
	if err != nil {
		panic(err)
	}
	tf.Close()
	os.Setenv(EnvKey, tf.Name())

	return func() { os.Remove(tf.Name()) }
}

func TestLUCIContextInitialization(t *testing.T) {
	// t.Parallel() because of os.Environ manipulation

	Convey("extractFromEnv", t, func() {
		os.Unsetenv(EnvKey)
		buf := &bytes.Buffer{}

		Convey("works with missing envvar", func() {
			So(extractFromEnv(buf), ShouldBeNil)
			So(buf.String(), ShouldEqual, "")
			_, ok := os.LookupEnv(EnvKey)
			So(ok, ShouldBeFalse)
		})

		Convey("works with bad envvar", func() {
			os.Setenv(EnvKey, "sup")
			So(extractFromEnv(buf), ShouldBeNil)
			So(buf.String(), ShouldContainSubstring, "Could not open LUCI_CONTEXT file")
		})

		Convey("works with bad file", func() {
			defer rawctx(`"not a map"`)()

			So(extractFromEnv(buf), ShouldBeNil)
			So(buf.String(), ShouldContainSubstring, "cannot unmarshal string into Go value")
		})

		Convey("ignores bad sections", func() {
			defer rawctx(`{"hi": {"there": 10}, "not": "good"}`)()

			blob := json.RawMessage(`{"there":10}`)
			So(extractFromEnv(buf), ShouldResemble, lctx{
				"hi": &blob,
			})
			So(buf.String(), ShouldContainSubstring, `section "not": Not a map`)
		})
	})
}

func TestLUCIContextMethods(t *testing.T) {
	// t.Parallel() manipulates environ
	defer rawctx(`{"hi": {"there": 10}}`)()
	externalContext = extractFromEnv(nil)

	type Hi struct {
		There int `json:"there"`
	}

	Convey("lctx", t, func() {
		c := context.Background()

		Convey("Get/Set", func() {
			Convey("defaults to env values", func() {
				h := Hi{}
				So(Get(c, "hi", &h), ShouldBeNil)
				So(h, ShouldResemble, Hi{10})
			})

			Convey("nop for missing sections", func() {
				h := Hi{}
				So(Get(c, "wut", &h), ShouldBeNil)
				So(h, ShouldResemble, Hi{0})
			})

			Convey("can add section", func() {
				c, err := Set(c, "wut", &Hi{100})
				So(err, ShouldBeNil)

				h := Hi{}
				So(Get(c, "wut", &h), ShouldBeNil)
				So(h, ShouldResemble, Hi{100})

				So(Get(c, "hi", &h), ShouldBeNil)
				So(h, ShouldResemble, Hi{10})
			})

			Convey("can override section", func() {
				c, err := Set(c, "hi", &Hi{100})
				So(err, ShouldBeNil)

				h := Hi{}
				So(Get(c, "hi", &h), ShouldBeNil)
				So(h, ShouldResemble, Hi{100})
			})

			Convey("can remove section", func() {
				c, err := Set(c, "hi", nil)
				So(err, ShouldBeNil)

				h := Hi{}
				So(Get(c, "hi", &h), ShouldBeNil)
				So(h, ShouldResemble, Hi{0})
			})
		})

		Convey("Export", func() {
			Convey("empty export is a noop", func() {
				c, _ = Set(c, "hi", nil)
				e, err := Export(c, "")
				So(err, ShouldBeNil)
				So(e, ShouldHaveSameTypeAs, &nullExport{})
			})

			Convey("Exporting with content gives a live export", func() {
				e, err := Export(c, "")
				So(err, ShouldBeNil)
				So(e, ShouldHaveSameTypeAs, &liveExport{})
				defer e.Close()
				cmd := exec.Command("something", "something")
				e.SetInCmd(cmd)
				path, ok := environ.New(cmd.Env).Get(EnvKey)
				So(ok, ShouldBeTrue)

				// There's a valid JSON there.
				blob, err := ioutil.ReadFile(path)
				So(err, ShouldBeNil)
				So(string(blob), ShouldEqual, `{"hi":{"there":10}}`)
			})
		})
	})
}
