// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package annotation

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/proto/milo"
)

// Callbacks is the set of callbacks that a State may invoke as it processes
// annotations.
type Callbacks interface {
	// StepClosed is called when a Step has closed. An Updated callback will still
	// be invoked.
	StepClosed(*Step)
	// Updated is called when a Step's state has been updated.
	Updated(*Step)
	// StepLogLine is called when a Step emits a log line.
	StepLogLine(s *Step, stream types.StreamName, label, line string)
	// StepLogEnd is called when a Step finishes emitting logs.
	StepLogEnd(*Step, types.StreamName)
}

// State is the aggregate annotation state for a given annotation
// stream. It receives updates in the form of annotations added via Append,
// and can be serialized to a Milo annotation state protobuf.
type State struct {
	// LogNameBase is the base log stream name that is prepeneded to generated
	// log streams.
	LogNameBase types.StreamName
	// Callbacks implements annotation callbacks. It may not be nil.
	Callbacks Callbacks
	// Execution is the supplied Execution. If nil, no execution details will be
	// added to the generated Milo protos.
	Execution *Execution
	// Clock is the clock implementation to use for time information.
	Clock clock.Clock

	// stepMap is a map of step name to Step instance.
	//
	// If stepMap is nil, the State is considered uninitialized.
	stepMap  map[string]*Step
	steps    []*Step
	rootStep Step
	// stepCursor is the current cursor step name. This will always point to a
	// valid Step, falling back to rootStep.
	stepCursor *Step

	closed        bool
	haltOnFailure bool
}

