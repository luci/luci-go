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
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/tq"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTQifyError(t *testing.T) {
	t.Parallel()

	Convey("TQifyError works", t, func() {
		ctx := context.Background()
		if testing.Verbose() {
			ctx = logging.SetLevel(gologger.StdConfig.Use(ctx), logging.Debug)
		}

		err := errors.New("oops")
		err = TQifyError(ctx, err)
		So(tq.Fatal.In(err), ShouldBeTrue)
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
