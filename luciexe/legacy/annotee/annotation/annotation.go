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

package annotation

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/proto/srcman"
	"go.chromium.org/luci/logdog/common/types"

	annopb "go.chromium.org/luci/luciexe/legacy/annotee/proto"
)

// UpdateType is information sent to the Updated callback to indicate the nature
// of the update.
type UpdateType int

const (
	// UpdateIterative indicates that a non-structural update occurred.
	UpdateIterative UpdateType = iota
	// UpdateStructural indicates that a structural update has occurred. A
	// structural update is one that affects the existence of or relationship of
	// the Steps in the annotation.
	UpdateStructural
)

// Callbacks is the set of callbacks that a State may invoke as it processes
// annotations.
type Callbacks interface {
	// StepClosed is called when a Step has closed. An Updated callback will still
	// be invoked.
	StepClosed(*Step)
	// Updated is called when a Step's state has been updated.
	Updated(*Step, UpdateType)
	// StepLogLine is called when a Step emits a log line.
	StepLogLine(s *Step, stream types.StreamName, label, line string)
	// StepLogEnd is called when a Step finishes emitting logs.
	StepLogEnd(*Step, types.StreamName)
}

// State is the aggregate annotation state for a given annotation
// stream. It receives updates in the form of annotations added via Append,
// and can be serialized to an annotation state protobuf.
type State struct {
	// LogNameBase is the base log stream name that is prepeneded to generated
	// log streams.
	LogNameBase types.StreamName
	// Callbacks implements annotation callbacks. It may not be nil.
	Callbacks Callbacks
	// Execution is the supplied Execution. If nil, no execution details will be
	// added to the generated annotation protos.
	Execution *Execution
	// Offline specifies whether parsing happens not at the same time as
	// emitting. If true and CURRENT_TIMESTAMP annotations are not provided
	// then step start/end times are left empty.
	Offline bool
	// Clock is the clock implementation to use for time information.
	// Defaults to system time.
	Clock clock.Clock

	// stepMap is a map of step name to Step instance.
	//
	// If stepMap is nil, the State is considered uninitialized.
	stepMap    map[string]*Step
	latestStep *Step
	rootStep   Step
	// stepCursor is the current cursor step name. This will always point to a
	// valid Step, falling back to rootStep.
	stepCursor *Step
	// startedProcessing is true iff processed at least one annotation.
	startedProcessing bool

	// stepLookup is a mapping of *annopb.Step entries to their respective *Step
	// entries.
	stepLookup map[*annopb.Step]*Step

	// currentTimestamp is time for the next annotation expected in Append.
	currentTimestamp *timestamppb.Timestamp
	closed           bool
	haltOnFailure    bool
}

// initialize sets of the State's initial state. It will execute exactly once,
// and must be called by any State methods that access internal variables.
func (s *State) initialize() {
	if s.stepMap != nil {
		return
	}

	s.stepMap = map[string]*Step{}
	s.stepLookup = map[*annopb.Step]*Step{}

	name := "steps"
	if s.Execution != nil {
		name = s.Execution.Name
	}
	s.rootStep.initializeStep(s, nil, name, false)
	s.rootStep.LogNameBase = s.LogNameBase
	s.SetCurrentStep(nil)

	// Add our Command parameters, if applicable.
	if exec := s.Execution; exec != nil {
		s.rootStep.Command = &annopb.Step_Command{
			CommandLine: exec.Command,
			Cwd:         exec.Dir,
			Environ:     exec.Env,
		}
	}

	var annotatedNow *timestamppb.Timestamp
	if !s.Offline {
		annotatedNow = s.now()
	}
	s.rootStep.Start(annotatedNow)
}

