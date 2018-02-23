// Copyright 2015 The LUCI Authors.
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

// Package catalog implements a part that talks to luci-config service to fetch
// and parse job definitions. Catalog knows about all task types and can
// instantiate task.Manager's.
package catalog

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/common/config/validation"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/text/pattern"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/textproto"

	"go.chromium.org/luci/scheduler/appengine/acl"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/schedule"
	"go.chromium.org/luci/scheduler/appengine/task"
)

var (
	// jobIDRe is used to validate job ID field.
	jobIDRe = regexp.MustCompile(`^[0-9A-Za-z_\-\. \)\(]{1,100}$`)

	// TODO(tandrii): deprecate these metrics once scheduler implements validation
	// endpoint which luci-config will use to pre-validate configs before giving
	// them to scheduler. See https://crbug.com/761488.

	metricConfigValid = metric.NewBool(
		"luci/scheduler/config/valid",
		"Whether project config is valid or invalid.",
		nil,
		field.String("project"),
	)

	metricConfigJobs = metric.NewInt(
		"luci/scheduler/config/jobs",
		"Number of job or trigger definitions in a project.",
		nil,
		field.String("project"),
		field.String("status"), // one of "disabled", "invalid", "valid".
	)
)

const (
	// defaultJobSchedule is default value of 'schedule' field of Job proto.
	defaultJobSchedule = "triggered"
	// defaultTriggerSchedule is default value of 'schedule' field of Trigger
	// proto.
	defaultTriggerSchedule = "with 30s interval"
)

// Catalog knows how to enumerate all scheduler configs across all projects.
// Methods return errors.Transient on non-fatal errors. Any other error means
// that retry won't help.
type Catalog interface {
	// RegisterTaskManager registers a manager that knows how to deal with
	// a particular kind of tasks (as specified by its ProtoMessageType method,
	// e.g. SwarmingTask proto).
	RegisterTaskManager(m task.Manager) error

	// GetTaskManager takes pointer to a proto message describing some task config
	// (e.g. SwarmingTask proto) and returns corresponding TaskManager
	// implementation (or nil).
	GetTaskManager(m proto.Message) task.Manager

	// UnmarshalTask takes a serialized task definition (as in Definition.Task),
	// unmarshals and validates it, and returns proto.Message that represent
	// the concrete task to run (e.g. SwarmingTask proto). It can be passed to
	// corresponding task.Manager.
	UnmarshalTask(c context.Context, task []byte) (proto.Message, error)

	// GetAllProjects returns a list of all known project ids.
	//
	// It assumes there's cfgclient implementation installed in
	// the context, will panic if it's not there.
	GetAllProjects(c context.Context) ([]string, error)

	// GetProjectJobs returns a list of scheduler jobs defined within a project or
	// empty list if no such project.
	//
	// It assumes there's cfgclient implementation installed in
	// the context, will panic if it's not there.
	GetProjectJobs(c context.Context, projectID string) ([]Definition, error)

	// ValidateConfig performs the config validation and stores any errors in
	// the validation.Context.
	//
	// This is part of the components needed to install validation endpoints
	// on the service. When a config validation request is received by the
	// service from luci-config, this is called to perform the validation.
	ValidateConfig(ctx *validation.Context, configSet, path string, content []byte)

	// ConfigPatterns returns the patterns of configSets and paths that the
	// service is responsible for validating.
	//
	// This is part of the components needed to install validation endpoints
	// on the service. When request for metadata is received by the service
	// from luci-config, this is called to return the patterns.
	ConfigPatterns(c context.Context) []*validation.ConfigPattern
}

// JobFlavor describes a category of jobs.
type JobFlavor int

const (
	// JobFlavorPeriodic is a regular job (Swarming, Buildbucket) that runs on
	// a schedule or via a trigger.
	//
	// Defined via 'job {...}' config stanza with 'schedule' field.
	JobFlavorPeriodic JobFlavor = iota

	// JobFlavorTriggered is a regular jog (Swarming, Buildbucket) that runs only
	// when triggered.
	//
	// Defined via 'job {...}' config stanza with no 'schedule' field.
	JobFlavorTriggered

	// JobFlavorTrigger is a job that can trigger other jobs (e.g. git poller).
	//
	// Defined via 'trigger {...}' config stanza.
	JobFlavorTrigger
)

