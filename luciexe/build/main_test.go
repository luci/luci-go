package build

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/system/environ"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/luciexe/build/internal/testpb"
)

func init() {
	// ensure that send NEVER blocks while testing Main functionality
	mainSendRate = rate.Inf
}

func TestMain(t *testing.T) {
	// avoid t.Parallel() because this registers property handlers.

	Convey(`Main`, t, func() {
		ctx := memlogger.Use(context.Background())
		logs := logging.Get(ctx).(*memlogger.MemLogger)

		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		nowpb := timestamppb.New(testclock.TestRecentTimeUTC)

		scFake := streamclient.NewFake()
		defer scFake.Unregister()

		env := environ.New(nil)
		env.Set(bootstrap.EnvStreamServerPath, scFake.StreamServerPath())
		env.Set(bootstrap.EnvNamespace, "u")
		ctx = env.SetInCtx(ctx)

		imsg := &testpb.TopLevel{}
		var setOut func(*testpb.TopLevel)

		tdir := t.TempDir()

		finalBuildPath := filepath.Join(tdir, "finalBuild.json")
		args := []string{"myprogram", "--output", finalBuildPath}
		stdin := &bytes.Buffer{}

		mkStruct := func(dictlike map[string]interface{}) *structpb.Struct {
			s, err := structpb.NewStruct(dictlike)
			So(err, ShouldBeNil)
			return s
		}

		writeStdinProps := func(dictlike map[string]interface{}) {
			b := &bbpb.Build{
				Input: &bbpb.Build_Input{
					Properties: mkStruct(dictlike),
				},
			}
			data, err := proto.Marshal(b)
			So(err, ShouldBeNil)
			_, err = stdin.Write(data)
			So(err, ShouldBeNil)
		}

		getFinal := func() *bbpb.Build {
			data, err := os.ReadFile(finalBuildPath)
			So(err, ShouldBeNil)
			ret := &bbpb.Build{}
			So(protojson.Unmarshal(data, ret), ShouldBeNil)

			// proto module is cute and tries to introduce non-deterministic
			// characters into their error messages. This is annoying and unhelpful
			// for tests where error messages intentionally can show up in the Build
			// output. We manually normalize them here. Replaces non-breaking space
			// (U+00a0) with space (U+0020)
			ret.SummaryMarkdown = strings.ReplaceAll(ret.SummaryMarkdown, " ", " ")

			return ret
		}

		Convey(`good`, func() {
			Convey(`simple`, func() {
				err := main(ctx, args, stdin, imsg, nil, nil, func(ctx context.Context, args []string, st *State) error {
					So(args, ShouldBeNil)
					return nil
				})
				So(err, ShouldBeNil)
				So(getFinal(), ShouldResembleProto, &bbpb.Build{
					StartTime: nowpb,
					EndTime:   nowpb,
					Status:    bbpb.Status_SUCCESS,
					Output:    &bbpb.Build_Output{},
					Input:     &bbpb.Build_Input{},
				})
			})

			Convey(`user args`, func() {
				args = append(args, "--", "custom", "stuff")
				err := main(ctx, args, stdin, imsg, &setOut, nil, func(ctx context.Context, args []string, st *State) error {
					So(args, ShouldResemble, []string{"custom", "stuff"})
					return nil
				})
				So(err, ShouldBeNil)
				So(getFinal(), ShouldResembleProto, &bbpb.Build{
					StartTime: nowpb,
					EndTime:   nowpb,
					Status:    bbpb.Status_SUCCESS,
					Output:    &bbpb.Build_Output{},
					Input:     &bbpb.Build_Input{},
				})
			})

			Convey(`inputProps`, func() {
				writeStdinProps(map[string]interface{}{
					"field": "something",
					"$cool": "blah",
				})

				err := main(ctx, args, stdin, imsg, &setOut, nil, func(ctx context.Context, args []string, st *State) error {
					So(imsg, ShouldResembleProto, &testpb.TopLevel{
						Field:         "something",
						JsonNameField: "blah",
					})
					return nil
				})
				So(err, ShouldBeNil)
			})

			Convey(`help`, func() {
				args = append(args, "--help")
				err := main(ctx, args, stdin, imsg, &setOut, nil, func(ctx context.Context, args []string, st *State) error {
					return nil
				})
				So(err, ShouldBeNil)
				So(logs, memlogger.ShouldHaveLog, logging.Info, "`myprogram` is a `luciexe` binary. See go.chromium.org/luci/luciexe.")
				So(logs, memlogger.ShouldHaveLog, logging.Info, "======= I/O Proto =======")
				// TODO(iannucci): check I/O proto when implemented
			})
		})

		Convey(`errors`, func() {
			Convey(`returned`, func() {
				err := main(ctx, args, stdin, imsg, &setOut, nil, func(ctx context.Context, args []string, st *State) error {
					So(args, ShouldBeNil)
					return errors.New("bad stuff")
				})
				So(err, ShouldEqual, errNonSuccess)
				So(getFinal(), ShouldResembleProto, &bbpb.Build{
					StartTime: nowpb,
					EndTime:   nowpb,
					Status:    bbpb.Status_FAILURE,
					Output:    &bbpb.Build_Output{},
					Input:     &bbpb.Build_Input{},
				})
				So(logs, memlogger.ShouldHaveLog, logging.Error, "set status: FAILURE: bad stuff")
			})

			Convey(`panic`, func() {
				err := main(ctx, args, stdin, imsg, &setOut, nil, func(ctx context.Context, args []string, st *State) error {
					So(args, ShouldBeNil)
					panic("BAD THINGS")
				})
				So(err, ShouldEqual, errNonSuccess)
				So(getFinal(), ShouldResembleProto, &bbpb.Build{
					StartTime: nowpb,
					EndTime:   nowpb,
					Status:    bbpb.Status_INFRA_FAILURE,
					Output:    &bbpb.Build_Output{},
					Input:     &bbpb.Build_Input{},
				})
				So(logs, memlogger.ShouldHaveLog, logging.Error, "set status: INFRA_FAILURE: PANIC")
				So(logs, memlogger.ShouldHaveLog, logging.Error, "recovered panic: BAD THINGS")
			})

			Convey(`inputProps`, func() {
				writeStdinProps(map[string]interface{}{
					"bogus": "something",
				})
				args = append(args, "--strict-input")

				err := main(ctx, args, stdin, imsg, &setOut, nil, func(ctx context.Context, args []string, st *State) error {
					return nil
				})
				So(err, ShouldErrLike, "parsing top-level properties")
				So(getFinal(), ShouldResembleProto, &bbpb.Build{
					StartTime:       nowpb,
					EndTime:         nowpb,
					Status:          bbpb.Status_INFRA_FAILURE,
					Output:          &bbpb.Build_Output{},
					SummaryMarkdown: "fatal error starting build: parsing top-level properties: proto: (line 1:2): unknown field \"bogus\"",
					Input: &bbpb.Build_Input{
						Properties: mkStruct(map[string]interface{}{
							"bogus": "something",
						}),
					},
				})
			})

		})
	})
}