// Append adds an annotation to the state. If the state was updated, Append will
// return true.
//
// The appended annotation should only contain the annotation text body, not any
// annotation indicators (e.g., "@@@") that surround it.
//
// If the annotation is invalid or could not be added to the state, an error
// will be returned.
//
// Steps and descriptions can be found at:
// https://chromium.googlesource.com/chromium/tools/build/+/master/scripts/
// master/chromium_step.py
func (s *State) Append(annotation string) error {
	s.initialize()

	firstAnnotation := !s.startedProcessing
	s.startedProcessing = true

	command, params := annotation, ""
	splitIdx := strings.IndexAny(command, "@ ")
	if splitIdx > 0 {
		command, params = command[:splitIdx], command[splitIdx+1:]
	}

	if s.closed {
		return nil
	}

	var (
		updated    *Step
		updateType UpdateType
	)
	updatedIf := func(s *Step, u UpdateType, b bool) {
		if b {
			updated, updateType = s, u
		}
	}

	annotatedNow := s.currentTimestamp
	s.currentTimestamp = nil
	if annotatedNow == nil && !s.Offline {
		annotatedNow = s.now()
	}

	switch command {
	// @@@CURRENT_TIMESTAMP@unix_timestamp@@@
	case "CURRENT_TIMESTAMP":
		// This annotation is printed at the beginning and end of the
		// stream, as well as before each STEP_STARTED and STEP_CLOSED
		// annotations. It effectively specifies step start/end times,
		// including root step.
		timestamp, err := strconv.ParseFloat(params, 64)
		if err != nil {
			return fmt.Errorf("CURRENT_TIMESTAMP parameter %q is not a number: %s", params, err)
		}
		s.currentTimestamp = timestamppb.New(time.Unix(
			int64(timestamp),
			int64(timestamp*1000000000)%1000000000))
		if firstAnnotation {
			s.rootStep.Started = s.currentTimestamp
		}

	// @@@BUILD_STEP <stepname>@@@
	case "BUILD_STEP":
		// Close the last section.
		step := s.CurrentStep()
		if step != nil && step != s.RootStep() {
			if step.Name() == params {
				// Same step; ignore the command.
				break
			}
			if step.legacy {
				step.Close(annotatedNow)
			}
		}

		step = s.rootStep.AddStep(params, true)
		step.Start(annotatedNow)
		s.SetCurrentStep(step)
		updatedIf(step, UpdateStructural, true)

	// @@@SEED_STEP <stepname>@@@
	case "SEED_STEP":
		step := s.LookupStep(params)
		if step == nil {
			step = s.rootStep.AddStep(params, false)
			updatedIf(step, UpdateIterative, true)
		}

	// @@@STEP_CURSOR <stepname>@@@
	case "STEP_CURSOR":
		step, err := s.LookupStepErr(params)
		if err != nil {
			return fmt.Errorf("STEP_CURSOR could not lookup step: %s", err)
		}
		s.SetCurrentStep(step)

	// @@@STEP_LINK@<label>@<url>@@@
	case "link":
		fallthrough
	case "STEP_LINK":
		step := s.CurrentStep()
		parts := strings.SplitN(params, "@", 2)
		if len(parts) != 2 {
			return fmt.Errorf("STEP_LINK link [%s] missing URL", parts[0])
		}

		// If if link is an alias, parse it as one.
		alias := strings.SplitN(parts[0], "-->", 2)
		if len(alias) == 2 && len(alias[0]) > 0 && len(alias[1]) > 0 {
			// parrts[0] is an alias of the form: "text-->base"
			step.AddURLLink(alias[1], alias[0], parts[1])
		} else {
			step.AddURLLink(parts[0], "", parts[1])
		}
		updatedIf(step, UpdateIterative, true)

	// @@@STEP_STARTED@@@
	case "STEP_STARTED":
		step := s.CurrentStep()
		updatedIf(step, UpdateIterative, step.Start(annotatedNow))

	// @@@STEP_WARNINGS@@@
	case "BUILD_WARNINGS":
		fallthrough
	case "STEP_WARNINGS":
		// No warnings because they don't generally help. Builds that want to add
		// information can do so with notes. A "WARNING" state is traditionally a
		// success state with a call to attention, and that call can be done through
		// other means.
		break

	// @@@STEP_FAILURE@@@
	case "BUILD_FAILED":
		fallthrough
	case "STEP_FAILURE":
		step := s.CurrentStep()
		updatedIf(step, UpdateIterative, step.SetStatus(annopb.Status_FAILURE, nil))
		if s.haltOnFailure {
			updatedIf(step, UpdateIterative, s.finishWithStatus(annopb.Status_FAILURE, nil))
		}

	// @@@STEP_EXCEPTION@@@
	case "BUILD_EXCEPTION":
		fallthrough
	case "STEP_EXCEPTION":
		step := s.CurrentStep()
		updatedIf(step, UpdateIterative, step.SetStatus(annopb.Status_FAILURE, &annopb.FailureDetails{
			Type: annopb.FailureDetails_EXCEPTION,
		}))

	// @@@STEP_CLOSED@@@
	case "STEP_CLOSED":
		step := s.CurrentStep()
		updatedIf(step, UpdateStructural, step.Close(annotatedNow))

	// @@@STEP_LOG_LINE@<label>@<line>@@@
	case "STEP_LOG_LINE":
		step := s.CurrentStep()

		parts := strings.SplitN(params, "@", 2)
		label, line := parts[0], ""
		if len(parts) == 2 {
			line = parts[1]
		}
		updatedIf(step, UpdateIterative, step.LogLine(label, line))

	// @@@STEP_LOG_END@<label>@@@
	case "STEP_LOG_END":
		s.CurrentStep().LogEnd(params)

	// @@@STEP_LOG_END_PERF@<label>@@@
	case "STEP_LOG_END_PERF":
		// Ignore for now. Ideally would emit a link to the perf dashboard.
		break

	// @@@STEP_CLEAR@@@
	case "STEP_CLEAR":
		step := s.CurrentStep()
		updatedIf(step, UpdateIterative, step.ClearText())

	// @@@STEP_SUMMARY_CLEAR@@@
	case "STEP_SUMMARY_CLEAR":
		step := s.CurrentStep()
		step.ClearSummary()
		updatedIf(step, UpdateIterative, true)

	// @@@STEP_TEXT@<msg>@@@
	case "STEP_TEXT":
		step := s.CurrentStep()
		updatedIf(step, UpdateIterative, step.AddText(params))

	// @@@SEED_STEP_TEXT@step@<msg>@@@
	case "SEED_STEP_TEXT":
		parts := strings.SplitN(params, "@", 2)
		if len(parts) < 2 {
			return nil
		}
		step, err := s.LookupStepErr(parts[0])
		if err != nil {
			return err
		}
		updatedIf(step, UpdateIterative, step.AddText(parts[1]))

	// @@@STEP_SUMMARY_TEXT@<msg>@@@
	case "STEP_SUMMARY_TEXT":
		step := s.CurrentStep()
		updatedIf(step, UpdateIterative, step.SetSummary(params))

	// @@@STEP_NEST_LEVEL@<level>@@@
	case "STEP_NEST_LEVEL":
		level, err := strconv.Atoi(params)
		if err != nil {
			return fmt.Errorf("could not parse nest level from %q: %v", params, err)
		}
		if level < 0 {
			return fmt.Errorf("level must be >= 0, not %d", level)
		}

		step := s.CurrentStep()
		updatedIf(step, UpdateStructural, step.SetNestLevel(level))
		break

	// @@@HALT_ON_FAILURE@@@
	case "HALT_ON_FAILURE":
		s.haltOnFailure = true

	// @@@HONOR_ZERO_RETURN_CODE@@@
	case "HONOR_ZERO_RETURN_CODE":
		// We don't capture the step return code, so not much we can do here.
		break

	// @@@SET_BUILD_PROPERTY@<name>@<json>@@@
	case "SET_BUILD_PROPERTY":
		step := s.CurrentStep()
		parts := strings.SplitN(params, "@", 2)
		if len(parts) == 1 {
			parts = append(parts, "")
		}
		updatedIf(step, UpdateIterative, step.SetProperty(parts[0], parts[1]))

	// @@@STEP_TRIGGER@<spec>@@@
	case "STEP_TRIGGER":
		// Annotee will stop short of sending an actual request to BuildBucket.
		break

	// This is ONLY supported by annotee, not by buildbot.
	// @@@SOURCE_MANIFEST@<name>@<sha256>@<url>@@@
	case "SOURCE_MANIFEST":
		parts := strings.SplitN(params, "@", 3)
		if len(parts) != 3 {
			return fmt.Errorf("SOURCE_MANIFEST expected 3 params, got %q", params)
		}

		step := s.RootStep()
		if step.SourceManifests == nil {
			step.SourceManifests = map[string]*srcman.ManifestLink{}
		}

		name, hashHex, url := parts[0], parts[1], parts[2]
		hash, err := hex.DecodeString(hashHex)
		if err != nil {
			return fmt.Errorf("SOURCE_MANIFEST has bad hash: %s", err)
		}
		if _, ok := step.SourceManifests[name]; ok {
			return fmt.Errorf("repeated SOURCE_MANIFEST name %q", name)
		}

		step.SourceManifests[name] = &srcman.ManifestLink{
			Sha256: hash,
			Url:    url,
		}
		updated = step
	}

	if updated != nil {
		s.Callbacks.Updated(updated, updateType)
	}
	return nil
}