// Definition wraps definition of a scheduler job fetched from the config.
type Definition struct {
	// JobID is globally unique job identifier: "<ProjectID>/<JobName>".
	JobID string

	// Acls describes who can read and who owns this job.
	Acls acl.GrantsByRole

	// Flavor describes what category of jobs this is, see the enum.
	Flavor JobFlavor

	// Revision is config revision this definition was fetched from.
	Revision string

	// RevisionURL is URL to human readable page with config file.
	RevisionURL string

	// Schedule is job's schedule in regular cron expression format.
	Schedule string

	// Task is serialized representation of scheduler job. It can be fed back to
	// Catalog.UnmarshalTask(...) to get proto.Message describing the task.
	//
	// Internally it is TaskDefWrapper proto message, but callers must treat it as
	// an opaque byte blob.
	Task []byte

	// TriggeredJobIDs is a list of jobIDs which this job triggers.
	// It's set only for triggering jobs.
	TriggeredJobIDs []string
}

// New returns implementation of Catalog.
//
// If configFileName is not "", it specifies name of *.cfg file to read job
// definitions from. If it is "", <gae-app-id>.cfg will be used instead.
func New(configFileName string) Catalog {
	return &catalog{
		managers:       map[reflect.Type]task.Manager{},
		configFileName: configFileName,
	}
}

type catalog struct {
	managers       map[reflect.Type]task.Manager
	configFileName string
}

func (cat *catalog) RegisterTaskManager(m task.Manager) error {
	prototype := m.ProtoMessageType()
	typ := reflect.TypeOf(prototype)
	if typ == nil || typ.Kind() != reflect.Ptr || typ.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("expecting pointer to a struct, got %T instead", prototype)
	}
	if _, ok := cat.managers[typ]; ok {
		return fmt.Errorf("task kind %T is already registered", prototype)
	}
	cat.managers[typ] = m
	return nil
}

func (cat *catalog) GetTaskManager(msg proto.Message) task.Manager {
	return cat.managers[reflect.TypeOf(msg)]
}

func (cat *catalog) UnmarshalTask(c context.Context, task []byte) (proto.Message, error) {
	msg := messages.TaskDefWrapper{}
	if err := proto.Unmarshal(task, &msg); err != nil {
		return nil, err
	}
	return cat.extractTaskProto(c, &msg)
}

func (cat *catalog) GetAllProjects(c context.Context) ([]string, error) {
	// Enumerate all projects that have <config>.cfg. Do not fetch actual configs
	// yet.
	var metas []*cfgclient.Meta
	if err := cfgclient.Projects(c, cfgclient.AsService, cat.configFile(c), nil, &metas); err != nil {
		return nil, err
	}

	out := make([]string, 0, len(metas))
	for _, meta := range metas {
		if projectName := meta.ConfigSet.Project(); projectName != "" {
			out = append(out, projectName)
		} else {
			logging.Warningf(c, "Unexpected ConfigSet: %s", meta.ConfigSet)
		}
	}
	return out, nil
}