// initialize sets of the State's initial state. It will execute exactly once,
// and must be called by any State methods that access internal variables.
func (s *State) initialize() {
	if s.stepMap != nil {
		return
	}

	s.stepMap = map[string]*Step{}

	name := "steps"
	if s.Execution != nil {
		name = s.Execution.Name
	}
	s.rootStep.initialize(s, nil, name, 0, s.LogNameBase)
	s.registerStep(&s.rootStep)
	s.SetCurrentStep(nil)

	// Add our Command parameters, if applicable.
	if s.Execution != nil {
		name = s.Execution.Name
		var env *milo.Command_Environment
		if len(s.Execution.Env) > 0 {
			env = &milo.Command_Environment{}
			keys := make([]string, 0, len(s.Execution.Env))
			for k := range s.Execution.Env {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				entry := &milo.Command_Environment_Entry{
					Name:  k,
					Value: s.Execution.Env[k],
				}
				env.Entries = append(env.Entries, entry)
			}
		}

		s.rootStep.Command = &milo.Command{
			CommandLine: s.Execution.Command,
			Cwd:         s.Execution.Dir,
			Environ:     env,
		}
	}

	s.rootStep.Start()
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

	command, params := annotation, ""
	splitIdx := strings.IndexAny(command, "@ ")
	if splitIdx > 0 {
		command, params = command[:splitIdx], command[splitIdx+1:]
	}

	if s.closed {
		return nil
	}

	var updated *Step
	updatedIf := func(s *Step, b bool) {
		if b {
			updated = s
		}
	}

	switch command {
	// @@@BUILD_STEP <stepname>@@@
	case "BUILD_STEP":
		// Close the last section.
		step := s.CurrentStep()
		if step != nil && step != s.RootStep() {
			if step.Name() == params {
				// Same step; ignore the command.
				break
			}
			step.Close()
		}

		step = s.rootStep.AddStep(params)
		step.Start()
		s.SetCurrentStep(step)
		updatedIf(step, true)

	//  @@@SEED_STEP <stepname>@@@
	case "SEED_STEP":
		step := s.LookupStep(params)
		if step == nil {
			step = s.rootStep.AddStep(params)
			updatedIf(step, true)
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
		step.AddURLLink(parts[0], parts[1])
		updatedIf(step, true)

	//  @@@STEP_STARTED@@@
	case "STEP_STARTED":
		step := s.CurrentStep()
		updatedIf(step, step.Start())

	//  @@@STEP_WARNINGS@@@
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
		updatedIf(step, step.SetStatus(milo.Status_FAILURE))
		if s.haltOnFailure {
			updatedIf(step, s.finishWithStatus(milo.Status_FAILURE))
		}

	// @@@STEP_EXCEPTION@@@
	case "BUILD_EXCEPTION":
		fallthrough
	case "STEP_EXCEPTION":
		step := s.CurrentStep()
		updatedIf(step, step.SetStatus(milo.Status_EXCEPTION))

		// @@@STEP_CLOSED@@@
	case "STEP_CLOSED":
		step := s.CurrentStep()
		updatedIf(step, step.Close())

		// @@@STEP_LOG_LINE@<label>@<line>@@@
	case "STEP_LOG_LINE":
		step := s.CurrentStep()

		parts := strings.SplitN(params, "@", 2)
		label, line := parts[0], ""
		if len(parts) == 2 {
			line = parts[1]
		}
		updatedIf(step, step.LogLine(label, line))

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
		updatedIf(step, step.ClearText())

		// @@@STEP_SUMMARY_CLEAR@@@
	case "STEP_SUMMARY_CLEAR":
		step := s.CurrentStep()
		step.ClearSummary()
		updatedIf(step, true)

		// @@@STEP_TEXT@<msg>@@@
	case "STEP_TEXT":
		step := s.CurrentStep()
		updatedIf(step, step.AddText(params))

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
		updatedIf(step, step.AddText(parts[1]))

		// @@@STEP_SUMMARY_TEXT@<msg>@@@
	case "STEP_SUMMARY_TEXT":
		step := s.CurrentStep()
		updatedIf(step, step.SetSummary(params))

		// @@@STEP_NEST_LEVEL@<level>@@@
	case "STEP_NEST_LEVEL":
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
		updatedIf(step, step.SetProperty(parts[0], parts[1]))

		// @@@STEP_TRIGGER@<spec>@@@
	case "STEP_TRIGGER":
		// Annotee will stop short of sending an actual request to BuildBucket.
		break

	default:
		break
	}

	if updated != nil {
		s.Callbacks.Updated(updated)
	}
	return nil
}

// Finish closes the top-level annotation state and any outstanding steps.
func (s *State) Finish() {
	s.initialize()
	s.finishWithStatusImpl(nil)
}

func (s *State) finishWithStatus(st milo.Status) bool {
	return s.finishWithStatusImpl(&st)
}

func (s *State) finishWithStatusImpl(sp *milo.Status) bool {
	if s.closed {
		return false
	}

	unfinished := false
	for _, step := range s.steps[1:] {
		if u := step.closeWithStatus(nil); u {
			unfinished = true
		}
	}

	// If some steps were unfinished, show a root exception.
	if unfinished && sp == nil {
		status := milo.Status_EXCEPTION
		sp = &status
	}
	s.rootStep.closeWithStatus(sp)

	// Probe the status from our steps, if one is not supplied.
	s.closed = true
	return true
}

// LookupStep returns the step with the supplied name, or nil if no such step
// exists.
func (s *State) LookupStep(name string) *Step {
	return s.stepMap[name]
}

// LookupStepErr returns the step with the supplied name, or an error if no
// such step exists.
func (s *State) LookupStepErr(name string) (*Step, error) {
	if as := s.LookupStep(name); as != nil {
		return as, nil
	}
	return nil, fmt.Errorf("no step named %q", name)
}

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
	s.steps = append(s.steps, as)
}

func (s *State) unregisterStep(as *Step) {
	name := as.Name()
	if cas := s.stepMap[name]; cas == as {
		delete(s.stepMap, name)
	}

	if s.stepCursor == as {
		s.stepCursor = as.closestOpenParent()
	}
}

// ForEachStep iterates over all registered steps, invoking the supplied
// callback with each step's protobuf state.
func (s *State) ForEachStep(f func(*Step)) {
	for _, as := range s.steps {
		f(as)
	}
}

func (s *State) annotationNow() *google.Timestamp {
	c := s.Clock
	if c == nil {
		c = clock.GetSystemClock()
	}
	return google.NewTimestamp(c.Now())
}