// Finish closes the top-level annotation state and any outstanding steps.
func (s *State) Finish() {
	s.initialize()
	s.finishAndDeriveStatus()
}

func (s *State) finishAndDeriveStatus() bool {
	return s.finishWithStatusImpl(nil, nil)
}

func (s *State) finishWithStatus(st annopb.Status, fd *annopb.FailureDetails) bool {
	return s.finishWithStatusImpl(&st, fd)
}

func (s *State) finishWithStatusImpl(status *annopb.Status, fd *annopb.FailureDetails) bool {
	if s.closed {
		return false
	}

	// if s.currentTimestamp is not nil, the last annotation was
	// CURRENT_TIMESTAMP and s.currentTimestamp contains its value.
	buildEndTime := s.currentTimestamp
	s.currentTimestamp = nil
	if buildEndTime == nil && !s.Offline {
		buildEndTime = s.now()
	}

	// Traverse through every step *except* our root step.
	unfinished := false
	for step := s.rootStep.nextStep; step != nil; step = step.nextStep {
		if u := step.closeWithStatus(buildEndTime, nil); u {
			unfinished = true
		}
	}

	// If some steps were unfinished, show a root exception.
	if unfinished && status == nil {
		exception := annopb.Status_FAILURE
		status = &exception
		if fd == nil {
			fd = &annopb.FailureDetails{
				Type: annopb.FailureDetails_EXCEPTION,
			}
		}
	}
	s.rootStep.FailureDetails = fd
	s.rootStep.closeWithStatus(buildEndTime, status)

	// Probe the status from our steps, if one is not supplied.
	s.closed = true
	return true
}