func (cat *catalog) GetProjectJobs(c context.Context, projectID string) ([]Definition, error) {
	c = logging.SetField(c, "project", projectID)

	// TODO(vadimsh): This is a workaround for http://crbug.com/710619. Remove it
	// once the bug is fixed.
	projects, err := cat.GetAllProjects(c)
	if err != nil {
		return nil, err
	}
	found := false
	for _, p := range projects {
		if p == projectID {
			found = true
		}
	}
	if !found {
		return nil, nil
	}

	// TODO(tandrii): remove this after https://crbug.com/761488 is fixed.
	projectHasConfig := true
	projectIsValid := false
	defer func() {
		if projectHasConfig {
			metricConfigValid.Set(c, projectIsValid, projectID)
		}
	}()

	configSet := config.ProjectSet(projectID)
	var (
		cfg  messages.ProjectConfig
		meta cfgclient.Meta
	)
	switch err := cfgclient.Get(c, cfgclient.AsService, configSet, cat.configFile(c), textproto.Message(&cfg), &meta); err {
	case nil:
		break
	case cfgclient.ErrNoConfig:
		// Project is not using scheduler, so monitoring-wise pretend the project
		// doesn't exist.
		projectHasConfig = false
		return nil, nil
	default:
		return nil, err
	}

	revisionURL := meta.ViewURL
	if revisionURL != "" {
		logging.Infof(c, "Importing %s", revisionURL)
	}
	ctx := &validation.Context{Context: c}
	knownACLSets := acl.ValidateACLSets(ctx, cfg.GetAclSets())
	if err := ctx.Finalize(); err != nil {
		return nil, errors.Annotate(err, "invalid aclsets in a project %s", projectID).Err()
	}

	out := make([]Definition, 0, len(cfg.Job)+len(cfg.Trigger))
	disabledCount := 0

	// Regular jobs, triggered jobs.
	// TODO(tandrii): consider switching to validateProjectConfig because configs
	// provided by luci-config are known to be valid and so there is little value
	// in finding all valid jobs/triggers vs complexity of this function.
	for _, job := range cfg.Job {
		if job.Disabled {
			disabledCount++
			continue
		}
		id := "(empty)"
		if job.Id != "" {
			id = job.Id
		}
		// Create a new validation context for each job/trigger since errors
		// persist in context but we want to find all valid jobs/trigger.
		ctx = &validation.Context{Context: c}
		task := cat.validateJobProto(ctx, job)
		if err := ctx.Finalize(); err != nil {
			logging.Errorf(c, "Invalid job definition %s: %s", id, err)
			continue
		}
		packed, err := cat.marshalTask(task)
		if err != nil {
			logging.Errorf(c, "Failed to marshal the task: %s: %s", id, err)
			continue
		}
		schedule := job.Schedule
		if schedule == "" {
			schedule = defaultJobSchedule
		}
		flavor := JobFlavorTriggered
		if schedule != "triggered" {
			flavor = JobFlavorPeriodic
		}
		acls := acl.ValidateTaskACLs(ctx, knownACLSets, job.GetAclSets(), job.GetAcls())
		if err := ctx.Finalize(); err != nil {
			logging.Errorf(c, "Failed to compute task ACLs: %s: %s", id, err)
			continue
		}
		out = append(out, Definition{
			JobID:       fmt.Sprintf("%s/%s", projectID, job.Id),
			Acls:        *acls,
			Flavor:      flavor,
			Revision:    meta.Revision,
			RevisionURL: revisionURL,
			Schedule:    schedule,
			Task:        packed,
		})
	}

	// Triggering jobs.
	for _, trigger := range cfg.Trigger {
		if trigger.Disabled {
			disabledCount++
			continue
		}
		id := "(empty)"
		if trigger.Id != "" {
			id = trigger.Id
		}
		ctx = &validation.Context{Context: c}
		task := cat.validateTriggerProto(ctx, trigger)
		if err := ctx.Finalize(); err != nil {
			logging.Errorf(c, "Invalid trigger definition %s: %s", id, err)
			continue
		}
		packed, err := cat.marshalTask(task)
		if err != nil {
			logging.Errorf(c, "Failed to marshal the task: %s: %s", id, err)
			continue
		}
		schedule := trigger.Schedule
		if schedule == "" {
			schedule = defaultTriggerSchedule
		}
		acls := acl.ValidateTaskACLs(ctx, knownACLSets, trigger.GetAclSets(), trigger.GetAcls())
		if err := ctx.Finalize(); err != nil {
			logging.Errorf(c, "Failed to compute task ACLs: %s: %s", id, err)
			continue
		}
		// TODO(tandrii): validate triggered job names after collecting all valid job names.
		out = append(out, Definition{
			JobID:           fmt.Sprintf("%s/%s", projectID, trigger.Id),
			Acls:            *acls,
			Flavor:          JobFlavorTrigger,
			Revision:        meta.Revision,
			RevisionURL:     revisionURL,
			Schedule:        schedule,
			Task:            packed,
			TriggeredJobIDs: cat.normalizeTriggeredJobIDs(projectID, trigger),
		})
	}

	// Mark project as valid even if not all its jobs/triggers are.
	projectIsValid = true
	invalidCount := len(cfg.Job) + len(cfg.Trigger) - len(out) - disabledCount
	metricConfigJobs.Set(c, int64(len(out)), projectID, "valid")
	metricConfigJobs.Set(c, int64(disabledCount), projectID, "disabled")
	metricConfigJobs.Set(c, int64(invalidCount), projectID, "invalid")
	return out, nil
}

