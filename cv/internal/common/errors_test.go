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
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/logging/teelogger"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/tq"
)

func TestTQifyError(t *testing.T) {
	t.Parallel()

	errRetryTag := errtag.Make("this error should be retried", true)
	errIgnoreTag := errtag.Make("this error should be ignored", true)

	ftt.Run("TQify works", t, func(t *ftt.Test) {
		ctx := memlogger.Use(context.Background())
		ml := logging.Get(ctx).(*memlogger.MemLogger)
		if testing.Verbose() {
			// Write Debug log to both memlogger and gologger.
			ctx = teelogger.Use(ctx, gologger.StdConfig.NewLogger)
		}
		ctx = logging.SetLevel(ctx, logging.Debug)

		assertLoggedStack := func() {
			assert.Loosely(t, ml.Messages(), should.HaveLength(1))
			m := ml.Messages()[0]
			assert.Loosely(t, m.Level, should.Equal(logging.Error))
			assert.Loosely(t, m.StackTrace.Standard, should.ContainSubstring("common.TestTQifyError"))
			assert.Loosely(t, m.StackTrace.Standard, should.ContainSubstring("errors_test.go"))
		}
		assertLoggedAt := func(level logging.Level) {
			assert.Loosely(t, ml.Messages(), should.HaveLength(1))
			m := ml.Messages()[0]
			assert.Loosely(t, m.Level, should.Equal(level))
			assert.Loosely(t, m.Msg, should.NotContainSubstring("errors_test.go"))
		}

		errOops := errors.New("oops")
		errBoo := errors.New("boo")
		errTransBoo := transient.Tag.Apply(errBoo)
		errWrapOops := errors.Annotate(errOops, "wrapped").Err()
		errMulti := errors.NewMultiError(errWrapOops, errBoo)
		errRare := errors.New("an oppressed invertebrate lacking emoji unlike 🐞 or 🐛")
		errTransRare := transient.Tag.Apply(errRare)

		t.Run("matchesErrors is true if it matches ANY leaf errors", func(t *ftt.Test) {
			assert.Loosely(t, matchesErrors(errWrapOops, errOops), should.BeTrue)
			assert.Loosely(t, matchesErrors(errMulti, errOops), should.BeTrue)

			assert.Loosely(t, matchesErrors(errTransRare, errOops, errBoo, errRare), should.BeTrue)
			assert.Loosely(t, matchesErrors(errTransRare, errOops, errBoo), should.BeFalse)
		})

		t.Run("Simple", func(t *ftt.Test) {
			t.Run("noop", func(t *ftt.Test) {
				err := TQifyError(ctx, nil)
				assert.NoErr(t, err)
				assert.Loosely(t, ml.Messages(), should.HaveLength(0))
			})
			t.Run("non-transient becomes Fatal and is logged", func(t *ftt.Test) {
				err := TQifyError(ctx, errOops)
				assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
				assert.Loosely(t, tq.Ignore.In(err), should.BeFalse)
				assertLoggedStack()
			})
			t.Run("transient is retried and logged", func(t *ftt.Test) {
				err := TQifyError(ctx, errTransBoo)
				assert.Loosely(t, tq.Fatal.In(err), should.BeFalse)
				assert.Loosely(t, tq.Ignore.In(err), should.BeFalse)
				assertLoggedStack()
			})
			t.Run("with NeverRetry set", func(t *ftt.Test) {
				t.Run("non-transient is still Fatal and logged", func(t *ftt.Test) {
					err := TQIfy{NeverRetry: true}.Error(ctx, errOops)
					assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
					assert.Loosely(t, tq.Ignore.In(err), should.BeFalse)
					assertLoggedStack()
				})
				t.Run("transient is not retried but logged", func(t *ftt.Test) {
					err := TQIfy{NeverRetry: true}.Error(ctx, errTransBoo)
					assert.Loosely(t, tq.Ignore.In(err), should.BeTrue)
					assert.Loosely(t, tq.Fatal.In(err), should.BeFalse)
					assertLoggedStack()
				})
			})
		})

		t.Run("With Known errors", func(t *ftt.Test) {
			tqify := TQIfy{
				KnownRetry:      []error{errBoo},
				KnownRetryTags:  []errtag.Tag[bool]{errRetryTag},
				KnownIgnore:     []error{errOops},
				KnownIgnoreTags: []errtag.Tag[bool]{errIgnoreTag},
			}
			t.Run("on unknown error", func(t *ftt.Test) {
				t.Run("transient -> retry and log entire stack", func(t *ftt.Test) {
					err := tqify.Error(ctx, errTransRare)
					assert.Loosely(t, tq.Fatal.In(err), should.BeFalse)
					assertLoggedStack()
				})
				t.Run("non-transient -> tq.Fatal", func(t *ftt.Test) {
					err := tqify.Error(ctx, errRare)
					assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
					assertLoggedStack()
				})
			})

			t.Run("KnownIgnore => tq.Ignore, log as warning", func(t *ftt.Test) {
				err := tqify.Error(ctx, errWrapOops)
				assert.Loosely(t, tq.Ignore.In(err), should.BeTrue)
				assert.Loosely(t, tq.Fatal.In(err), should.BeFalse)
				assertLoggedAt(logging.Warning)
			})
			t.Run("KnownIgnoreTags => tq.Ignore, log as warning", func(t *ftt.Test) {
				err := tqify.Error(ctx, errIgnoreTag.Apply(errRare))
				assert.Loosely(t, tq.Ignore.In(err), should.BeTrue)
				assert.Loosely(t, tq.Fatal.In(err), should.BeFalse)
				assertLoggedAt(logging.Warning)
			})
			t.Run("KnownRetry => non-transient, non-Fatal, log as warning", func(t *ftt.Test) {
				err := tqify.Error(ctx, errTransBoo)
				assert.Loosely(t, tq.Fatal.In(err), should.BeFalse)
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
				assertLoggedAt(logging.Warning)
			})
			t.Run("KnownRetryTags => non-transient, non-Fatal, log as warning", func(t *ftt.Test) {
				err := tqify.Error(ctx, errRetryTag.Apply(errRare))
				assert.Loosely(t, tq.Fatal.In(err), should.BeFalse)
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
				assertLoggedAt(logging.Warning)
			})
			t.Run("KnownRetry & KnownIgnore => KnownRetry wins, log about a BUG", func(t *ftt.Test) {
				err := tqify.Error(ctx, errMulti)
				assert.Loosely(t, tq.Fatal.In(err), should.BeFalse)
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
				assert.Loosely(t, ml.Messages(), should.HaveLength(2))
				assert.Loosely(t, ml.Messages()[0].Level, should.Equal(logging.Error))
				assert.Loosely(t, ml.Messages()[0].Msg, should.ContainSubstring("BUG: invalid TQIfy config"))
				assert.Loosely(t, ml.Messages()[1].Level, should.Equal(logging.Warning))
				assert.Loosely(t, ml.Messages()[1].Msg, should.ContainSubstring(errMulti.Error()))
			})

			t.Run("Panic if KnownRetry is used with with NeverRetry", func(t *ftt.Test) {
				tqify = TQIfy{
					KnownRetry: []error{errBoo},
					NeverRetry: true,
				}
				assert.Loosely(t, func() { tqify.Error(ctx, errBoo) }, should.Panic)
			})
			t.Run("Panic if KnownRetryTag is used with with NeverRetry", func(t *ftt.Test) {
				tqify = TQIfy{
					KnownRetryTags: []errtag.Tag[bool]{errRetryTag},
					NeverRetry:     true,
				}
				assert.Loosely(t, func() { tqify.Error(ctx, errRetryTag.Apply(errRare)) }, should.Panic)
			})
		})
	})
}

