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

package common

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/logging/teelogger"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/tq"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTQifyError(t *testing.T) {
	t.Parallel()

	Convey("TQifyError works", t, func() {
		ctx := memlogger.Use(context.Background())
		ml := logging.Get(ctx).(*memlogger.MemLogger)
		if testing.Verbose() {
			// Write Debug log to both memlogger and gologger.
			ctx = teelogger.Use(ctx, gologger.StdConfig.NewLogger)
		}
		ctx = logging.SetLevel(ctx, logging.Debug)

		assertLoggedStack := func() string {
			So(ml.Messages(), ShouldHaveLength, 1)
			m := ml.Messages()[0]
			So(m.Level, ShouldEqual, logging.Error)
			So(m.Msg, ShouldContainSubstring, "common.TestTQifyError()")
			So(m.Msg, ShouldContainSubstring, "errors_test.go")
			return m.Msg
		}

		errOops := errors.New("oops")
		errBoo := errors.New("boo")
		errTransBoo := transient.Tag.Apply(errBoo)
		errWrapOops := errors.Annotate(errOops, "wrapped").Err()
		errMulti := errors.NewMultiError(errWrapOops, errBoo)

		Convey("noop", func() {
			err := TQifyError(ctx, nil)
			So(err, ShouldBeNil)
			So(ml.Messages(), ShouldHaveLength, 0)
		})
		Convey("fatal", func() {
			err := TQifyError(ctx, errOops)
			So(tq.Fatal.In(err), ShouldBeTrue)
			So(assertLoggedStack(), ShouldContainSubstring, "oops")
		})
		Convey("trans", func() {
			err := TQifyError(ctx, errTransBoo)
			So(tq.Fatal.In(err), ShouldBeFalse)
			So(assertLoggedStack(), ShouldContainSubstring, "boo")
		})
		Convey("not excluded", func() {
			err := TQifyError(ctx, errTransBoo, errOops)
			So(tq.Fatal.In(err), ShouldBeFalse)
			So(assertLoggedStack(), ShouldContainSubstring, "boo")
		})
		Convey("exclude simple unwrap", func() {
			err := TQifyError(ctx, errTransBoo, errBoo)
			So(tq.Fatal.In(err), ShouldBeFalse)
			So(ml.Messages(), ShouldBeEmpty)
		})
		Convey("exclude with multierror", func() {
			err := TQifyError(ctx, errMulti, errOops)
			So(tq.Fatal.In(err), ShouldBeTrue)
			So(ml.Messages(), ShouldBeEmpty)
		})
	})
}

func TestMostSevereError(t *testing.T) {
	t.Parallel()

	Convey("MostSevereError works", t, func() {
		// fatal means non-transient here.
		fatal1 := errors.New("fatal1")
		fatal2 := errors.New("fatal2")
		trans1 := errors.New("trans1", transient.Tag)
		trans2 := errors.New("trans2", transient.Tag)
		multFatal := errors.NewMultiError(trans1, nil, fatal1, fatal2)
		multTrans := errors.NewMultiError(nil, trans1, nil, trans2, nil)
		tensor := errors.NewMultiError(nil, errors.NewMultiError(nil, multTrans, multFatal))

		So(MostSevereError(nil), ShouldBeNil)
		So(MostSevereError(fatal1), ShouldEqual, fatal1)
		So(MostSevereError(trans1), ShouldEqual, trans1)

		So(MostSevereError(multFatal), ShouldEqual, fatal1)
		So(MostSevereError(multTrans), ShouldEqual, trans1)
		So(MostSevereError(tensor), ShouldEqual, fatal1)
	})
}
