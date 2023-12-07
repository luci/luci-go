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

package tq

import (
	"context"
	"flag"
	"strings"

	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/tq/internal/db"
	"go.chromium.org/luci/server/tq/tqtesting"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/tq")

// ModuleOptions contain configuration of the TQ server module.
//
// It will be used to initialize Default dispatcher.
type ModuleOptions struct {
	// Dispatcher is a dispatcher to use.
	//
	// Default is the global Default instance.
	Dispatcher *Dispatcher

	// CloudProject is ID of a project to use to construct full queue names.
	//
	// Default is the project the server is running in.
	CloudProject string

	// CloudRegion is a ID of a region to use to construct full queue names.
	//
	// Default is the region the server is running in.
	CloudRegion string

	// Namespace is a namespace for tasks that use DeduplicationKey.
	//
	// This is needed if two otherwise independent deployments share a single
	// Cloud Tasks instance.
	//
	// Default is "".
	Namespace string

	// DefaultTargetHost is a hostname to dispatch Cloud Tasks to by default.
	//
	// Individual task classes may override it with their own specific host.
	//
	// On GAE defaults to the GAE application itself. Elsewhere has no default:
	// if the dispatcher can't figure out where to send the task, the task
	// submission fails.
	DefaultTargetHost string

	// PushAs is a service account email to be used for generating OIDC tokens.
	//
	// The service account must be within the same project. The server account
	// must have "iam.serviceAccounts.actAs" permission for `PushAs` account.
	//
	// Default is the server's own account.
	PushAs string

	// AuthorizedPushers is a list of service account emails to accept pushes from
	// in addition to PushAs.
	//
	// This is handy when migrating from one PushAs account to another, or when
	// submitting tasks from one service, but handing them in another.
	//
	// Optional.
	AuthorizedPushers []string

	// ServingPrefix is a URL path prefix to serve registered task handlers from.
	//
	// POSTs to a URL under this prefix (regardless which one) will be treated
	// as Cloud Tasks pushes.
	//
	// Must start with "/internal/". Default is "/internal/tasks". If set to
	// literal "-", no routes will be registered at all.
	ServingPrefix string

	// SweepMode defines how to perform sweeps of the transaction tasks reminders.
	//
	// This process is necessary to make sure all transactionally submitted tasks
	// eventually execute, even if Cloud Tasks RPCs fail. When enqueueing a task
	// the client transactionally commits a special "reminder" record, which
	// indicates an intent to submit a Cloud Task. If the subsequent Cloud Tasks
	// RPC fails (or the process crashes before attempting it), the reminder
	// record is discovered by the sweep process and used to ensure the task is
	// eventually submitted.
	//
	// There are two stages: the sweep initiation and the actual processing.
	//
	// The initiation should happen periodically and centrally: no mater how many
	// replicas of the process are running, there needs to be only one sweep
	// initiator. But it doesn't have to be the same process each time. Also
	// multiple concurrent initiations are not catastrophic, though they impose
	// huge overhead and should be avoided.
	//
	// Two ways to do sweep initiations are:
	//   * Based on a periodic external signal such as a Cloud Scheduler job or
	//     GAE cron handler. See SweepInitiationEndpoint and
	//     SweepInitiationLaunchers.
	//   * Based on a timer inside some *single* primary process. For example
	//     on Kubernetes this may be a single pod Deployment, or a zero-indexed
	//     replica in a StatefulSet. See Sweep().
	//
	// Once the initiation happens, there are two ways to process the sweep (and
	// this is what SweepMode defines):
	//   * "inproc" - do all the processing right inside the replica that
	//     performed the initiation. This has scalability and reliability limits,
	//     but it doesn't require any additional infrastructure setup and has
	//     somewhat better observability.
	//   * "distributed" - use Cloud Tasks itself to distribute the work across
	//     many replicas. This requires some configuration. See SweepTaskQueue,
	//     SweepTaskPrefix and SweepTargetHost.
	//
	// Default is "distributed" mode.
	SweepMode string

	// SweepInitiationEndpoint is a URL path that can be hit to initiate a sweep.
	//
	// GET requests to this endpoint (if they have proper authentication headers)
	// will initiate sweeps. If SweepMode is "inproc" the sweep will happen in
	// the same process that handled the request.
	//
	// On GAE default is "/internal/tasks/c/sweep". On non-GAE it is "-", meaning
	// the endpoint is not exposed. When not using the endpoint there should be
	// some single process somewhere that calls Sweep() to periodically initiate
	// sweeps.
	SweepInitiationEndpoint string

	// SweepInitiationLaunchers is a list of service account emails authorized to
	// launch sweeps via SweepInitiationEndpoint.
	//
	// Additionally on GAE the Appengine service itself is always authorized to
	// launch sweeps via cron or task queues.
	//
	// Default is the server's own account.
	SweepInitiationLaunchers []string

	// SweepTaskQueue is a Cloud Tasks queue name to use to distribute sweep
	// subtasks when running in "distributed" SweepMode.
	//
	// Can be in short or full form. See Queue in TaskClass for details. The queue
	// should be configured to allow at least 10 QPS.
	//
	// Default is "tq-sweep".
	SweepTaskQueue string

	// SweepTaskPrefix is a URL prefix to use for sweep subtasks when running
	// in "distributed" SweepMode.
	//
	// There should be a Dispatcher instance somewhere that is configured to
	// receive such tasks (via non-default ServingPrefix). This is useful if
	// you want to limit what processes process the sweeps.
	//
	// Must start with "/internal/". If unset defaults to the value of
	// ServingPrefix.
	SweepTaskPrefix string

	// SweepTargetHost is a hostname to dispatch sweep subtasks to when running
	// in "distributed" SweepMode.
	//
	// This usually should be DefaultTargetHost, but it may be different if you
	// want to route sweep subtasks somewhere else.
	//
	// If unset defaults to the value of DefaultTargetHost.
	SweepTargetHost string

	// SweepShards defines how many subtasks are submitted when initiating
	// a sweep.
	//
	// It is safe to change it any time. Default is 16.
	SweepShards int
}

// Register registers the command line flags.
//
// Mutates `o` by populating defaults.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(&o.CloudProject, "tq-cloud-project", o.CloudProject,
		`Cloud Project to use to construct full queue names, default is the same as -cloud-project.`)

	f.StringVar(&o.CloudRegion, "tq-cloud-region", o.CloudRegion,
		`Cloud Region to use to construct full queue names, default is the same as -cloud-region.`)

	f.StringVar(&o.Namespace, "tq-namespace", o.Namespace,
		`Namespace for tasks that use deduplication keys (optional).`)

	f.StringVar(&o.DefaultTargetHost, "tq-default-target-host", o.DefaultTargetHost,
		`Hostname to dispatch Cloud Tasks to by default.`)

	f.StringVar(&o.PushAs, "tq-push-as", o.PushAs,
		`Service account email to be used for generating OIDC tokens. `+
			`Default is server's own account.`)

	f.Var(luciflag.StringSlice(&o.AuthorizedPushers), "tq-authorized-pusher",
		`Service account email to accept pushes from (in addition to -tq-push-as). May be repeated.`)

	if o.ServingPrefix == "" {
		o.ServingPrefix = "/internal/tasks"
	}
	f.StringVar(&o.ServingPrefix, "tq-serving-prefix", o.ServingPrefix,
		`URL prefix to serve registered task handlers from, must start with '/internal/'. Set to '-' to disable serving.`)

	if o.SweepMode == "" {
		o.SweepMode = "distributed"
	}
	f.StringVar(&o.SweepMode, "tq-sweep-mode", o.SweepMode,
		`How to do sweeps of transactional task reminders: either "distributed" or "inproc".`)

	f.StringVar(&o.SweepInitiationEndpoint, "tq-sweep-initiation-endpoint", o.SweepInitiationEndpoint,
		`URL path of an endpoint that launches sweeps.`)

	f.Var(luciflag.StringSlice(&o.SweepInitiationLaunchers), "tq-sweep-initiation-launcher",
		`Service account email allowed to hit -tq-sweep-initiation-endpoint. May be repeated.`)

	if o.SweepTaskQueue == "" {
		o.SweepTaskQueue = "tq-sweep"
	}
	f.StringVar(&o.SweepTaskQueue, "tq-sweep-task-queue", o.SweepTaskQueue,
		`A queue name to use to distribute sweep subtasks`)

	f.StringVar(&o.SweepTaskPrefix, "tq-sweep-task-prefix", o.SweepTaskPrefix,
		`URL prefix to use for sweep subtasks, must start with '/internal/'. Defaults to -tq-serving-prefix.`)

	f.StringVar(&o.SweepTargetHost, "tq-sweep-target-host", o.SweepTargetHost,
		`Hostname to dispatch sweep subtasks to. Defaults to -tq-default-target-host.`)

	if o.SweepShards == 0 {
		o.SweepShards = 16
	}
	f.IntVar(&o.SweepShards, "tq-sweep-shards", o.SweepShards,
		`How many subtasks are submitted when initiating a sweep.`)
}