func (cat *catalog) ConfigPatterns(c context.Context) []*validation.ConfigPattern {
	return []*validation.ConfigPattern{
		{
			// Pattern automatically adds ^ and $ to regex strings.
			pattern.MustParse("regex:projects/.*"),
			pattern.MustParse(cat.configFile(c)),
		},
	}
}

func (cat *catalog) ValidateConfig(ctx *validation.Context, configSet, path string, content []byte) {
	if strings.HasPrefix(configSet, "projects/") && path == cat.configFile(ctx.Context) {
		cat.validateProjectConfig(ctx, content)
	}
}

// validateProjectConfig validates the content of a project config file.
//
// Errors are returned via validation.Context.
func (cat *catalog) validateProjectConfig(ctx *validation.Context, content []byte) {
	var cfg messages.ProjectConfig
	err := proto.UnmarshalText(string(content), &cfg)
	if err != nil {
		ctx.Error(err)
		return
	}

	// AclSets
	ctx.Enter("acl_sets")
	knownACLSets := acl.ValidateACLSets(ctx, cfg.GetAclSets())
	ctx.Exit()

	// Jobs
	ctx.Enter("job")
	for _, job := range cfg.Job {
		id := "(empty)"
		if job.Id != "" {
			id = job.Id
		}
		ctx.Enter(id)
		cat.validateJobProto(ctx, job)
		acl.ValidateTaskACLs(ctx, knownACLSets, job.GetAclSets(), job.GetAcls())
		ctx.Exit()
	}
	ctx.Exit()

	// Triggers
	ctx.Enter("trigger")
	for _, trigger := range cfg.Trigger {
		id := "(empty)"
		if trigger.Id != "" {
			id = trigger.Id
		}
		ctx.Enter(id)
		cat.validateTriggerProto(ctx, trigger)
		acl.ValidateTaskACLs(ctx, knownACLSets, trigger.GetAclSets(), trigger.GetAcls())
		ctx.Exit()
	}
	ctx.Exit()
}

// configFile returns a name of *.cfg file (inside project's config set) with
// all scheduler job definitions for a project. This file contains text-encoded
// ProjectConfig message.
func (cat *catalog) configFile(c context.Context) string {
	if cat.configFileName != "" {
		return cat.configFileName
	}
	return info.AppID(c) + ".cfg"
}

// validateJobProto validates messages.Job protobuf message.
//
// It also extracts a task definition from it (e.g. SwarmingTask proto).
// Errors are returned via validation.Context.
func (cat *catalog) validateJobProto(ctx *validation.Context, j *messages.Job) proto.Message {
	if j.Id == "" {
		ctx.Errorf("missing 'id' field'")
	} else if !jobIDRe.MatchString(j.Id) {
		ctx.Errorf("%q is not valid value for 'id' field", j.Id)
	}
	if j.Schedule != "" {
		ctx.Enter("schedule")
		if _, err := schedule.Parse(j.Schedule, 0); err != nil {
			ctx.Errorf("%s is not valid value for 'schedule' field - %s", j.Schedule, err)
		}
		ctx.Exit()
	}
	return cat.validateTaskProto(ctx, j)
}