// LookupStep returns the step with the supplied name, or nil if no such step
// exists.
//
// If multiple steps share a name, this will return the latest registered step
// with that name.
func (s *State) LookupStep(name string) *Step { return s.stepMap[name] }

// LookupStepErr returns the step with the supplied name, or an error if no
// such step exists.
//
// If multiple steps share a name, this will return the latest registered step
// with that name.
func (s *State) LookupStepErr(name string) (*Step, error) {
	if as := s.LookupStep(name); as != nil {
		return as, nil
	}
	return nil, fmt.Errorf("no step named %q", name)
}

// ResolveStep returns the annotation package *Step corresponding to the
// supplied *annopb.Step. This is a reverse lookup operation.
//
// If the supplied *annopb.Step is not registered with this annotation State,
// this function will return nil.
func (s *State) ResolveStep(ms *annopb.Step) *Step { return s.stepLookup[ms] }

// RootStep returns the root step.
func (s *State) RootStep() *Step {
	s.initialize()

	return &s.rootStep
}

// CurrentStep returns the step referenced by the step cursor.
func (s *State) CurrentStep() *Step {
	s.initialize()

	return s.stepCursor
}

// SetCurrentStep sets the current step. If the supplied step is nil, the root
// step will be used.
//
// The supplied step must already be registered with the State.
func (s *State) SetCurrentStep(v *Step) {
	if v == nil {
		v = &s.rootStep
	}
	if v.s != s {
		panic("step is not bound to state")
	}
	s.stepCursor = v
}

func (s *State) registerStep(as *Step) {
	s.stepMap[as.Name()] = as
	s.stepLookup[&as.Step] = as

	if latest := s.latestStep; latest != nil {
		latest.nextStep = as
		as.prevStep = latest
	}
	s.latestStep = as
}

func (s *State) unregisterStep(as *Step) {
	name := as.Name()
	if cas := s.stepMap[name]; cas == as {
		delete(s.stepMap, name)
	}

	if s.stepCursor == as {
		s.stepCursor = as.closestOpenStep()
	}
}

