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
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/system/exitcode"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/resultdb/internal/schemes"
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

	previousTestIDPrefixDisabledValue = ":disabled"
)

// MustReturnInvURL returns a string of the Invocation URL.
func MustReturnInvURL(rdbHost, invName string) string {
	invID, err := pbutil.ParseInvocationName(invName)
	if err != nil {
		panic(err)
	}

	miloHost := chromeinfra.MiloDevUIHost
	if rdbHost == chromeinfra.ResultDBHost {
		miloHost = chromeinfra.MiloUIHost
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
			r.Flags.StringVar(&r.moduleName, "module-name", "", text.Doc(`
				Module name to upload test results to.
				Requires clients to supply structured test IDs to ReportTestResults.
				Set in conjunction with -module-scheme. May not be set in conjunction with
				-test-id-prefix.
			`))
			r.Flags.StringVar(&r.moduleScheme, "module-scheme", "", text.Doc(`
				Module scheme to upload test results to. See go/resultdb-schemes.
			`))
			r.Flags.StringVar(&r.testIDPrefix, "test-id-prefix", "", text.Doc(`
				Prefix to prepend to the test ID of every test result.
				Deprecated. Requires clients to supply legacy test IDs to ReportTestResults.
				Prefer to use upload structured test IDs by setting -module-name and -module-scheme.
			`))
			r.Flags.StringVar(&r.previousTestIDPrefix, "previous-test-id-prefix", previousTestIDPrefixDisabledValue, text.Doc(`
				Sets the test ID prefix that was previously used for these tests.
				This prefix will be combined with the legacy test ID reported to
				ReportTestResults and used to populate test_metadata.previous_test_id.

				When used in conjunction with test harnesses that report both structured
				and legacy test IDs to ReportTestResults, facilitates migration from legacy
				test IDs to structured test IDs.
				Omitting this flag or setting the value '`+previousTestIDPrefixDisabledValue+`'
				disables this feature.
			`))
			r.Flags.Var(flag.StringMap(r.vars), "var", text.Doc(`
				The module variant. This also sets the module variant when uploading
				test results with legacy test IDs.
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
				The command will fail if properties have already been set on
				the invocation (NOT ENFORCED YET).
			`))
			r.Flags.StringVar(&r.invPropertiesFile, "inv-properties-file", "", text.Doc(`
				Similar to -inv-properties but takes a path to the file that contains the JSON object.
				Cannot be used when -inv-properties is specified.
			`))
			r.Flags.StringVar(&r.invExtendedPropertiesDir, "inv-extended-properties-dir", "", text.Doc(`
				Path to a directory that contains files for the invocation's extended_properties in JSON format.
				The directory will only be read after the test command has run.
				Only files directly under this dir with the extension ".jsonpb" will be
				read. The filename after removing ".jsonpb" and the file content will be
				added as a key-value pair to the invocation's extended_properties map.
			`))
			r.Flags.BoolVar(&r.inheritSources, "inherit-sources", false, text.Doc(`
				If true, sets that the invocation inherits the code sources tested from its
				parent invocation (source_spec.inherit = true).
				If false, does not alter the invocation.
				Cannot be used in conjunction with -sources or -sources-file.
				This command will fail if the source_spec has already been set
				on the invocation (NOT ENFORCED YET).
			`))
			r.Flags.TextVar(&r.sources, "sources", &sources{}, text.Doc(`
				JSON-serialized luci.resultdb.v1.Sources object that
				contains information about the code sources tested by the
				invocation.
				Cannot be used in conjunction with -inherit-sources or -sources-file.
				This command will fail if the source_spec has already been set
				on the invocation (NOT ENFORCED YET).
			`))
			r.Flags.StringVar(&r.sourcesFile, "sources-file", "", text.Doc(`
				Similar to -sources, but takes the path to a file that
				contains the JSON-serialized luci.resultdb.v1.Sources
				object.
				Cannot be used in combination with -sources or -inherit-sources.
			`))
			r.Flags.StringVar(&r.baselineID, "baseline-id", "", text.Doc(`
				Baseline identifier for this invocation, usually of the format
				{buildbucket bucket}:{buildbucket builder name}.
				For example, 'try:linux-rel'.
			`))
			r.Flags.BoolVar(&r.shortenIDs, "shorten-ids", false, text.Doc(`
				Set this option to shorten uploaded test IDs to 350 bytes
				before being combined with the test ID prefix / module name.

				Warning: shortened test IDs will no longer match the ID
				reported by the source test harness, which may ability to
				inject IDs into commands (e.g. for reproduction instructions).
				Before resorting to this option, prefer to shorten the
				test IDs declared in source code instead.
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

type sources struct {
	*pb.Sources
}

// Implements encoding.TextUnmarshaler.
func (s *sources) UnmarshalText(text []byte) error {
	// Treat empty text as nil. This indicates that the code sources
	// tested are not specified and will not be updated.
	// '{}' means the sources should be set to an empty object.
	if len(text) == 0 {
		s.Sources = nil
		return nil
	}

	sources := &pb.Sources{}
	if err := protojson.Unmarshal(text, sources); err != nil {
		return err
	}
	s.Sources = sources
	return nil
}

// Implements encoding.TextMarshaler.
func (s *sources) MarshalText() (text []byte, err error) {
	// Serialize nil struct to empty string so nil struct won't be serialized as
	// '{}'.
	if s.Sources == nil {
		return nil, nil
	}

	return protojson.Marshal(s.Sources)
}

type streamRun struct {
	baseCommandRun

	// flags
	isNew                    bool
	isIncluded               bool
	realm                    string
	moduleName               string
	moduleScheme             string
	previousTestIDPrefix     string
	testIDPrefix             string // deprecated
	testTestLocationBase     string
	vars                     map[string]string
	artChannelMaxLeases      uint
	trChannelMaxLeases       uint
	tags                     strpair.Map
	coerceNegativeDuration   bool
	locTagsFile              string
	exonerateUnexpectedPass  bool
	invPropertiesFile        string
	invProperties            invProperties
	invExtendedPropertiesDir string
	inheritSources           bool
	sourcesFile              string
	sources                  sources
	baselineID               string
	instructionFile          string
	shortenIDs               bool
	// TODO(ddoman): add flags
	// - invocation-tag
	// - log-file

	invocation *lucictx.ResultDBInvocation
}

func (r *streamRun) validate(ctx context.Context, args []string) (err error) {
	if len(args) == 0 {
		return errors.New("missing a test command to run")
	}
	if err := pbutil.ValidateVariant(&pb.Variant{Def: r.vars}); err != nil {
		return errors.Fmt("invalid variant: %w", err)
	}
	if r.realm != "" {
		if err := realms.ValidateRealmName(r.realm, realms.GlobalScope); err != nil {
			return errors.Fmt("invalid realm: %w", err)
		}
	}
	if r.invProperties.Struct != nil && r.invPropertiesFile != "" {
		return errors.New("cannot specify both -inv-properties and -inv-properties-file at the same time")
	}
	if r.moduleName != "" {
		if err := pbutil.ValidateModuleName(r.moduleName); err != nil {
			return errors.Fmt("invalid module name: %w", err)
		}
		if r.moduleName == pbutil.LegacyModuleName {
			return errors.Fmt("-module-name cannot be %q", pbutil.LegacyModuleName)
		}
		if r.moduleScheme == "" {
			return errors.New("-module-name requires -module-scheme to also be specified")
		}
		if err := pbutil.ValidateModuleScheme(r.moduleScheme, false /*isLegacyModule*/); err != nil {
			return errors.Fmt("invalid module scheme: %w", err)
		}
	} else {
		if r.moduleScheme != "" {
			return errors.New("-module-scheme requires -module-name to also be specified")
		}
	}

	sourceSpecs := 0
	if r.sources.Sources != nil {
		sourceSpecs++
	}
	if r.sourcesFile != "" {
		sourceSpecs++
	}
	if r.inheritSources {
		sourceSpecs++
	}
	if sourceSpecs > 1 {
		return errors.New("cannot specify more than one of -inherit-sources, -sources and -sources-file at the same time")
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
			return r.done(errors.New("-realm is required for new invocations"))
		}
		loginMode = auth.SilentLogin
	}
	if err := r.initClients(ctx, loginMode); err != nil {
		return r.done(err)
	}

	// Fetch details of the module scheme from the ResultDB host.
	// This allows us to locally validate test IDs against the scheme and push
	// back against any invalid test IDs before we buffer them for async
	// upload.
	scheme, err := r.fetchScheme(ctx)
	if err != nil {
		return r.done(errors.Fmt("fetch scheme: %w", err))
	}

	// if -new is passed, create a new invocation. If not, use the existing one set in
	// lucictx.
	switch {
	case r.isNew:
		if r.isIncluded && r.resultdbCtx == nil {
			return r.done(errors.New("missing an invocation in LUCI_CONTEXT, but -include was given"))
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
		return r.done(errors.New("-new is required for -include"))
	case r.resultdbCtx == nil:
		return r.done(errors.New("missing an invocation in LUCI_CONTEXT; use -new to create a new one"))
	default:
		if err := r.validateCurrentInvocation(); err != nil {
			return r.done(err)
		}
		r.invocation = r.resultdbCtx.CurrentInvocation
	}

	invProperties, err := r.invPropertiesFromArgs(ctx)
	if err != nil {
		return r.done(errors.Fmt("get invocation properties from arguments: %w", err))
	}
	sourceSpec, err := r.sourceSpecFromArgs(ctx)
	if err != nil {
		return r.done(errors.Fmt("get source spec from arguments: %w", err))
	}
	moduleID := r.moduleIDFromArgs()

	if err := r.updateInvocation(ctx, moduleID, invProperties, sourceSpec, r.baselineID); err != nil {
		return r.done(err)
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

	err = r.runTestCmd(ctx, args, scheme)
	ec, ok := exitcode.Get(err)
	if !ok {
		logging.Errorf(ctx, "rdb-stream: failed to run the test command: %s", err)
		return r.done(err)
	}

	// The extended_properties files will be generated by test cmd so read them
	// after test cmd finishes.
	invExtendedProperties, err := r.invExtendedPropertiesFromArgs(ctx)
	if err != nil {
		return r.done(errors.Fmt("get invocation extended_properties from arguments: %w", err))
	}
	if invExtendedProperties != nil {
		if err := r.updateInvocationExtendedProperties(ctx, invExtendedProperties); err != nil {
			return r.done(err)
		}
	}

	logging.Infof(ctx, "rdb-stream: exiting with %d", ec)
	return ec
}

func (r *streamRun) runTestCmd(ctx context.Context, args []string, scheme *schemes.Scheme) error {
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
	// SIGKILLed by the expiration of cmdCtx.
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
		return errors.Fmt("get location tags: %w", err)
	}
	// TODO(ddoman): send the logs of SinkServer to --log-file

	cfg := sink.ServerConfig{
		Recorder:             r.recorder,
		ArtifactStreamClient: r.http,

		ArtChannelMaxLeases:        r.artChannelMaxLeases,
		ArtifactStreamHost:         r.host,
		TestResultChannelMaxLeases: r.trChannelMaxLeases,

		Invocation:  r.invocation.Name,
		UpdateToken: r.invocation.UpdateToken,

		TestIDPrefix:            r.testIDPrefix,
		ModuleName:              r.moduleName,
		ModuleScheme:            scheme,
		PreviousTestIDPrefix:    r.previousTestIDPrefixFromArg(),
		BaseTags:                pbutil.FromStrpairMap(r.tags),
		Variant:                 &pb.Variant{Def: r.vars},
		CoerceNegativeDuration:  r.coerceNegativeDuration,
		LocationTags:            locationTags,
		TestLocationBase:        r.testTestLocationBase,
		ExonerateUnexpectedPass: r.exonerateUnexpectedPass,
		ShortenIDs:              r.shortenIDs,
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
			logging.Warningf(ctx, "rdb-stream: failed to start test process: %s", err)
			return errors.Fmt("cmd.start: %w", err)
		}
		err = cmd.Wait()
		if err != nil {
			logging.Warningf(ctx, "rdb-stream: test process exited with error: %s", err)
			return err
		}
		return nil
	})
}

func (r *streamRun) fetchScheme(ctx context.Context) (*schemes.Scheme, error) {
	schemeToFetch := r.moduleScheme
	if r.moduleScheme == "" {
		// Use of structured test result uploads not enabled.
		return schemes.LegacyScheme, nil
	}

	var scheme *pb.Scheme
	doFetchScheme := func() error {
		result, err := r.schemas.GetScheme(ctx, &pb.GetSchemeRequest{
			Name: fmt.Sprintf("schema/schemes/%s", r.moduleScheme),
		})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				logging.Errorf(ctx, "Module scheme %q is not known by the ResultDB deployment. Refer to go/resultdb-schemes for information about valid schemes and/or update the value passed to -module-scheme.", schemeToFetch)
			}
			// Tag transient response codes with transient.Tag so that
			// they can be retried.
			return grpcutil.WrapIfTransient(err)
		}
		scheme = result
		return nil
	}

	// Configure the retry
	err := retry.Retry(ctx, transient.Only(func() retry.Iterator {
		// Start at one second, and increase exponentially to
		// 32 seconds.
		return &retry.ExponentialBackoff{
			Limited: retry.Limited{
				Delay:   1 * time.Second, // initial delay time
				Retries: 6,               // number of retries
			},
			Multiplier: 2,                // backoff multiplier
			MaxDelay:   32 * time.Second, // maximum delay time
		}
	}), doFetchScheme, nil)
	if err != nil {
		return nil, err
	}
	return schemes.FromProto(scheme)
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

func (r *streamRun) previousTestIDPrefixFromArg() *string {
	if r.previousTestIDPrefix == previousTestIDPrefixDisabledValue {
		return nil
	}
	val := r.previousTestIDPrefix
	return &val
}

// invPropertiesFromArgs gets invocation-level properties from arguments.
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
	if err != nil {
		return nil, errors.Fmt("read file: %w", err)
	}

	properties := &structpb.Struct{}
	if err = protojson.Unmarshal(f, properties); err != nil {
		return nil, errors.Fmt("unmarshal file: %w", err)
	}

	return properties, nil
}

// invExtendedPropertiesFromArgs gets invocation-level extended properties from
// arguments.
func (r *streamRun) invExtendedPropertiesFromArgs(ctx context.Context) (map[string]*structpb.Struct, error) {
	if r.invExtendedPropertiesDir == "" {
		return nil, nil
	}

	files, err := os.ReadDir(r.invExtendedPropertiesDir)
	if err != nil {
		return nil, errors.Fmt("read invocation extended properties directory %q: %w", r.invExtendedPropertiesDir, err)
	}
	extendedProperties := make(map[string]*structpb.Struct)
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".jsonpb") {
			continue
		}
		extPropKey := strings.TrimSuffix(file.Name(), ".jsonpb")
		fileFullPath := filepath.Join(r.invExtendedPropertiesDir, file.Name())
		f, err := os.ReadFile(fileFullPath)
		if err != nil {
			return nil, errors.Fmt("read file %q: %w", fileFullPath, err)
		}
		extPropValue := &structpb.Struct{}
		if err = protojson.Unmarshal(f, extPropValue); err != nil {
			return nil, errors.Fmt("unmarshal file %q: %w", fileFullPath, err)
		}
		extendedProperties[extPropKey] = extPropValue
	}
	return extendedProperties, nil
}

// sourceSpecFromArgs gets the invocation source spec from arguments.
// Return nil if none is set.
func (r *streamRun) sourceSpecFromArgs(ctx context.Context) (*pb.SourceSpec, error) {
	if r.sources.Sources != nil {
		return &pb.SourceSpec{Sources: r.sources.Sources}, nil
	}
	if r.inheritSources {
		return &pb.SourceSpec{Inherit: true}, nil
	}

	if r.sourcesFile == "" {
		return nil, nil
	}

	f, err := os.ReadFile(r.sourcesFile)
	if err != nil {
		return nil, errors.Fmt("read file: %w", err)
	}

	sources := &pb.Sources{}
	if err = protojson.Unmarshal(f, sources); err != nil {
		return nil, errors.Fmt("unmarshal file: %w", err)
	}

	return &pb.SourceSpec{Sources: sources}, nil
}

func (r *streamRun) moduleIDFromArgs() *pb.ModuleIdentifier {
	if r.moduleName != "" {
		// We're using structured test IDs.
		return &pb.ModuleIdentifier{
			ModuleName:    r.moduleName,
			ModuleScheme:  r.moduleScheme,
			ModuleVariant: &pb.Variant{Def: r.vars},
		}
	} else {
		return &pb.ModuleIdentifier{
			ModuleName:   "legacy",
			ModuleScheme: "legacy",
			// Do not set vars, it is possible legacy clients are calling `rdb stream`
			// multiple times within one invocation and this will cause attempts to
			// update the module variant after it has already been set, which will
			// cause errors.
		}
	}
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
		err = errors.Fmt("failed to create an invocation: %w", err)
		return
	}
	tks := md.Get(pb.UpdateTokenMetadataKey)
	if len(tks) == 0 {
		err = errors.Fmt("Missing header: %s", pb.UpdateTokenMetadataKey)
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

// updateInvocation sets the extended properties on the invocation.
func (r *streamRun) updateInvocationExtendedProperties(ctx context.Context, extendedProperties map[string]*structpb.Struct) error {
	ctx = metadata.AppendToOutgoingContext(ctx, pb.UpdateTokenMetadataKey, r.invocation.UpdateToken)
	request := &pb.UpdateInvocationRequest{
		Invocation: &pb.Invocation{
			Name:               r.invocation.Name,
			ExtendedProperties: extendedProperties,
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"extended_properties"}},
	}
	_, err := r.recorder.UpdateInvocation(ctx, request)
	return err
}

// updateInvocation sets the module ID, properties, baseline ID and/or source spec on the invocation.
func (r *streamRun) updateInvocation(
	ctx context.Context,
	moduleID *pb.ModuleIdentifier,
	properties *structpb.Struct,
	sourceSpec *pb.SourceSpec,
	baselineID string) error {
	ctx = metadata.AppendToOutgoingContext(ctx, pb.UpdateTokenMetadataKey, r.invocation.UpdateToken)
	request := &pb.UpdateInvocationRequest{
		Invocation: &pb.Invocation{
			Name: r.invocation.Name,
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{}},
	}
	if moduleID != nil {
		request.Invocation.ModuleId = moduleID
		request.UpdateMask.Paths = append(request.UpdateMask.Paths, "module_id")
	}
	if properties != nil {
		request.Invocation.Properties = properties
		request.UpdateMask.Paths = append(request.UpdateMask.Paths, "properties")
	}
	if sourceSpec != nil {
		request.Invocation.SourceSpec = sourceSpec
		request.UpdateMask.Paths = append(request.UpdateMask.Paths, "source_spec")
	}
	if baselineID != "" {
		request.Invocation.BaselineId = baselineID
		request.UpdateMask.Paths = append(request.UpdateMask.Paths, "baseline_id")
	}
	if len(request.UpdateMask.Paths) > 0 {
		_, err := r.recorder.UpdateInvocation(ctx, request)
		return err
	}
	return nil
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