// validateTriggerProto validates messages.Trigger protobuf message.
//
// It also extracts a task definition from it.
// Errors are returned via validation.Context.
func (cat *catalog) validateTriggerProto(ctx *validation.Context, t *messages.Trigger) proto.Message {
	if t.Id == "" {
		ctx.Errorf("missing 'id' field'")
	} else if !jobIDRe.MatchString(t.Id) {
		ctx.Errorf("%q is not valid value for 'id' field", t.Id)
	}
	if t.Schedule != "" {
		ctx.Enter("schedule")
		if _, err := schedule.Parse(t.Schedule, 0); err != nil {
			ctx.Errorf("%s is not valid value for 'schedule' field - %s", t.Schedule, err)
		}
		ctx.Exit()
	}
	return cat.validateTaskProto(ctx, t)
}

// normalizeTriggeredJobIDs returns sorted list without duplicates.
func (cat *catalog) normalizeTriggeredJobIDs(projectID string, t *messages.Trigger) []string {
	set := stringset.New(len(t.Triggers))
	for _, j := range t.Triggers {
		set.Add(projectID + "/" + j)
	}
	out := set.ToSlice()
	sort.Strings(out)
	return out
}

// validateTaskProto visits all fields of a proto and sniffs ones that correspond
// to task definitions (as registered via RegisterTaskManager). It ensures
// there's one and only one such field, validates it, and returns it.
//
// Errors are returned via validation.Context.
func (cat *catalog) validateTaskProto(ctx *validation.Context, t proto.Message) proto.Message {
	var taskMsg proto.Message

	v := reflect.ValueOf(t)
	if v.Kind() != reflect.Ptr {
		ctx.Errorf("expecting a pointer to proto message, got %T", t)
		return nil
	}
	v = v.Elem()

	for i := 0; i < v.NumField(); i++ {
		// Skip unset, scalar and repeated fields and fields that do not correspond
		// to registered task types.
		field := v.Field(i)
		if field.Kind() != reflect.Ptr || field.IsNil() || field.Elem().Kind() != reflect.Struct {
			continue
		}
		fieldVal, _ := field.Interface().(proto.Message)
		if fieldVal != nil && cat.GetTaskManager(fieldVal) != nil {
			if taskMsg != nil {
				ctx.Errorf("only one field with task definition must be set, at least two are given (%T and %T)", taskMsg, fieldVal)
				return nil
			}
			taskMsg = fieldVal
		}
	}

	if taskMsg == nil {
		ctx.Errorf("can't find a recognized task definition inside %T", t)
		return nil
	}

	taskMan := cat.GetTaskManager(taskMsg)
	ctx.Enter("task")
	taskMan.ValidateProtoMessage(ctx, taskMsg)
	ctx.Exit()
	if ctx.Finalize() != nil {
		return nil
	}
	return taskMsg
}

// extractTaskProto visits all fields of a proto and sniffs ones that correspond
// to task definitions (as registered via RegisterTaskManager). It ensures
// there's one and only one such field, validates it, and returns it.
func (cat *catalog) extractTaskProto(c context.Context, t proto.Message) (proto.Message, error) {
	ctx := &validation.Context{Context: c}
	return cat.validateTaskProto(ctx, t), ctx.Finalize()
}

// marshalTask takes a concrete task definition proto (e.g. SwarmingTask), wraps
// it into TaskDefWrapper proto and marshals this proto. The resulting blob can
// be sent to UnmarshalTask to get back the task definition proto.
func (cat *catalog) marshalTask(task proto.Message) ([]byte, error) {
	if cat.GetTaskManager(task) == nil {
		return nil, fmt.Errorf("unrecognized task definition type %T", task)
	}
	// Enumerate all fields of the wrapper until we find a matching type.
	taskType := reflect.TypeOf(task)
	wrapper := messages.TaskDefWrapper{}
	v := reflect.ValueOf(&wrapper).Elem()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if field.Type() == taskType {
			field.Set(reflect.ValueOf(task))
			return proto.Marshal(&wrapper)
		}
	}
	// This can happen only if TaskDefWrapper wasn't updated when a new task type
	// was added. This is a developer's mistake, not a config mistake.
	return nil, fmt.Errorf("could not find a field of type %T in TaskDefWrapper", task)
}