// now returns current time of s.Clock. Defaults to system clock.
func (s *State) now() *timestamppb.Timestamp {
	c := s.Clock
	if c == nil {
		c = clock.GetSystemClock()
	}
	return timestamppb.New(c.Now())
}

// Step represents a single step.
type Step struct {
	annopb.Step
	s *State

	// parent is the step that spawned this step.
	parent *Step

	// prevStep is the step that was created immediately before this step. It is
	// nil if this is the root step.
	//
	// Both prevStep and nextStep are creation-ordered, and don't change even if
	// a Step is reparented.
	prevStep *Step
	// nextStep is the step that was created immediately after this step. It is
	// nil if this is the latest step.
	//
	// Both prevStep and nextStep are creation-ordered, and don't change even if
	// a Step is reparented.
	nextStep *Step

	level int

	// legacy is explicit support for the legacy "BUILD_STEP" annotation. Any Step
	// that is created via BUILD_STEP is considered a legacy step. Only legacy
	// steps get automatically closed when a new "BUILD_STEP" annotation is
	// encountered.
	legacy bool

	// logPathIndex is a map of the number of log paths with the given base name.
	// Each time a log path is generated, it will register with this map and
	// increase the count.
	logPathIndex map[types.StreamName]int

	// logLines is a map of log line labels to full log stream names.
	logLines map[string]types.StreamName
	// logLineCount is a map of log line label to the number of times that log
	// line has appeared. This is to prevent the case where multiple log lines
	// with the same label may be emitted, which would cause duplicate log stream
	// names.
	logLineCount map[string]int

	// linkMap is a map of link label to link struct. BuildBot only retains the
	// latest link for a given label, so we use this to enforce that.
	linkMap map[string]*annopb.AnnotationLink

	// logNameBase is the LogDog stream name root for this step.
	LogNameBase types.StreamName
	// hasSummary, if true, means that this Step has summary text. The summary
	// text is stored as the first line in its Step.Text slice.
	hasSummary bool
	// closed is true if the element is closed.
	closed bool
}

func (as *Step) String() string { return string(as.LogNameBase) }

func (as *Step) initializeStep(s *State, parent *Step, name string, legacy bool) *Step {
	t := annopb.Status_RUNNING
	as.Step = annopb.Step{
		Name:   name,
		Status: t,
	}

	as.s = s
	as.legacy = legacy
	as.logLines = map[string]types.StreamName{}
	as.logLineCount = map[string]int{}
	as.logPathIndex = map[types.StreamName]int{}

	// Add this Step to our parent's Substep list.
	if parent != nil {
		parent.appendSubstep(as)
	}
	s.registerStep(as)

	return as
}

func (as *Step) appendSubstep(s *Step) {
	if s.parent == as {
		// Already parented to as, so do nothing.
		return
	}
	s.detachFromParent()

	s.parent = as
	as.Substep = append(as.Substep, &annopb.Step_Substep{
		Substep: &annopb.Step_Substep_Step{
			Step: &s.Step,
		},
	})
	s.regenerateLogPath()
}

func (as *Step) detachFromParent() {
	parent := as.parent
	if parent == nil {
		return
	}

	// Remove any instances of "as" from its current parent's Substeps.
	ssPtr := 0
	for _, ss := range parent.Substep {
		if ss.GetStep() != &as.Step {
			parent.Substep[ssPtr] = ss
			ssPtr++
		}
	}
	parent.Substep = parent.Substep[:ssPtr]
	as.parent = nil
}

// Name returns the step's component name.
func (as *Step) Name() string {
	return as.Step.Name
}

// Proto returns the annotation Step protobuf associated with this Step.
func (as *Step) Proto() *annopb.Step {
	return &as.Step
}

// BaseStream returns the supplied name prepended with this Step's base
// log name.
//
// For example, if the base name is "foo/bar", BaseStream("baz") will return
// "foo/bar/baz".
func (as *Step) BaseStream(name types.StreamName) types.StreamName {
	if as.LogNameBase == "" {
		return name
	}
	return as.LogNameBase.Concat(name)
}

// AddStep generates a new substep.
func (as *Step) AddStep(name string, legacy bool) *Step {
	return (&Step{}).initializeStep(as.s, as, name, legacy)
}

