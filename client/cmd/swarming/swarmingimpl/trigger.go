// Copyright 2016 The LUCI Authors.
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

package swarmingimpl

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/google/uuid"
	"github.com/maruel/subcommands"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/base"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/clipb"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/output"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/flagenum"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/flag/stringmapflag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/swarming/client/swarming"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

// CmdTrigger returns an object for the `trigger` subcommand.
func CmdTrigger(authFlags base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "trigger -S <server> <parameters>",
		ShortDesc: "triggers a single Swarming task",
		LongDesc:  "Triggers a single Swarming task.",
		CommandRun: func() subcommands.CommandRun {
			return base.NewCommandRun(authFlags, &triggerImpl{}, base.Features{
				MinArgs:         0,
				MaxArgs:         base.Unlimited,
				MeasureDuration: true,
				OutputJSON: base.OutputJSON{
					Enabled:             true,
					DeprecatedAliasFlag: "dump-json",
					DefaultToStdout:     false,
				},
			})
		},
	}
}

// mapToArray converts a stringmapflag.Value into an array of
// swarmingv2.StringPair, sorted by key and then value.
//
// TODO(vadimsh): Move to utils.
func mapToArray(m stringmapflag.Value) []*swarmingv2.StringPair {
	a := make([]*swarmingv2.StringPair, 0, len(m))
	for k, v := range m {
		a = append(a, &swarmingv2.StringPair{Key: k, Value: v})
	}

	sort.Slice(a, func(i, j int) bool {
		return a[i].Key < a[j].Key ||
			(a[i].Key == a[j].Key && a[i].Value < a[j].Value)
	})
	return a
}

// listToStringListPairArray converts a stringlistflag.Flag into an array of
// swarmingv2.StringListPair, sorted by key.
//
// TODO(vadimsh): Move to utils.
func listToStringListPairArray(m stringlistflag.Flag) []*swarmingv2.StringListPair {
	prefixes := make(map[string][]string)
	for _, f := range m {
		kv := strings.SplitN(f, "=", 2)
		prefixes[kv[0]] = append(prefixes[kv[0]], kv[1])
	}

	a := make([]*swarmingv2.StringListPair, 0, len(prefixes))

	for key, value := range prefixes {
		a = append(a, &swarmingv2.StringListPair{
			Key:   key,
			Value: value,
		})
	}

	sort.Slice(a, func(i, j int) bool {
		return a[i].Key < a[j].Key
	})
	return a
}

// namePartFromDimensions creates a string from a map of dimensions that can
// be used as part of the task name.  The dimensions are first sorted as
// described in mapToArray().
//
// TODO(vadimsh): Move to utils.
func namePartFromDimensions(m stringmapflag.Value) string {
	a := mapToArray(m)
	pairs := make([]string, 0, len(a))
	for i := 0; i < len(a); i++ {
		pairs = append(pairs, fmt.Sprintf("%s=%s", a[i].Key, a[i].Value))
	}
	return strings.Join(pairs, "_")
}

type containmentType string

func (c *containmentType) String() string {
	return string(*c)
}

func (c *containmentType) Set(v string) error {
	return containmentChoices.FlagSet(c, v)
}

var containmentChoices = flagenum.Enum{
	"none":          containmentType("NONE"),
	"auto":          containmentType("AUTO"),
	"job_object":    containmentType("JOB_OBJECT"),
	"not_specified": containmentType("NOT_SPECIFIED"),
}

type optionalDimension struct {
	kv         *swarmingv2.StringPair
	expiration int64
}

var _ flag.Value = (*optionalDimension)(nil)

// String implements the flag.Value interface.
func (f *optionalDimension) String() string {
	if f == nil || f.isEmpty() {
		return ""
	}
	return fmt.Sprintf("%s=%s:%d", f.kv.Key, f.kv.Value, f.expiration)
}

