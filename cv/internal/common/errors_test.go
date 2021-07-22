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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

	Convey("TQify works", t, func() {
		ctx := memlogger.Use(context.Background())
		ml := logging.Get(ctx).(*memlogger.MemLogger)
		if testing.Verbose() {
			// Write Debug log to both memlogger and gologger.
			ctx = teelogger.Use(ctx, gologger.StdConfig.NewLogger)
		}
		ctx = logging.SetLevel(ctx, logging.Debug)

		assertLoggedStack := func() {
			So(ml.Messages(), ShouldHaveLength, 1)
			m := ml.Messages()[0]
			So(m.Level, ShouldEqual, logging.Error)
			So(m.Msg, ShouldContainSubstring, "common.TestTQifyError()")
			So(m.Msg, ShouldContainSubstring, "errors_test.go")
		}
		assertLoggedAt := func(level logging.Level) {
			So(ml.Messages(), ShouldHaveLength, 1)
			m := ml.Messages()[0]
			So(m.Level, ShouldEqual, level)
			So(m.Msg, ShouldNotContainSubstring, "errors_test.go")
		}

		errOops := errors.New("oops")
		errBoo := errors.New("boo")
		errTransBoo := transient.Tag.Apply(errBoo)
		errWrapOops := errors.Annotate(errOops, "wrapped").Err()
		errMulti := errors.NewMultiError(errWrapOops, errBoo)
		errRare := errors.New("an oppressed invertebrate lacking emoji unlike 🐞 or 🐛")
		errTransRare := transient.Tag.Apply(errRare)

		Convey("matchesErrors is true if it matches ANY leaf errors", func() {
			So(matchesErrors(errWrapOops, errOops), ShouldBeTrue)
			So(matchesErrors(errMulti, errOops), ShouldBeTrue)

			So(matchesErrors(errTransRare, errOops, errBoo, errRare), ShouldBeTrue)
			So(matchesErrors(errTransRare, errOops, errBoo), ShouldBeFalse)
		})

		Convey("matchesErrors panics for invalid knownErrs", func() {
			So(func() { matchesErrors(errOops, errMulti) }, ShouldPanic)
			So(func() { matchesErrors(errOops, errWrapOops) }, ShouldPanic)
		})

		Convey("Simple", func() {
			Convey("noop", func() {
				err := TQifyError(ctx, nil)
				So(err, ShouldBeNil)
				So(ml.Messages(), ShouldHaveLength, 0)
			})
			Convey("non-transient becomes Fatal and is logged", func() {
				err := TQifyError(ctx, errOops)
				So(tq.Fatal.In(err), ShouldBeTrue)
				So(tq.Ignore.In(err), ShouldBeFalse)
				assertLoggedStack()
			})
			Convey("transient is retried and logged", func() {
				err := TQifyError(ctx, errTransBoo)
				So(tq.Fatal.In(err), ShouldBeFalse)
				So(tq.Ignore.In(err), ShouldBeFalse)
				assertLoggedStack()
			})
			Convey("with NeverRetry set", func() {
				Convey("non-transient is still Fatal and logged", func() {
					err := TQIfy{NeverRetry: true}.Error(ctx, errOops)
					So(tq.Fatal.In(err), ShouldBeTrue)
					So(tq.Ignore.In(err), ShouldBeFalse)
					assertLoggedStack()
				})
				Convey("transient is not retried but logged", func() {
					err := TQIfy{NeverRetry: true}.Error(ctx, errTransBoo)
					So(tq.Ignore.In(err), ShouldBeTrue)
					So(tq.Fatal.In(err), ShouldBeFalse)
					assertLoggedStack()
				})
			})
		})

		Convey("With Known errors", func() {
			errRetryTag := errors.BoolTag{
				Key: errors.NewTagKey("this error should be retried"),
			}
			errIgnoreTag := errors.BoolTag{
				Key: errors.NewTagKey("this error should be ignored"),
			}
			tqify := TQIfy{
				KnownRetry:      []error{errBoo},
				KnownRetryTags:  []errors.BoolTag{errRetryTag},
				KnownIgnore:     []error{errOops},
				KnownIgnoreTags: []errors.BoolTag{errIgnoreTag},
			}
			Convey("on unknown error", func() {
				Convey("transient -> retry and log entire stack", func() {
					err := tqify.Error(ctx, errTransRare)
					So(tq.Fatal.In(err), ShouldBeFalse)
					assertLoggedStack()
				})
				Convey("non-transient -> tq.Fatal", func() {
					err := tqify.Error(ctx, errRare)
					So(tq.Fatal.In(err), ShouldBeTrue)
					assertLoggedStack()
				})
			})

			Convey("KnownIgnore => tq.Ignore, log as warning", func() {
				err := tqify.Error(ctx, errWrapOops)
				So(tq.Ignore.In(err), ShouldBeTrue)
				So(tq.Fatal.In(err), ShouldBeFalse)
				assertLoggedAt(logging.Warning)
			})
			Convey("KnownIgnoreTags => tq.Ignore, log as warning", func() {
				err := tqify.Error(ctx, errIgnoreTag.Apply(errRare))
				So(tq.Ignore.In(err), ShouldBeTrue)
				So(tq.Fatal.In(err), ShouldBeFalse)
				assertLoggedAt(logging.Warning)
			})
			Convey("KnownRetry => non-transient, non-Fatal, log as warning", func() {
				err := tqify.Error(ctx, errTransBoo)
				So(tq.Fatal.In(err), ShouldBeFalse)
				So(transient.Tag.In(err), ShouldBeFalse)
				assertLoggedAt(logging.Warning)
			})
			Convey("KnownRetryTags => non-transient, non-Fatal, log as warning", func() {
				err := tqify.Error(ctx, errRetryTag.Apply(errRare))
				So(tq.Fatal.In(err), ShouldBeFalse)
				So(transient.Tag.In(err), ShouldBeFalse)
				assertLoggedAt(logging.Warning)
			})
			Convey("KnownRetry & KnownIgnore => KnownRetry wins, log about a BUG", func() {
				err := tqify.Error(ctx, errMulti)
				So(tq.Fatal.In(err), ShouldBeFalse)
				So(transient.Tag.In(err), ShouldBeFalse)
				So(ml.Messages(), ShouldHaveLength, 2)
				So(ml.Messages()[0].Level, ShouldEqual, logging.Error)
				So(ml.Messages()[0].Msg, ShouldContainSubstring, "BUG: invalid TQIfy config")
				So(ml.Messages()[1].Level, ShouldEqual, logging.Warning)
				So(ml.Messages()[1].Msg, ShouldContainSubstring, errMulti.Error())
			})

			Convey("Panic if KnownRetry is used with with NeverRetry", func() {
				tqify = TQIfy{
					KnownRetry: []error{errBoo},
					NeverRetry: true,
				}
				So(func() { tqify.Error(ctx, errBoo) }, ShouldPanic)
			})
			Convey("Panic if KnownRetryTag is used with with NeverRetry", func() {
				tqify = TQIfy{
					KnownRetryTags: []errors.BoolTag{errRetryTag},
					NeverRetry:     true,
				}
				So(func() { tqify.Error(ctx, errRetryTag.Apply(errRare)) }, ShouldPanic)
			})
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

func TestIsDatastoreContention(t *testing.T) {
	t.Parallel()

	Convey("IsDatastoreContention works", t, func() {
		// fatal means non-transient here.
		So(IsDatastoreContention(errors.New("fatal1")), ShouldBeFalse)
		// This is copied from what was actually observed in prod as of 2021-07-22.
		err := status.Errorf(codes.Aborted, "Aborted due to cross-transaction contention. This occurs when multiple transactions attempt to access the same data, requiring Firestore to abort at least one in order to enforce serializability")
		So(IsDatastoreContention(err), ShouldBeTrue)
		So(IsDatastoreContention(errors.Annotate(err, "wrapped").Err()), ShouldBeTrue)
	})
}