func (as *Step) regenerateLogPath() {
	if as.parent == nil {
		panic("log path regeneration cannot be called on root step")
	}

	// Recipe engine nests steps by prepending their parents' name, e.g.
	// if "foo" has a nested child, it will be named "foo.bar". This is redundant
	// for our stream names, so strip that off.
	//
	// We throw the length conditional in just in case the child step happens to
	// have the exact same name as the parent. This shouldn't happen naturally,
	// but let's be robust.
	name := as.Name()
	if parentPrefix := (as.parent.Name() + "."); len(parentPrefix) < len(name) {
		name = strings.TrimPrefix(name, parentPrefix)
	}

	logPath, err := types.MakeStreamName("s_", "steps", name)
	if err != nil {
		panic(fmt.Errorf("failed to generate step name for [%s]: %s", as.Name(), err))
	}

	index := as.parent.logPathIndex[logPath]
	as.parent.logPathIndex[logPath] = (index + 1)

	// Append the index to the stream name.
	logPath = logPath.Concat(types.StreamName(strconv.Itoa(index)))
	if err := logPath.Validate(); err != nil {
		panic(fmt.Errorf("generated invalid log stream path %q: %v", logPath, err))
	}

	as.LogNameBase = as.parent.BaseStream(logPath)
}

// Start marks the Step as started.
func (as *Step) Start(startTime *timestamppb.Timestamp) bool {
	if as.Started != nil {
		return false
	}
	as.Started = startTime
	return true
}

// Close closes this step and any outstanding resources that it owns.
// If it is already closed, does not have side effects and returns false.
func (as *Step) Close(closeTime *timestamppb.Timestamp) bool {
	return as.closeWithStatus(closeTime, nil)
}

func (as *Step) closeWithStatus(closeTime *timestamppb.Timestamp, sp *annopb.Status) bool {
	if as.closed {
		return false
	}

	// Close our outstanding substeps, and get their highest status value.
	stepStatus := annopb.Status_SUCCESS
	if sp == nil {
		for _, ss := range as.Substep {
			sub := as.s.ResolveStep(ss.GetStep())
			if sub == nil {
				continue
			}

			sub.Close(closeTime)
			if sub.Status > stepStatus {
				stepStatus = sub.Status
			}
		}
	} else {
		// If a status is provided, use it.
		stepStatus = *sp
	}

	// Close any outstanding log streams.
	for l := range as.logLines {
		as.LogEnd(l)
	}

	if as.Status == annopb.Status_RUNNING {
		as.Status = stepStatus
	}
	as.Ended = closeTime
	if as.Started == nil {
		as.Started = as.Ended
	}

	as.closed = true
	as.s.unregisterStep(as)
	as.s.Callbacks.Updated(as, UpdateStructural)
	as.s.Callbacks.StepClosed(as)
	return true
}

func (as *Step) closestOpenStep() *Step {
	for ps := as.prevStep; ps != nil; ps = ps.prevStep {
		if !ps.closed {
			return ps
		}
	}
	return &as.s.rootStep
}

// LogLine emits a log line for a specified log label.
func (as *Step) LogLine(label, line string) bool {
	updated := false

	name, ok := as.logLines[label]
	if !ok {
		// No entry for this log line. Create a new one and register it.
		//
		// This will appear as:
		// [BASE]/logs/[label]/[ord]
		subName, err := types.MakeStreamName("s_", "logs", label, strconv.Itoa(as.logLineCount[label]))
		if err != nil {
			panic(fmt.Errorf("failed to generate log stream name for [%s]: %s", label, err))
		}
		name = as.BaseStream(subName)
		as.AddLogdogStreamLink("", label, "", name)

		as.logLines[label] = name
		as.logLineCount[label]++
		updated = true
	}

	as.s.Callbacks.StepLogLine(as, name, label, line)
	return updated
}

// LogEnd ends the log for the specified label.
func (as *Step) LogEnd(label string) {
	name, ok := as.logLines[label]
	if !ok {
		return
	}

	delete(as.logLines, label)
	as.s.Callbacks.StepLogEnd(as, name)
}

// AddText adds a line of step component text.
func (as *Step) AddText(text string) bool {
	as.Text = append(as.Text, text)
	return true
}

// ClearText clears step component text.
func (as *Step) ClearText() bool {
	if len(as.Text) == 0 {
		return false
	}
	as.Text = nil
	return true
}