// Set implements the flag.Value interface.
func (f *optionalDimension) Set(s string) error {
	if s == "" {
		return nil
	}
	splits := strings.SplitN(s, "=", 2)

	if len(splits) != 2 {
		return errors.Reason("cannot find key in the optional dimension: %q", s).Err()
	}
	k := splits[0]
	valExp := splits[1]
	colon := strings.LastIndexByte(valExp, ':')
	if colon == -1 {
		return errors.Reason(`cannot find ":" between value and expiration in the optional dimension: %q`, valExp).Err()
	}
	exp, err := strconv.ParseInt(valExp[colon+1:], 10, 64)
	if err != nil {
		return errors.Reason("cannot parse the expiration in the optional dimension: %q", valExp).Err()
	}
	f.kv = &swarmingv2.StringPair{Key: k, Value: valExp[:colon]}
	f.expiration = exp
	return nil
}

func (f *optionalDimension) isEmpty() bool {
	return f.kv == nil
}

type triggerImpl struct {
	// Task properties.
	casInstance       string
	digest            string
	dimensions        stringmapflag.Value
	env               stringmapflag.Value
	envPrefix         stringlistflag.Flag
	idempotent        bool
	containmentType   containmentType
	namedCache        stringmapflag.Value
	hardTimeout       int
	ioTimeout         int
	gracePeriod       int
	cipdPackage       stringmapflag.Value
	outputs           stringlistflag.Flag
	optionalDimension optionalDimension
	serviceAccount    string
	relativeCwd       string
	secretBytesPath   string

	// Task request.
	taskName       string
	priority       int
	tags           stringlistflag.Flag
	user           string
	expiration     int
	enableResultDB bool
	realm          string
	cmd            []string

	// Env.
	parentTaskID string
}

func (cmd *triggerImpl) RegisterFlags(fs *flag.FlagSet) {
	// Task properties.
	fs.StringVar(&cmd.casInstance, "cas-instance", "", "CAS instance (GCP). Format is \"projects/<project_id>/instances/<instance_id>\". Default is constructed from -server.")
	fs.StringVar(&cmd.digest, "digest", "", "Digest of root directory uploaded to CAS `<Hash>/<Size>`.")
	fs.Var(&cmd.dimensions, "dimension", "Dimension to select the right kind of bot. In the form of `key=value`")
	fs.Var(&cmd.dimensions, "d", "Alias for -dimension.")
	fs.Var(&cmd.env, "env", "Environment variables to set.")
	fs.Var(&cmd.envPrefix, "env-prefix", "Environment prefixes to set.")
	fs.BoolVar(&cmd.idempotent, "idempotent", false, "When set, the server will actively try to find a previous task with the same parameter and return this result instead if possible.")
	cmd.containmentType = "NONE"
	fs.Var(&cmd.containmentType, "containment-type", "Specify which type of process containment to use. Choices are: "+containmentChoices.Choices())
	fs.IntVar(&cmd.hardTimeout, "hard-timeout", 60*60, "Seconds to allow the task to complete.")
	fs.IntVar(&cmd.ioTimeout, "io-timeout", 20*60, "Seconds to allow the task to be silent.")
	fs.IntVar(&cmd.gracePeriod, "grace-period", 30, "Seconds to wait after sending SIGBREAK.")
	fs.Var(&cmd.cipdPackage, "cipd-package",
		"(repeatable) CIPD packages to install on the swarming bot. This takes a parameter of `[installdir:]pkgname=version`. "+
			"Using an empty version will remove the package. The installdir is optional and defaults to '.'.")
	fs.Var(&cmd.namedCache, "named-cache", "This takes a parameter of `name=cachedir`.")
	fs.Var(&cmd.outputs, "output", "(repeatable) Specify an output file or directory that can be retrieved via collect.")
	fs.Var(&cmd.optionalDimension, "optional-dimension", "Format: <key>=<value>:<expiration>. See -expiration for the requirement.")
	fs.StringVar(&cmd.relativeCwd, "relative-cwd", "", "Use this flag instead of the isolated 'relative_cwd'.")
	fs.StringVar(&cmd.serviceAccount, "service-account", "",
		`Email of a service account to run the task as, or literal "bot" string to indicate that the task should use the same account the bot itself is using to authenticate to Swarming. Don't use task service accounts if not given (default).`)
	fs.StringVar(&cmd.secretBytesPath, "secret-bytes-path", "", "Specify the secret bytes file path.")

	// Task request.
	fs.StringVar(&cmd.taskName, "task-name", "", "Display name of the task. Defaults to <base_name>/<dimensions>/<isolated hash>/<timestamp> if an  isolated file is provided, if a hash is provided, it defaults to <user>/<dimensions>/<isolated hash>/<timestamp>")
	fs.IntVar(&cmd.priority, "priority", 200, "The lower value, the more important the task.")
	fs.Var(&cmd.tags, "tag", "Tags to assign to the task. In the form of `key:value`.")
	fs.StringVar(&cmd.user, "user", "", "User associated with the task. Defaults to authenticated user on the server.")
	fs.IntVar(&cmd.expiration, "expiration", 6*60*60, "Seconds to allow the task to be pending for a bot to run before this task request expires.")
	fs.BoolVar(&cmd.enableResultDB, "enable-resultdb", false, "Enable ResultDB for this task.")
	fs.StringVar(&cmd.realm, "realm", "", "Realm name for this task.")
}