// NewModule returns a server module that sets up a TQ dispatcher.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &tqModule{opts: opts}
}

// NewModuleFromFlags is a variant of NewModule that initializes options through
// command line flags.
//
// Calling this function registers flags in flag.CommandLine. They are usually
// parsed in server.Main(...).
func NewModuleFromFlags() module.Module {
	opts := &ModuleOptions{}
	opts.Register(flag.CommandLine)
	return NewModule(opts)
}

// tqModule implements module.Module.
type tqModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*tqModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*tqModule) Dependencies() []module.Dependency {
	var deps []module.Dependency
	db.VisitImpls(func(db *db.Impl) {
		if db.Module.Valid() {
			deps = append(deps, module.RequiredDependency(db.Module))
		}
	})
	return deps
}

// Initialize is part of module.Module interface.
func (m *tqModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	if m.opts.Dispatcher == nil {
		m.opts.Dispatcher = &Default
	}
	submitter, err := m.initDispatching(ctx, host, opts)
	if err != nil {
		return nil, err
	}
	if err := m.initSweeping(ctx, host, opts); err != nil {
		return nil, err
	}
	return UseSubmitter(ctx, submitter), nil
}

func (m *tqModule) initDispatching(ctx context.Context, host module.Host, opts module.HostOptions) (Submitter, error) {
	disp := m.opts.Dispatcher

	disp.GAE = opts.Serverless == module.GAE
	disp.DisableAuth = !opts.Prod
	disp.DefaultTargetHost = m.opts.DefaultTargetHost
	disp.AuthorizedPushers = m.opts.AuthorizedPushers

	disp.CloudProject = m.opts.CloudProject
	if disp.CloudProject == "" {
		disp.CloudProject = opts.CloudProject
	}
	disp.CloudRegion = m.opts.CloudRegion
	if disp.CloudRegion == "" {
		disp.CloudRegion = opts.CloudRegion
	}

	if err := ValidateNamespace(m.opts.Namespace); err != nil {
		return nil, errors.Annotate(err, "bad TQ namespace %q", m.opts.Namespace).Err()
	}
	disp.Namespace = m.opts.Namespace

	if m.opts.PushAs != "" {
		disp.PushAs = m.opts.PushAs
	} else {
		info, err := auth.GetSigner(ctx).ServiceInfo(ctx)
		if err != nil {
			return nil, errors.Annotate(err, "failed to get own service account email").Err()
		}
		disp.PushAs = info.ServiceAccountName
	}

	var submitter Submitter
	if opts.Prod {
		// When running for real use real services.
		creds, err := auth.GetPerRPCCredentials(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
		if err != nil {
			return nil, errors.Annotate(err, "failed to get PerRPCCredentials").Err()
		}
		cloudSub, err := NewCloudSubmitter(ctx, creds)
		if err != nil {
			return nil, err
		}
		host.RegisterCleanup(func(ctx context.Context) { cloudSub.Close() })
		submitter = cloudSub
	} else {
		// When running locally use a simple in-memory scheduler, but go through
		// HTTP layer to pick up logging, middlewares, etc.
		scheduler := &tqtesting.Scheduler{
			Executor: &tqtesting.LoopbackHTTPExecutor{
				ServerAddr: host.HTTPAddr(),
			},
		}
		host.RunInBackground("luci.tq", func(ctx context.Context) {
			scheduler.Run(ctx, tqtesting.ParallelExecute())
		})
		submitter = scheduler
	}

	if m.opts.ServingPrefix != "-" {
		logging.Infof(ctx, "TQ is serving tasks from %q", m.opts.ServingPrefix)
		if !strings.HasPrefix(m.opts.ServingPrefix, "/internal/") {
			return nil, errors.Reason(`-tq-serving-prefix must start with "/internal/", got %q`, m.opts.ServingPrefix).Err()
		}
		disp.InstallTasksRoutes(host.Routes(), m.opts.ServingPrefix)
	}

	// Report gauge metrics on flush.
	tsmon.RegisterCallbackIn(ctx, disp.ReportMetrics)

	return submitter, nil
}

func (m *tqModule) initSweeping(ctx context.Context, host module.Host, opts module.HostOptions) error {
	// Fill in defaults.
	if m.opts.SweepInitiationEndpoint == "" {
		if opts.Serverless == module.GAE || !opts.Prod {
			m.opts.SweepInitiationEndpoint = "/internal/tasks/c/sweep"
		} else {
			m.opts.SweepInitiationEndpoint = "-"
		}
	}

	if len(m.opts.SweepInitiationLaunchers) == 0 {
		info, err := auth.GetSigner(ctx).ServiceInfo(ctx)
		if err != nil {
			return errors.Annotate(err, "failed to get own service account email").Err()
		}
		m.opts.SweepInitiationLaunchers = []string{info.ServiceAccountName}
	}

	if m.opts.SweepTaskPrefix == "" {
		if m.opts.ServingPrefix != "-" {
			m.opts.SweepTaskPrefix = m.opts.ServingPrefix
		} else {
			m.opts.SweepTaskPrefix = "/internal/tasks"
		}
	}
	if !strings.HasPrefix(m.opts.SweepTaskPrefix, "/internal/") {
		return errors.Reason(`-tq-sweep-task-prefix must start with "/internal/", got %q`, m.opts.SweepTaskPrefix).Err()
	}

	if m.opts.SweepTargetHost == "" {
		m.opts.SweepTargetHost = m.opts.DefaultTargetHost // may be "" on GAE
	}

	disp := m.opts.Dispatcher

	// Setup the sweep processing.
	switch m.opts.SweepMode {
	case "distributed":
		logging.Infof(ctx, "TQ sweep task queue is %q", m.opts.SweepTaskQueue)
		disp.Sweeper = NewDistributedSweeper(disp, DistributedSweeperOptions{
			SweepShards:         m.opts.SweepShards,
			TasksPerScan:        2048, // TODO: make configurable if necessary
			SecondaryScanShards: 16,   // TODO: make configurable if necessary
			LessorID:            "",   // TODO: make configurable if necessary
			TaskQueue:           m.opts.SweepTaskQueue,
			TaskPrefix:          m.opts.SweepTaskPrefix,
			TaskHost:            m.opts.SweepTargetHost,
		})
	case "inproc":
		logging.Infof(ctx, "TQ is using inproc sweeper")
		disp.Sweeper = NewInProcSweeper(InProcSweeperOptions{
			SweepShards:             m.opts.SweepShards,
			TasksPerScan:            2048, // TODO: make configurable if necessary
			SecondaryScanShards:     16,   // TODO: make configurable if necessary
			SubmitBatchSize:         128,  // TODO: make configurable if necessary
			SubmitConcurrentBatches: 32,   // TODO: make configurable if necessary
		})
	default:
		return errors.Reason(`invalid -sweep-mode %q, must be either "distributed" or "inproc"`, m.opts.SweepMode).Err()
	}

	// Setup the sweep initiation.
	if m.opts.SweepInitiationEndpoint != "-" {
		logging.Infof(ctx, "TQ sweep initiation endpoint is %q", m.opts.SweepInitiationEndpoint)
		disp.InstallSweepRoute(host.Routes(), m.opts.SweepInitiationEndpoint)
	}

	return nil
}
