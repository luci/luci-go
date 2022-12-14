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

package cli

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/exitcode"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/sink"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

var matchInvalidInvocationIDChars = regexp.MustCompile(`[^a-z0-9_\-:.]`)

const (
	// reservePeriodSecs is how many seconds should be reserved for `rdb stream` to
	// complete (out of a grace period), the rest can be given to the payload.
	reservePeriodSecs = 3
)

// MustReturnInvURL returns a string of the Invocation URL.
func MustReturnInvURL(rdbHost, invName string) string {
	invID, err := pbutil.ParseInvocationName(invName)
	if err != nil {
		panic(err)
	}

	miloHost := chromeinfra.MiloDevHost
	if rdbHost == chromeinfra.ResultDBHost {
		miloHost = chromeinfra.MiloHost
	}
	return fmt.Sprintf("https://%s/ui/inv/%s", miloHost, invID)
}

func cmdStream(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `stream [flags] TEST_CMD [TEST_ARG]...`,
		ShortDesc: "Run a given test command and upload the results to ResultDB",
		// TODO(crbug.com/1017288): add a link to ResultSink protocol doc
		LongDesc: text.Doc(`
			Run a given test command, continuously collect the results over IPC, and
			upload them to ResultDB. Either use the current invocation from
			LUCI_CONTEXT or create/finalize a new one. Example:
				rdb stream -new -realm chromium:public ./out/chrome/test/browser_tests
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &streamRun{
				vars: make(map[string]string),
				tags: make(strpair.Map),
			}
			r.baseCommandRun.RegisterGlobalFlags(p)
			r.Flags.BoolVar(&r.isNew, "new", false, text.Doc(`
				If true, create and use a new invocation for the test command.
				If false, use the current invocation, set in LUCI_CONTEXT.
			`))
			r.Flags.BoolVar(&r.isIncluded, "include", false, text.Doc(`
				If true with -new, the new invocation will be included
				in the current invocation, set in LUCI_CONTEXT.
			`))
			r.Flags.StringVar(&r.realm, "realm", "", text.Doc(`
				Realm to create the new invocation in. Required if -new is set,
				ignored otherwise.
				e.g. "chromium:public"
			`))
			r.Flags.StringVar(&r.testIDPrefix, "test-id-prefix", "", text.Doc(`
				Prefix to prepend to the test ID of every test result.
			`))
			r.Flags.Var(flag.StringMap(r.vars), "var", text.Doc(`
				Variant to add to every test result in "key:value" format.
				If the test command adds a variant with the same key, the value given by
				this flag will get overridden.
			`))
			r.Flags.UintVar(&r.artChannelMaxLeases, "max-concurrent-artifact-uploads",
				sink.DefaultArtChannelMaxLeases, text.Doc(`
				The maximum number of goroutines uploading artifacts.
			`))
			r.Flags.UintVar(&r.trChannelMaxLeases, "max-concurrent-test-result-uploads",
				sink.DefaultTestResultChannelMaxLeases, text.Doc(`
				The maximum number of goroutines uploading test results.
			`))
			r.Flags.StringVar(&r.testTestLocationBase, "test-location-base", "", text.Doc(`
				File base to prepend to the test location file name, if the file name is a relative path.
				It must start with "//".
			`))
			r.Flags.Var(flag.StringPairs(r.tags), "tag", text.Doc(`
				Tag to add to every test result in "key:value" format.
				A key can be repeated.
			`))
			r.Flags.BoolVar(&r.coerceNegativeDuration, "coerce-negative-duration",
				false, text.Doc(`
				If true, all negative durations will be coerced to 0.
				If false, test results with negative durations will be rejected.
			`))
			r.Flags.StringVar(&r.locTagsFile, "location-tags-file", "", text.Doc(`
				Path to the file that contains test location tags in JSON format. See
				https://source.chromium.org/chromium/infra/infra/+/master:go/src/go.chromium.org/luci/resultdb/sink/proto/v1/location_tag.proto.
			`))
			r.Flags.BoolVar(&r.exonerateUnexpectedPass, "exonerate-unexpected-pass",
				false, text.Doc(`
				If true, any unexpected pass result will be exonerated.
			`))
			r.Flags.TextVar(&r.invProperties, "inv-properties", &invProperties{}, text.Doc(`
				Stringified JSON object that contains structured,
				domain-specific properties of the invocation.
				The command will fail if the 'properties' is already set on the invocation
				(NOT ENFORCED YET).
			`))
			r.Flags.StringVar(&r.invPropertiesFile, "inv-properties-file", "", text.Doc(`
				Similar to -inv-properties but takes a path to the file that contains the JSON object.
				Cannot be used when -inv-properties is specified.
			`))
			return r
		},
	}
}

type invProperties struct {
	*structpb.Struct
}

// Implements encoding.TextUnmarshaler.
func (s *invProperties) UnmarshalText(text []byte) error {
	// Treat empty text as nil. This indicates that the properties is not
	// specified and will not be updated.
	// '{}' means the properties should be set to an empty object.
	if len(text) == 0 {
		s.Struct = nil
		return nil
	}

	properties := &structpb.Struct{}
	if err := protojson.Unmarshal(text, properties); err != nil {
		return err
	}
	s.Struct = properties
	return nil
}

// Implements encoding.TextMarshaler.
func (s *invProperties) MarshalText() (text []byte, err error) {
	// Serialize nil struct to empty string so nil struct won't be serialized as
	// '{}'.
	if s.Struct == nil {
		return nil, nil
	}

	return protojson.Marshal(s.Struct)
}

type streamRun struct {
	baseCommandRun

	// flags
	isNew                   bool
	isIncluded              bool
	realm                   string
	testIDPrefix            string
	testTestLocationBase    string
	vars                    map[string]string
	artChannelMaxLeases     uint
	trChannelMaxLeases      uint
	tags                    strpair.Map
	coerceNegativeDuration  bool
	locTagsFile             string
	exonerateUnexpectedPass bool
	invPropertiesFile       string
	invProperties           invProperties
	// TODO(ddoman): add flags
	// - invocation-tag
	// - log-file

	invocation *lucictx.ResultDBInvocation
}

func (r *streamRun) validate(ctx context.Context, args []string) (err error) {
	if len(args) == 0 {
		return errors.Reason("missing a test command to run").Err()
	}
	if err := pbutil.ValidateVariant(&pb.Variant{Def: r.vars}); err != nil {
		return errors.Annotate(err, "invalid variant").Err()
	}
	if r.realm != "" {
		if err := realms.ValidateRealmName(r.realm, realms.GlobalScope); err != nil {
			return errors.Annotate(err, "invalid realm").Err()
		}
	}
	if r.invProperties.Struct != nil && r.invPropertiesFile != "" {
		return errors.Reason("cannot specify both -inv-properties and -inv-properties-file at the same time").Err()
	}
	return nil
}

func (r *streamRun) Run(a subcommands.Application, args []string, env subcommands.Env) (ret int) {
	ctx := cli.GetContext(a, r, env)

	if err := r.validate(ctx, args); err != nil {
		return r.done(err)
	}

	loginMode := auth.OptionalLogin
	// login is required only if it creates a new invocation.
	if r.isNew {
		if r.realm == "" {
			return r.done(errors.Reason("-realm is required for new invocations").Err())
		}
		loginMode = auth.SilentLogin
	}
	if err := r.initClients(ctx, loginMode); err != nil {
		return r.done(err)
	}

	// if -new is passed, create a new invocation. If not, use the existing one set in
	// lucictx.
	switch {
	case r.isNew:
		if r.isIncluded && r.resultdbCtx == nil {
			return r.done(errors.Reason("missing an invocation in LUCI_CONTEXT, but -include was given").Err())
		}

		newInv, err := r.createInvocation(ctx, r.realm)
		if err != nil {
			return r.done(err)
		}
		fmt.Fprintf(os.Stderr, "rdb-stream: created invocation - %s\n", MustReturnInvURL(r.host, newInv.Name))
		if r.isIncluded {
			curInv := r.resultdbCtx.CurrentInvocation
			if err := r.includeInvocation(ctx, curInv, &newInv); err != nil {
				if ferr := r.finalizeInvocation(ctx); ferr != nil {
					logging.Errorf(ctx, "failed to finalize the invocation: %s", ferr)
				}
				return r.done(err)
			}
			fmt.Fprintf(os.Stderr, "rdb-stream: included %q in %q\n", newInv.Name, curInv.Name)
		}

		// Update lucictx with the new invocation.
		r.invocation = &newInv
		ctx = lucictx.SetResultDB(ctx, &lucictx.ResultDB{
			Hostname:          r.host,
			CurrentInvocation: r.invocation,
		})
	case r.isIncluded:
		return r.done(errors.Reason("-new is required for -include").Err())
	case r.resultdbCtx == nil:
		return r.done(errors.Reason("missing an invocation in LUCI_CONTEXT; use -new to create a new one").Err())
	default:
		if err := r.validateCurrentInvocation(); err != nil {
			return r.done(err)
		}
		r.invocation = r.resultdbCtx.CurrentInvocation
	}

	invProperties, err := r.invPropertiesFromArgs(ctx)
	if err != nil {
		return r.done(errors.Annotate(err, "get invocation properties from arguments").Err())
	}
	if invProperties != nil {
		if err := r.updateInvProperties(ctx, invProperties); err != nil {
			return r.done(err)
		}
	}

	defer func() {
		// Finalize the invocation if it was created by -new.
		if r.isNew {
			if err := r.finalizeInvocation(ctx); err != nil {
				logging.Errorf(ctx, "failed to finalize the invocation: %s", err)
				ret = r.done(err)
			}
			fmt.Fprintf(os.Stderr, "rdb-stream: finalized invocation - %s\n", MustReturnInvURL(r.host, r.invocation.Name))
		}
	}()

	err = r.runTestCmd(ctx, args)
	ec, ok := exitcode.Get(err)
	if !ok {
		logging.Errorf(ctx, "rdb-stream: failed to run the test command: %s", err)
		return r.done(err)
	}
	logging.Infof(ctx, "rdb-stream: exiting with %d", ec)
	return ec
}

func (r *streamRun) runTestCmd(ctx context.Context, args []string) error {
	cmdCtx, cancelCmd := lucictx.TrackSoftDeadline(ctx, reservePeriodSecs*time.Second)
	defer cancelCmd()

	cmd := exec.CommandContext(cmdCtx, args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	setSysProcAttr(cmd)
	cmdProcMu := sync.Mutex{}

	// Interrupt the subprocess if rdb-stream is interrupted or the deadline
	// approaches.
	// If it does not finish before the grace period expires, it will be
	// SIGKILLed by the the expiration of cmdCtx.
	go func() {
		evt := <-lucictx.SoftDeadlineDone(cmdCtx)
		if evt == lucictx.ClosureEvent {
			// Cleanup only.
			return
		}
		logging.Infof(ctx, "Caught %s", evt.String())

		// Prevent accessing cmd.Process while it's being started.
		cmdProcMu.Lock()
		defer cmdProcMu.Unlock()
		if err := terminate(ctx, cmd.Process); err != nil {
			logging.Warningf(ctx, "Could not terminate subprocess (%s), cancelling its context", err)
			cancelCmd()
			return
		}
		logging.Infof(ctx, "Sent termination signal to subprocess, it has ~%s to terminate", lucictx.GetDeadline(cmdCtx).GracePeriodDuration())
	}()

	locationTags, err := r.locationTagsFromArg(ctx)
	if err != nil {
		return errors.Annotate(err, "get location tags").Err()
	}
	// TODO(ddoman): send the logs of SinkServer to --log-file

	cfg := sink.ServerConfig{
		ArtChannelMaxLeases:        r.artChannelMaxLeases,
		ArtifactStreamClient:       r.http,
		ArtifactStreamHost:         r.host,
		Recorder:                   r.recorder,
		TestResultChannelMaxLeases: r.trChannelMaxLeases,

		Invocation:  r.invocation.Name,
		UpdateToken: r.invocation.UpdateToken,

		BaseTags:                pbutil.FromStrpairMap(r.tags),
		BaseVariant:             &pb.Variant{Def: r.vars},
		CoerceNegativeDuration:  r.coerceNegativeDuration,
		LocationTags:            locationTags,
		TestLocationBase:        r.testTestLocationBase,
		TestIDPrefix:            r.testIDPrefix,
		ExonerateUnexpectedPass: r.exonerateUnexpectedPass,
	}
	return sink.Run(ctx, cfg, func(ctx context.Context, cfg sink.ServerConfig) error {
		exported, err := lucictx.Export(ctx)
		if err != nil {
			return err
		}
		defer func() {
			logging.Infof(ctx, "rdb-stream: the test process terminated")
			exported.Close()
		}()
		exported.SetInCmd(cmd)
		logging.Infof(ctx, "rdb-stream: starting the test command - %q", cmd.Args)

		cmdProcMu.Lock()
		err = cmd.Start()
		cmdProcMu.Unlock()

		if err != nil {
			return errors.Annotate(err, "cmd.start").Err()
		}
		return cmd.Wait()
	})
}

func (r *streamRun) locationTagsFromArg(ctx context.Context) (*sinkpb.LocationTags, error) {
	if r.locTagsFile == "" {
		return nil, nil
	}
	f, err := os.ReadFile(r.locTagsFile)
	switch {
	case os.IsNotExist(err):
		logging.Warningf(ctx, "rdb-stream: %s does not exist", r.locTagsFile)
		return nil, nil
	case err != nil:
		return nil, err
	}
	locationTags := &sinkpb.LocationTags{}
	if err = protojson.Unmarshal(f, locationTags); err != nil {
		return nil, err
	}
	return locationTags, nil
}

// invPropertiesFromArgs gets invocation-level proeprties from arguments.
// If r.invProperties is set, return it.
// If r.invPropertiesFile is set, parse the file and return the value.
// Return nil if neither are set.
func (r *streamRun) invPropertiesFromArgs(ctx context.Context) (*structpb.Struct, error) {
	if r.invProperties.Struct != nil {
		return r.invProperties.Struct, nil
	}

	if r.invPropertiesFile == "" {
		return nil, nil
	}

	f, err := os.ReadFile(r.invPropertiesFile)
	switch {
	case os.IsNotExist(err):
		logging.Warningf(ctx, "rdb-stream: %s does not exist", r.invPropertiesFile)
		return nil, nil
	case err != nil:
		return nil, err
	}

	properties := &structpb.Struct{}
	if err = protojson.Unmarshal(f, properties); err != nil {
		return nil, err
	}

	return properties, nil
}

func (r *streamRun) createInvocation(ctx context.Context, realm string) (ret lucictx.ResultDBInvocation, err error) {
	invID, err := GenInvID(ctx)
	if err != nil {
		return
	}

	md := metadata.MD{}
	resp, err := r.recorder.CreateInvocation(ctx, &pb.CreateInvocationRequest{
		InvocationId: invID,
		Invocation: &pb.Invocation{
			Realm: realm,
		},
	}, grpc.Header(&md))
	if err != nil {
		err = errors.Annotate(err, "failed to create an invocation").Err()
		return
	}
	tks := md.Get(pb.UpdateTokenMetadataKey)
	if len(tks) == 0 {
		err = errors.Reason("Missing header: %s", pb.UpdateTokenMetadataKey).Err()
		return
	}

	ret = lucictx.ResultDBInvocation{Name: resp.Name, UpdateToken: tks[0]}
	return
}

func (r *streamRun) includeInvocation(ctx context.Context, parent, child *lucictx.ResultDBInvocation) error {
	ctx = metadata.AppendToOutgoingContext(ctx, pb.UpdateTokenMetadataKey, parent.UpdateToken)
	_, err := r.recorder.UpdateIncludedInvocations(ctx, &pb.UpdateIncludedInvocationsRequest{
		IncludingInvocation: parent.Name,
		AddInvocations:      []string{child.Name},
	})
	return err
}

// updateInvProperties sets the properties on the invocation.
func (r *streamRun) updateInvProperties(ctx context.Context, properties *structpb.Struct) error {
	ctx = metadata.AppendToOutgoingContext(ctx, pb.UpdateTokenMetadataKey, r.invocation.UpdateToken)
	_, err := r.recorder.UpdateInvocation(ctx, &pb.UpdateInvocationRequest{
		Invocation: &pb.Invocation{
			Name:       r.invocation.Name,
			Properties: properties,
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"properties"}},
	})
	return err
}

// finalizeInvocation finalizes the invocation.
func (r *streamRun) finalizeInvocation(ctx context.Context) error {
	ctx = metadata.AppendToOutgoingContext(ctx, pb.UpdateTokenMetadataKey, r.invocation.UpdateToken)
	_, err := r.recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{
		Name: r.invocation.Name,
	})
	return err
}

// GenInvID generates an invocation ID, made of the username, the current timestamp
// in a human-friendly format, and a random suffix.
//
// This can be used to generate a random invocation ID, but the creator and creation time
// can be easily found.
func GenInvID(ctx context.Context) (string, error) {
	whoami, err := user.Current()
	if err != nil {
		return "", err
	}
	bytes := make([]byte, 8)
	if _, err := mathrand.Read(ctx, bytes); err != nil {
		return "", err
	}

	username := strings.ToLower(whoami.Username)
	username = matchInvalidInvocationIDChars.ReplaceAllString(username, "")

	suffix := strings.ToLower(fmt.Sprintf(
		"%s-%s", time.Now().UTC().Format("2006-01-02-15-04-00"),
		// Note: cannot use base64 because not all of its characters are allowed
		// in invocation IDs.
		hex.EncodeToString(bytes)))

	// An invocation ID can contain up to 100 ascii characters that conform to the regex,
	return fmt.Sprintf("u-%.*s-%s", 100-len(suffix), username, suffix), nil
}