// Step represents a single step.
type Step struct {
	milo.Step
	s     *State
	p     *Step
	index int

	stepIndex map[string]int
	substeps  []*Step

	// logLines is a map of log line labels to full log stream names.
	logLines map[string]types.StreamName
	// logLineCount is a map of log line label to the number of times that log
	// line has appeared. This is to prevent the case where multiple log lines
	// with the same label may be emitted, which would cause duplicate log stream
	// names.
	logLineCount map[string]int

	// LogNameBase is the LogDog stream name root for this step.
	logNameBase types.StreamName
	// hasSummary, if true, means that this Step has summary text. The summary
	// text is stored as the first line in its StepComponent.Text slice.
	hasSummary bool
	// closed is true if the element is closed.
	closed bool
}

func (as *Step) initialize(s *State, parent *Step, name string, index int, logNameBase types.StreamName) *Step {
	t := milo.Status_RUNNING
	as.StepComponent = &milo.Component{
		Name:   name,
		Status: t,
	}
	as.s = s
	as.p = parent
	as.index = index
	as.logNameBase = logNameBase
	as.stepIndex = map[string]int{}
	as.logLines = map[string]types.StreamName{}
	as.logLineCount = map[string]int{}

	return as
}

// CanonicalName returns the canonical name of this Step. This name is
// guaranteed to be unique witin the State.
func (as *Step) CanonicalName() string {
	parts := []string(nil)
	if as.index == 0 {
		parts = append(parts, as.Name())
	} else {
		parts = append(parts, fmt.Sprintf("%s_%d", as.Name(), as.index))
	}
	for p := as.p; p != nil; p = p.p {
		parts = append(parts, p.Name())
	}
	for i := len(parts)/2 - 1; i >= 0; i-- {
		opp := len(parts) - 1 - i
		parts[i], parts[opp] = parts[opp], parts[i]
	}
	return strings.Join(parts, "/")
}

// Name returns the step's component name.
func (as *Step) Name() string {
	return as.StepComponent.Name
}

// Proto returns the Milo protobuf associated with this Step.
func (as *Step) Proto() *milo.Step {
	return &as.Step
}

// BaseStream returns the supplied name prepended with this Step's base
// log name.
//
// For example, if the base name is "foo/bar", BaseStream("baz") will return
// "foo/bar/baz".
func (as *Step) BaseStream(name types.StreamName) types.StreamName {
	if as.logNameBase == "" {
		return name
	}
	return as.logNameBase.Concat(name)
}

// AnnotationStream returns the name of this Step's Milo annotation datagram
// stream.
func (as *Step) AnnotationStream() types.StreamName {
	return as.BaseStream("annotations")
}

// AddStep generates a new substep.
func (as *Step) AddStep(name string) *Step {
	// Determine/advance step index.
	index := as.stepIndex[name]
	as.stepIndex[name]++

	logPath, err := types.MakeStreamName("s_", "steps", name, strconv.Itoa(index))
	if err != nil {
		panic(fmt.Errorf("failed to generate step name for [%s]: %s", name, err))
	}

	nas := (&Step{}).initialize(as.s, as, name, index, as.BaseStream(logPath))
	as.substeps = append(as.substeps, nas)
	as.s.registerStep(nas)
	as.EnsureSubstepLogNameBase(nas.logNameBase)
	return nas
}

// Start marks the Step as started.
func (as *Step) Start() bool {
	if as.StepComponent.Started != nil {
		return false
	}
	as.StepComponent.Started = as.s.annotationNow()
	return true
}

// Close closes this step and any outstanding resources that it owns.
func (as *Step) Close() bool {
	return as.closeWithStatus(nil)
}