func (cmd *triggerImpl) ParseInputs(ctx context.Context, args []string, env subcommands.Env, extra base.Extra) error {
	if len(cmd.dimensions) == 0 {
		return errors.Reason("please specify at least one dimension via -dimension").Err()
	}
	if len(args) == 0 {
		return errors.Reason("please specify command after '--'").Err()
	}
	if len(cmd.user) == 0 {
		cmd.user = env[swarming.UserEnvVar].Value
	}
	cmd.cmd = args
	cmd.parentTaskID = env[swarming.TaskIDEnvVar].Value
	return nil
}

func (cmd *triggerImpl) Execute(ctx context.Context, svc swarming.Client, sink *output.Sink, extra base.Extra) error {
	request, err := cmd.processTriggerOptions(cmd.cmd, extra.ServerURL)
	if err != nil {
		return errors.Annotate(err, "failed to process trigger options").Err()
	}

	result, err := svc.NewTask(ctx, request)
	if err != nil {
		return err
	}

	if extra.OutputJSON != "" {
		logging.Infof(ctx, "To collect results use:\n"+
			"  %s collect -server %s -output-dir out -task-summary-json summary.json -requests-json %s",
			os.Args[0], extra.ServerURL, extra.OutputJSON)
	} else {
		logging.Infof(ctx, "To collect results use:\n"+
			"  %s collect -server %s -output-dir out -task-summary-json summary.json %s",
			os.Args[0], extra.ServerURL, result.TaskId)
	}
	logging.Infof(ctx, "The task status:\n"+
		"  %s/task?id=%s\n", extra.ServerURL, result.TaskId)

	return output.Proto(sink, &clipb.SpawnTasksOutput{
		Tasks: []*swarmingv2.TaskRequestMetadataResponse{result},
	})
}

func (cmd *triggerImpl) createTaskSliceForOptionalDimension(properties *swarmingv2.TaskProperties) (*swarmingv2.TaskSlice, error) {
	if cmd.optionalDimension.isEmpty() {
		return nil, nil
	}
	optDim := cmd.optionalDimension.kv
	exp := cmd.optionalDimension.expiration

	propsCpy := proto.Clone(properties).(*swarmingv2.TaskProperties)
	propsCpy.Dimensions = append(propsCpy.Dimensions, optDim)

	return &swarmingv2.TaskSlice{
		ExpirationSecs: int32(exp),
		Properties:     propsCpy,
	}, nil
}

