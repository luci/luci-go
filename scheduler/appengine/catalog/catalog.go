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
	"net/url"
	"reflect"
	"regexp"
	"strings"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"
	"github.com/luci/luci-go/luci_config/server/cfgclient"
	"github.com/luci/luci-go/luci_config/server/cfgclient/textproto"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/scheduler/appengine/acl"
	"github.com/luci/luci-go/scheduler/appengine/messages"
	"github.com/luci/luci-go/scheduler/appengine/schedule"
	"github.com/luci/luci-go/scheduler/appengine/task"
)

var (
	// jobIDRe is used to validate job ID field.
	jobIDRe = regexp.MustCompile(`^[0-9A-Za-z_\-\.]{1,100}$`)
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
	UnmarshalTask(task []byte) (proto.Message, error)

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

func (cat *catalog) UnmarshalTask(task []byte) (proto.Message, error) {
	msg := messages.TaskDefWrapper{}
	if err := proto.Unmarshal(task, &msg); err != nil {
		return nil, err
	}
	return cat.extractTaskProto(&msg)
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
		projectName, _, _ := meta.ConfigSet.SplitProject()
		if projectName == "" {
			logging.Warningf(c, "Unexpected ConfigSet: %s", meta.ConfigSet)
		} else {
			out = append(out, string(projectName))
		}
	}
	return out, nil
}