// SetSummary sets the Step's summary text.
//
// The summary is implemented as the first line of step component text. If no
// summary is currently defined, one will be inserted; otherwise, the current
// summary will be replaced.
func (as *Step) SetSummary(value string) bool {
	if as.hasSummary {
		if as.Text[0] == value {
			return false
		}

		as.Text[0] = value
	} else {
		as.Text = append(as.Text, "")
		copy(as.Text[1:], as.Text)
		as.Text[0] = value
		as.hasSummary = true
	}
	return true
}

// ClearSummary clears the step's summary text.
func (as *Step) ClearSummary() {
	if as.hasSummary {
		as.Text = as.Text[:copy(as.Text, as.Text[1:])]
		as.hasSummary = false
	}
}

// SetNestLevel sets the nest level of this Step, and identifies its nesting
// parent.
//
// If no parent could be found at level "l-1", the root step will become the
// parent.
func (as *Step) SetNestLevel(l int) bool {
	if as.level == l {
		return false
	}
	as.level = l

	// Attach this step to the correct parent step based on nest level. Ascend
	// up the previously-declared steps.
	var nestParent *Step
	for prev := as.prevStep; prev != nil; prev = prev.prevStep {
		if prev.level < l {
			nestParent = prev
			break
		}
	}
	if nestParent == nil || nestParent == as.parent {
		return true
	}
	nestParent.appendSubstep(as)
	return true
}

// AddLogdogStreamLink adds a LogDog stream link to this Step's links list.
func (as *Step) AddLogdogStreamLink(server, label string, prefix, name types.StreamName) {
	link := as.getOrCreateLinkForLabel(label)
	link.Value = &annopb.AnnotationLink_LogdogStream{LogdogStream: &annopb.LogdogStream{
		Name:   string(name),
		Server: server,
		Prefix: string(prefix),
	}}
}

// AddURLLink adds a URL link to this Step's links list.
func (as *Step) AddURLLink(label, alias, url string) {
	link := as.getOrCreateLinkForLabel(label)
	link.AliasLabel = alias
	link.Value = &annopb.AnnotationLink_Url{Url: url}
}

func (as *Step) getOrCreateLinkForLabel(label string) *annopb.AnnotationLink {
	if cur := as.linkMap[label]; cur != nil {
		return cur
	}

	// New label, so create a new link.
	link := &annopb.AnnotationLink{
		Label: label,
	}
	if as.linkMap == nil {
		as.linkMap = make(map[string]*annopb.AnnotationLink)
	}
	as.OtherLinks = append(as.OtherLinks, link)
	as.linkMap[label] = link
	return link
}

// SetStatus sets this step's component status.
//
// If the status doesn't change, the supplied failure details will be ignored.
func (as *Step) SetStatus(s annopb.Status, fd *annopb.FailureDetails) bool {
	if as.closed || as.Status == s {
		return false
	}
	as.Status = s
	as.FailureDetails = fd
	return true
}

// SetProperty sets a key/value property for this Step.
func (as *Step) SetProperty(name, value string) bool {
	for _, p := range as.Property {
		if p.Name == name {
			if p.Value == value {
				return false
			}
			p.Value = value
			return true
		}
	}

	as.Property = append(as.Property, &annopb.Step_Property{
		Name:  name,
		Value: value,
	})
	return true
}

// SetSTDOUTStream sets the LogDog STDOUT stream value, returning true if the
// Step was updated.
func (as *Step) SetSTDOUTStream(st *annopb.LogdogStream) (updated bool) {
	as.StdoutStream, updated = as.maybeSetLogDogStream(as.StdoutStream, st)
	return
}

// SetSTDERRStream sets the LogDog STDERR stream value, returning true if the
// Step was updated.
func (as *Step) SetSTDERRStream(st *annopb.LogdogStream) (updated bool) {
	as.StderrStream, updated = as.maybeSetLogDogStream(as.StderrStream, st)
	return
}

func (as *Step) maybeSetLogDogStream(target *annopb.LogdogStream, st *annopb.LogdogStream) (*annopb.LogdogStream, bool) {
	if (target == nil && st == nil) || (target != nil && st != nil && proto.Equal(target, st)) {
		return target, false
	}
	return st, true
}