func (cmd *triggerImpl) processTriggerOptions(commands []string, serverURL *url.URL) (*swarmingv2.NewTaskRequest, error) {
	if cmd.taskName == "" {
		cmd.taskName = fmt.Sprintf("%s/%s", cmd.user, namePartFromDimensions(cmd.dimensions))
	}

	var err error
	var secretBytes []byte
	if cmd.secretBytesPath != "" {
		secretBytes, err = os.ReadFile(cmd.secretBytesPath)
		if err != nil {
			return nil, errors.Annotate(err, "failed to read secret bytes from %s", cmd.secretBytesPath).Err()
		}
	}

	var CASRef *swarmingv2.CASReference
	if cmd.digest != "" {
		d, err := digest.NewFromString(cmd.digest)
		if err != nil {
			return nil, errors.Annotate(err, "invalid digest: %s", cmd.digest).Err()
		}

		casInstance := cmd.casInstance
		if casInstance == "" {
			// Infer CAS instance from the swarming server URL.
			const appspot = ".appspot.com"
			if !strings.HasSuffix(serverURL.Host, appspot) {
				return nil, errors.Reason("server url should have '%s' suffix: %s", appspot, serverURL).Err()
			}
			casInstance = "projects/" + strings.TrimSuffix(serverURL.Host, appspot) + "/instances/default_instance"
		}

		CASRef = &swarmingv2.CASReference{
			CasInstance: casInstance,
			Digest: &swarmingv2.Digest{
				Hash:      d.Hash,
				SizeBytes: d.Size,
			},
		}
	}

	properties := swarmingv2.TaskProperties{
		Command:              commands,
		RelativeCwd:          cmd.relativeCwd,
		Dimensions:           mapToArray(cmd.dimensions),
		Env:                  mapToArray(cmd.env),
		EnvPrefixes:          listToStringListPairArray(cmd.envPrefix),
		ExecutionTimeoutSecs: int32(cmd.hardTimeout),
		GracePeriodSecs:      int32(cmd.gracePeriod),
		Idempotent:           cmd.idempotent,
		CasInputRoot:         CASRef,
		Outputs:              cmd.outputs,
		IoTimeoutSecs:        int32(cmd.ioTimeout),
		Containment: &swarmingv2.Containment{
			ContainmentType: swarmingv2.Containment_ContainmentType(swarmingv2.Containment_ContainmentType_value[cmd.containmentType.String()]),
		},
		SecretBytes: secretBytes,
	}

	if len(cmd.cipdPackage) > 0 {
		var pkgs []*swarmingv2.CipdPackage
		for k, v := range cmd.cipdPackage {
			s := strings.SplitN(k, ":", 2)
			pkg := swarmingv2.CipdPackage{
				PackageName: s[len(s)-1],
				Version:     v,
			}
			if len(s) > 1 {
				pkg.Path = s[0]
			}
			pkgs = append(pkgs, &pkg)
		}

		sort.Slice(pkgs, func(i, j int) bool {
			pi, pj := pkgs[i], pkgs[j]
			if pi.PackageName != pj.PackageName {
				return pi.PackageName < pj.PackageName
			}
			if pi.Version != pj.Version {
				return pi.Version < pj.Version
			}
			return pi.Path < pj.Path
		})

		properties.CipdInput = &swarmingv2.CipdInput{Packages: pkgs}
	}

	for name, path := range cmd.namedCache {
		properties.Caches = append(properties.Caches,
			&swarmingv2.CacheEntry{
				Name: name,
				Path: path,
			},
		)
	}

	sort.Slice(properties.Caches, func(i, j int) bool {
		ci, cj := properties.Caches[i], properties.Caches[j]
		if ci.Name != cj.Name {
			return ci.Name < cj.Name
		}
		return ci.Path < cj.Path
	})

	randomUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, errors.Annotate(err, "failed to get random UUID").Err()
	}

	var taskSlices []*swarmingv2.TaskSlice
	taskSlice, err := cmd.createTaskSliceForOptionalDimension(&properties)
	if err != nil {
		return nil, errors.Annotate(err, "failed to createTaskSliceForOptionalDimension").Err()
	}
	baseExpiration := int32(cmd.expiration)
	if taskSlice != nil {
		taskSlices = append(taskSlices, taskSlice)

		baseExpiration -= taskSlice.ExpirationSecs
		if baseExpiration < 60 {
			baseExpiration = 60
		}
	}
	taskSlices = append(taskSlices, &swarmingv2.TaskSlice{
		ExpirationSecs: int32(baseExpiration),
		Properties:     &properties,
	})

	return &swarmingv2.NewTaskRequest{
		Name:           cmd.taskName,
		ParentTaskId:   cmd.parentTaskID,
		Priority:       int32(cmd.priority),
		ServiceAccount: cmd.serviceAccount,
		Tags:           cmd.tags,
		TaskSlices:     taskSlices,
		User:           cmd.user,
		RequestUuid:    randomUUID.String(),
		Resultdb: &swarmingv2.ResultDBCfg{
			Enable: cmd.enableResultDB,
		},
		Realm: cmd.realm,
	}, nil
}