func TestMostSevereError(t *testing.T) {
	t.Parallel()

	ftt.Run("MostSevereError works", t, func(t *ftt.Test) {
		// fatal means non-transient here.
		fatal1 := errors.New("fatal1")
		fatal2 := errors.New("fatal2")
		trans1 := errors.New("trans1", transient.Tag)
		trans2 := errors.New("trans2", transient.Tag)
		multFatal := errors.NewMultiError(trans1, nil, fatal1, fatal2)
		multTrans := errors.NewMultiError(nil, trans1, nil, trans2, nil)
		tensor := errors.NewMultiError(nil, errors.NewMultiError(nil, multTrans, multFatal))

		assert.NoErr(t, MostSevereError(nil))
		assert.ErrIsLike(t, MostSevereError(fatal1), fatal1)
		assert.ErrIsLike(t, MostSevereError(trans1), trans1)

		assert.ErrIsLike(t, MostSevereError(multFatal), fatal1)
		assert.ErrIsLike(t, MostSevereError(multTrans), trans1)
		assert.ErrIsLike(t, MostSevereError(tensor), fatal1)
	})
}

func TestIsDatastoreContention(t *testing.T) {
	t.Parallel()

	ftt.Run("IsDatastoreContention works", t, func(t *ftt.Test) {
		// fatal means non-transient here.
		assert.Loosely(t, IsDatastoreContention(errors.New("fatal1")), should.BeFalse)
		// This is copied from what was actually observed in prod as of 2021-07-22.
		err := status.Errorf(codes.Aborted, "Aborted due to cross-transaction contention. This occurs when multiple transactions attempt to access the same data, requiring Firestore to abort at least one in order to enforce serializability")
		assert.Loosely(t, IsDatastoreContention(err), should.BeTrue)
		assert.Loosely(t, IsDatastoreContention(errors.Annotate(err, "wrapped").Err()), should.BeTrue)
	})
}