func (as *Step) closeWithStatus(sp *milo.Status) bool {
	if as.closed {
		return false
	}

	// Close our outstanding substeps, and get their highest status value.
	stepStatus := milo.Status_SUCCESS
	for _, sub := range as.substeps {
		sub.Close()
		if sub.StepComponent.Status > stepStatus {
			stepStatus = sub.StepComponent.Status
		}
	}

	// Close any ourstanding log streams.
	for l := range as.logLines {
		as.LogEnd(l)
	}

	// If a status is provided, use it.
	if sp != nil {
		stepStatus = *sp
	}

	if as.StepComponent.Status == milo.Status_RUNNING {
		as.StepComponent.Status = stepStatus
	}
	as.StepComponent.Ended = as.s.annotationNow()
	if as.StepComponent.Started == nil {
		as.StepComponent.Started = as.StepComponent.Ended
	}
	as.closed = true
	as.s.unregisterStep(as)
	as.s.Callbacks.Updated(as)
	as.s.Callbacks.StepClosed(as)
	return true
}

func (as *Step) closestOpenParent() *Step {
	s := as
	for {
		if s.p == nil || !s.p.closed {
			return s.p
		}
		s = s.p
	}
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
		as.AddLogdogStreamLink("", "", name)

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
	as.StepComponent.Text = append(as.StepComponent.Text, text)
	return true
}

// ClearText clears step component text.
func (as *Step) ClearText() bool {
	if len(as.StepComponent.Text) == 0 {
		return false
	}
	as.StepComponent.Text = nil
	return true
}

// SetSummary sets the Step's summary text.
//
// The summary is implemented as the first line of step component text. If no
// summary is currently defined, one will be inserted; otherwise, the current
// summary will be replaced.
func (as *Step) SetSummary(value string) bool {
	if as.hasSummary {
		if as.StepComponent.Text[0] == value {
			return false
		}

		as.StepComponent.Text[0] = value
	} else {
		as.StepComponent.Text = append(as.StepComponent.Text, "")
		copy(as.StepComponent.Text[1:], as.StepComponent.Text)
		as.StepComponent.Text[0] = value
		as.hasSummary = true
	}
	return true
}

// ClearSummary clears the step's summary text.
func (as *Step) ClearSummary() {
	if as.hasSummary {
		as.StepComponent.Text = as.StepComponent.Text[:copy(as.StepComponent.Text, as.StepComponent.Text[1:])]
		as.hasSummary = false
	}
}

// AddLogdogStreamLink adds a LogDog stream link to this Step's links list.
func (as *Step) AddLogdogStreamLink(server string, prefix, name types.StreamName) {
	link := &milo.Component_Link{
		Value: &milo.Component_Link_LogdogStream{&milo.LogdogStream{
			Name:   string(name),
			Server: server,
			Prefix: string(prefix),
		}},
	}
	as.StepComponent.OtherLinks = append(as.StepComponent.OtherLinks, link)
}

// AddURLLink adds a URL link to this Step's links list.
func (as *Step) AddURLLink(label string, url string) {
	link := &milo.Component_Link{
		Label: label,
		Value: &milo.Component_Link_Url{url},
	}
	as.StepComponent.OtherLinks = append(as.StepComponent.OtherLinks, link)
}

// SetStatus sets this step's component status.
func (as *Step) SetStatus(s milo.Status) bool {
	if as.closed || as.StepComponent.Status == s {
		return false
	}
	as.StepComponent.Status = s
	return true
}

// SetProperty sets a key/value property for this Step.
func (as *Step) SetProperty(name, value string) bool {
	for _, p := range as.StepComponent.Property {
		if p.Name == name {
			if p.Value == value {
				return false
			}
			p.Value = value
			return true
		}
	}

	as.StepComponent.Property = append(as.StepComponent.Property, &milo.Component_Property{
		Name:  name,
		Value: value,
	})
	return true
}

// EnsureSubstepLogNameBase adds the specified log name base to this Step's
// SubstepLogdogNameBase protobuf if it's not already present.
func (as *Step) EnsureSubstepLogNameBase(name types.StreamName) bool {
	nameStr := string(name)
	for _, lnb := range as.SubstepLogdogNameBase {
		if lnb == nameStr {
			return false
		}
	}
	as.SubstepLogdogNameBase = append(as.SubstepLogdogNameBase, nameStr)
	return true
}