func (cat *catalog) GetProjectJobs(c context.Context, projectID string) ([]Definition, error) {
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

	configSet := cfgtypes.ProjectConfigSet(cfgtypes.ProjectName(projectID))
	configSetURL, err := cfgclient.GetConfigSetURL(c, cfgclient.AsService, configSet)
	switch err {
	case nil: // Continue
	case cfgclient.ErrNoConfig:
		return nil, nil
	default:
		return nil, err
	}

	var (
		cfg  messages.ProjectConfig
		meta cfgclient.Meta
	)
	switch err := cfgclient.Get(c, cfgclient.AsService, configSet, cat.configFile(c), textproto.Message(&cfg), &meta); err {
	case nil:
		break
	case cfgclient.ErrNoConfig:
		return nil, nil
	default:
		return nil, err
	}

	revisionURL := getRevisionURL(&configSetURL, meta.Revision, meta.Path)
	if revisionURL != "" {
		logging.Infof(c, "Importing %s", revisionURL)
	}
	// TODO(tandrii): make use of https://godoc.org/github.com/luci/luci-go/common/config/validation
	knownAclSets, err := acl.ValidateAclSets(cfg.GetAclSets())
	if err != nil {
		logging.Errorf(c, "Invalid aclsets definition %s: %s", projectID, err)
		return nil, errors.Annotate(err, "invalid aclsets in a project %s", projectID).Err()
	}

	out := make([]Definition, 0, len(cfg.Job)+len(cfg.Trigger))

	// Regular jobs, triggered jobs.
	for _, job := range cfg.Job {
		if job.Disabled {
			continue
		}
		id := "(empty)"
		if job.Id != "" {
			id = job.Id
		}
		var task proto.Message
		if task, err = cat.validateJobProto(job); err != nil {
			logging.Errorf(c, "Invalid job definition %s/%s: %s", projectID, id, err)
			continue
		}
		packed, err := cat.marshalTask(task)
		if err != nil {
			logging.Errorf(c, "Failed to marshal the task: %s/%s: %s", projectID, id, err)
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
		acls, err := acl.ValidateTaskAcls(knownAclSets, job.GetAclSets(), job.GetAcls())
		if err != nil {
			logging.Errorf(c, "Failed to compute task ACLs: %s/%s: %s", projectID, id, err)
			continue
		}
		// TODO(tandrii): remove this once this warning stops firing.
		if len(acls.Owners) == 0 || len(acls.Readers) == 0 {
			logging.Warningf(c, "Missing some ACLs on %s/%s: R=%d O=%d", projectID, id, len(acls.Owners), len(acls.Readers))
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
			continue
		}
		id := "(empty)"
		if trigger.Id != "" {
			id = trigger.Id
		}
		var task proto.Message
		if task, err = cat.validateTriggerProto(trigger); err != nil {
			logging.Errorf(c, "Invalid trigger definition %s/%s: %s", projectID, id, err)
			continue
		}
		packed, err := cat.marshalTask(task)
		if err != nil {
			logging.Errorf(c, "Failed to marshal the task: %s/%s: %s", projectID, id, err)
			continue
		}
		schedule := trigger.Schedule
		if schedule == "" {
			schedule = defaultTriggerSchedule
		}
		acls, err := acl.ValidateTaskAcls(knownAclSets, trigger.GetAclSets(), trigger.GetAcls())
		if err != nil {
			logging.Errorf(c, "Failed to compute task ACLs: %s/%s: %s", projectID, id, err)
			continue
		}
		// TODO(tandrii): remove this once this warning stops firing.
		if len(acls.Owners) == 0 || len(acls.Readers) == 0 {
			logging.Warningf(c, "Missing some ACLs on %s/%s: R=%d O=%d", projectID, id, len(acls.Owners), len(acls.Readers))
		}
		out = append(out, Definition{
			JobID:       fmt.Sprintf("%s/%s", projectID, trigger.Id),
			Acls:        *acls,
			Flavor:      JobFlavorTrigger,
			Revision:    meta.Revision,
			RevisionURL: revisionURL,
			Schedule:    schedule,
			Task:        packed,
		})
	}

	return out, nil
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

// getRevisionURL derives URL to a config file revision, given URL of a config
// set (e.g. "https://host/repo/+/ref").
func getRevisionURL(configSetURL *url.URL, rev, path string) string {
	if path == "" || configSetURL == nil {
		return ""
	}
	// repoAndRef is something like /infra/experimental/+/refs/heads/infra/config
	repoAndRef := configSetURL.Path
	plus := strings.Index(repoAndRef, "/+/")
	if plus == -1 {
		return ""
	}
	// Replace ref in path with revision, add path.
	urlCpy := *configSetURL
	urlCpy.Path = fmt.Sprintf("%s/+/%s/%s", repoAndRef[:plus], rev, path)
	return urlCpy.String()
}

// validateJobProto validates messages.Job protobuf message.
//
// It also extracts a task definition from it (e.g. SwarmingTask proto).
func (cat *catalog) validateJobProto(j *messages.Job) (proto.Message, error) {
	if j.Id == "" {
		return nil, fmt.Errorf("missing 'id' field'")
	}
	if !jobIDRe.MatchString(j.Id) {
		return nil, fmt.Errorf("%q is not valid value for 'id' field", j.Id)
	}
	if j.Schedule != "" {
		if _, err := schedule.Parse(j.Schedule, 0); err != nil {
			return nil, fmt.Errorf("%s is not valid value for 'schedule' field - %s", j.Schedule, err)
		}
	}

	// Old-style config uses embedded TaskDefWrapper field. New configs have task
	// definitions right in the Job message.
	//
	// TODO(vadimsh): Remove this branch when all configs are updated to not use
	// TaskDefWrapper anymore.
	if j.Task != nil {
		return cat.extractTaskProto(j.Task)
	}
	return cat.extractTaskProto(j)
}

// validateTriggerProto validates messages.Trigger protobuf message.
//
// It also extracts a task definition from it.
func (cat *catalog) validateTriggerProto(t *messages.Trigger) (proto.Message, error) {
	if t.Id == "" {
		return nil, fmt.Errorf("missing 'id' field'")
	}
	if !jobIDRe.MatchString(t.Id) {
		return nil, fmt.Errorf("%q is not valid value for 'id' field", t.Id)
	}
	if t.Schedule != "" {
		if _, err := schedule.Parse(t.Schedule, 0); err != nil {
			return nil, fmt.Errorf("%s is not valid value for 'schedule' field - %s", t.Schedule, err)
		}
	}
	return cat.extractTaskProto(t)
}

// extractTaskProto visits all fields of a proto and sniffs ones that correspond
// to task definitions (as registered via RegisterTaskManager). It ensures
// there's one and only one such field, validates it, and returns it.
func (cat *catalog) extractTaskProto(t proto.Message) (proto.Message, error) {
	var taskMsg proto.Message

	v := reflect.ValueOf(t)
	if v.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("expecting a pointer to proto message, got %T", t)
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
				return nil, fmt.Errorf(
					"only one field with task definition must be set, at least two are given (%T and %T)", taskMsg, fieldVal)
			}
			taskMsg = fieldVal
		}
	}

	if taskMsg == nil {
		return nil, fmt.Errorf("can't find a recognized task definition inside %T", t)
	}

	taskMan := cat.GetTaskManager(taskMsg)
	if err := taskMan.ValidateProtoMessage(taskMsg); err != nil {
		return nil, err
	}

	return taskMsg, nil
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
