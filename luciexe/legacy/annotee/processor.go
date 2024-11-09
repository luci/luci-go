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

package annotee

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/logdog/common/types"

	"go.chromium.org/luci/luciexe/legacy/annotee/annotation"
	annopb "go.chromium.org/luci/luciexe/legacy/annotee/proto"
)

const (
	// DefaultBufferSize is the Stream BufferSize value that will be used if no
	// buffer size is provided.
	DefaultBufferSize = 8192
)

const (
	// STDOUT is the system STDOUT stream name.
	STDOUT = types.StreamName("stdout")
	// STDERR is the system STDERR stream.
	STDERR = types.StreamName("stderr")

	// DefaultAnnotationSubpath is the default annotation subpath. It will be used
	// if an explicit subpath is not provided.
	DefaultAnnotationSubpath = types.StreamName("annotations")
)

// Stream describes a single process stream.
type Stream struct {
	// Reader is the stream data reader. It will be processed until it returns
	// an error or io.EOF.
	Reader io.Reader
	// Name is the logdog stream name.
	Name types.StreamName
	// Tee, if not nil, is a writer where all consumed stream data should be
	// forwarded.
	Tee io.Writer
	// Alias is the base stream name that this stream should alias to.
	Alias string

	// Annotate, if true, causes annotations in this Stream to be captured and
	// an annotation LogDog stream to be emitted.
	Annotate bool

	// StripAnnotations, if true, causes all encountered annotations to be
	// stripped from incoming stream data. Otherwise, those annotations will
	// still advnace the annotation state (if Annotate is true), but will not be
	// included in any output streams.
	StripAnnotations bool

	// EmitAllLink, if true, instructs an "all STDOUT/STDERR" link to be injected
	// into this Stream.
	EmitAllLink bool

	// BufferSize is the size of the read buffer that will be used when processing
	// this stream's data.
	BufferSize int
}

// Options are the configuration options for a Processor.
type Options struct {
	// Base is the base log stream name. This is prepended to every log name, as
	// well as any generate log names.
	Base types.StreamName
	// AnnotationSubpath is the path underneath of Base where the annotation
	// stream will be written.
	//
	// If empty, DefaultAnnotationSubpath will be used.
	AnnotationSubpath types.StreamName

	// LinkGenerator generates links to alias for a given log stream.
	//
	// If nil, no link annotations will be injected.
	LinkGenerator LinkGenerator

	// Client is the LogDog Butler Client to use for stream creation.
	Client *streamclient.Client

	// Execution describes the current applicaton's execution parameters. This
	// will be used to construct annotation state.
	Execution *annotation.Execution

	// TeeAnnotations, if true, causes all encountered annotations to be
	// tee'd, if teeing is configured.
	TeeAnnotations bool
	// TeeText, if true, causes all encountered non-annotation lines to be
	// tee'd, if teeing is configured.
	TeeText bool

	// MetadataUpdateInterval is the amount of time to wait after stream metadata
	// updates to push the updated metadata protobuf to the metadata stream.
	//
	//	- If this is < 0, metadata will only be pushed at the beginning and end of
	//	  a step.
	//	- If this equals 0, metadata will be pushed every time it's updated.
	//	- If this is 0, DefaultMetadataUpdateInterval will be used.
	MetadataUpdateInterval time.Duration

	// Offline specifies whether parsing happens not at the same time as
	// emitting. If true and CURRENT_TIMESTAMP annotations are not provided
	// then step start/end times are left empty.
	Offline bool

	// CloseSteps specified whether outstanding open steps must be closed.
	CloseSteps bool

	// AnnotationUpdated is synchronously called when the annotation message
	// changes.
	// stepBinary is binary-serialized annopb.Step.
	// stepBinary must not be mutated.
	// The call blocks writing datagrams to the output stream.
	AnnotationUpdated func(stepBinary []byte)
}

// Processor consumes data from a list of Stream entries and interacts with the
// supplied Client instance.
//
// A Processor must be instantiated with New.
type Processor struct {
	ctx context.Context
	o   *Options

	astate       *annotation.State
	stepHandlers map[*annotation.Step]*stepHandler

	annotationStream    streamclient.DatagramStream
	annotationC         chan annotationSignal
	annotationFinishedC chan struct{}

	allEmittedStreams map[*Stream]struct{}
}

type annotationSignal struct {
	data       []byte
	updateType annotation.UpdateType
}

// New instantiates a new Processor.
func New(c context.Context, o Options) *Processor {
	p := Processor{
		ctx: c,
		o:   &o,

		stepHandlers: make(map[*annotation.Step]*stepHandler),
	}
	p.astate = &annotation.State{
		LogNameBase: o.Base,
		Callbacks:   &annotationCallbacks{&p},
		Execution:   o.Execution,
		Clock:       clock.Get(c),
		Offline:     o.Offline,
	}
	return &p
}

// initialize initializes p's annotation stream handling system. If it is called
// more than once, it is a no-op.
func (p *Processor) initialize() (err error) {
	// If we're already initialized, do nothing.
	if p.annotationStream != nil {
		return nil
	}

	annotationPath := p.o.AnnotationSubpath
	if annotationPath == "" {
		annotationPath = DefaultAnnotationSubpath
	}
	annotationPath = p.astate.RootStep().BaseStream(annotationPath)

	// Create our annotation stream.
	p.annotationStream, err = p.o.Client.NewDatagramStream(
		p.ctx, annotationPath,
		streamclient.WithContentType(annopb.ContentTypeAnnotations))
	if err != nil {
		log.WithError(err).Errorf(p.ctx, "Failed to create annotation stream.")
		return
	}

	// Complete initialization and start our annotation meter.
	p.annotationC = make(chan annotationSignal)
	p.annotationFinishedC = make(chan struct{})
	p.allEmittedStreams = map[*Stream]struct{}{}

	// Run our annotation meter in a separate goroutine.
	go p.runAnnotationMeter(p.annotationStream, p.o.MetadataUpdateInterval)
	return nil
}

// RunStreams executes the Processor, consuming data from its configured streams
// and forwarding it to LogDog. Run will block until all streams have
// terminated.
//
// If a stream terminates with an error, or if there is an error processing the
// stream data, Run will return an error. If multiple Streams fail with errors,
// an errors.MultiError will be returned. io.EOF does not count as an error.
func (p *Processor) RunStreams(streams []*Stream) error {
	ingestMu := sync.Mutex{}
	ingest := func(s *Stream, l string) error {
		ingestMu.Lock()
		defer ingestMu.Unlock()

		return p.IngestLine(s, l)
	}

	// Read from all configured streams until they are finished.
	return parallel.FanOutIn(func(taskC chan<- func() error) {
		for _, s := range streams {
			s := s
			bufferSize := s.BufferSize
			if bufferSize <= 0 {
				bufferSize = DefaultBufferSize
			}
			taskC <- func() error {
				lr := newLineReader(s.Reader, bufferSize)
				for {
					line, err := lr.readLine()
					if err != nil {
						if err == io.EOF {
							return nil
						}
						return err
					}
					if err := ingest(s, line); err != nil {
						log.Fields{
							log.ErrorKey: err,
							"stream":     s.Name,
							"line":       line,
						}.Errorf(p.ctx, "Failed to ingest line.")
					}
				}
			}
		}
	})
}

// IngestLine ingests a single line of text from an input stream, responding to
// any annotations encountered.
//
// This method is not goroutine-safe.
func (p *Processor) IngestLine(s *Stream, line string) error {
	// Initialize our annotation stream.
	if err := p.initialize(); err != nil {
		log.WithError(err).Errorf(p.ctx, "Failed to initialize.")
		return err
	}

	a := extractAnnotation(line)
	if a != "" {
		log.Debugf(p.ctx, "Annotation: %q", a)
	}

	var step *annotation.Step
	if s.Annotate {
		if a != "" {
			// Append our annotation to the annotation state. This may cause our
			// annotation callbacks to be invoked.
			if err := p.astate.Append(a); err != nil {
				log.Fields{
					log.ErrorKey: err,
					"stream":     s.Name,
					"annotation": a,
				}.Errorf(p.ctx, "Failed to process annotation.")
			}
		}

		// Use the step handler for the current step.
		step = p.astate.CurrentStep()
	} else {
		// Not handling annotations. Use our root step handler.
		step = p.astate.RootStep()
	}

	h, err := p.getStepHandler(step, true)
	if err != nil {
		return err
	}

	// Determine our injected annotations.
	injectedAnnotations := h.flushInjectedAnnotations()

	// Emit the "all" link if configured (at most once).
	if lg := p.o.LinkGenerator; lg != nil && s.EmitAllLink {
		injectedAnnotations = append(injectedAnnotations,
			buildAliasAnnotation("all", "stdio", lg.GetLink("**/stdout", "**/stderr")))

		s.EmitAllLink = false
	}

	// Get our root log stream handler. As an optimization, if "step" is
	// the root step, then "h" is already the root handler, so we don't need
	// to duplicate the lookup.
	//
	// We only need the handler if we're going to emit annotations to the root
	// stream.
	var rootHandler *stepHandler
	if !s.StripAnnotations && (a != "" || len(injectedAnnotations) > 0) {
		if rootStep := p.astate.RootStep(); rootStep != step {
			rootHandler, err = p.getStepHandler(rootStep, true)
			if err != nil {
				return err
			}
		} else {
			rootHandler = h
		}
	}

	// Handle annotation line.
	if a != "" {
		// If we're teeing annotations, emit this annotation.
		//
		// Some annotations (notably, STEP_LOG_LINE) contain actual content instead
		// of structural build hinting. We don't want to tee those unless we're also
		// teeing text.
		if s.Tee != nil && p.o.TeeAnnotations && (p.o.TeeText || !isContentAnnotation(a)) {
			if err := writeTextLine(s.Tee, line); err != nil {
				log.WithError(err).Errorf(h, "Failed to tee annotation line.")
				return err
			}
		}

		// If we're not stripping annotations, emit this to the root handler.
		if !s.StripAnnotations {
			if err := rootHandler.writeBaseStream(s, line); err != nil {
				log.WithError(err).Errorf(p.ctx, "Failed to send line to LogDog.")
				return err
			}
		}
	}

	// Emit any injected annotations.
	for _, anno := range injectedAnnotations {
		// Teeing?
		if s.Tee != nil && p.o.TeeAnnotations {
			if err := writeTextLine(s.Tee, anno); err != nil {
				log.WithError(err).Errorf(h, "Failed to tee annotation line.")
				return err
			}
		}

		// Not stripping?
		if !s.StripAnnotations {
			if err := rootHandler.writeBaseStream(s, anno); err != nil {
				log.WithError(err).Errorf(h, "Failed to send injected annotation line to LogDog.")
				return err
			}
		}
	}

	// If we're stripping text, write a warning message noting that this stream
	// will not have text in it.
	if !p.o.TeeText && s.Tee != nil {
		if !h.textStrippedNote {
			err := writeTextLine(s.Tee, "This build is configured to send log data exclusively to LogDog. "+
				"Please use the LogDog link on the build page to view this log stream.")
			if err != nil {
				log.WithError(err).Errorf(h, "Failed to write text stripped notice.")
				return err
			}

			h.textStrippedNote = true
		}

		// Add links to specific log streams as they are generated.
		injectTextStreamLines := h.flushInjectedTextStreamLines()
		for _, line := range injectTextStreamLines {
			if err := writeTextLine(s.Tee, line); err != nil {
				log.WithError(err).Errorf(h, "Failed to inject text stream line: %s", line)
				return err
			}
		}
	}

	// If this is a text line, and we're teeing text, emit this line.
	if a == "" {
		if s.Tee != nil && p.o.TeeText {
			if err := writeTextLine(s.Tee, line); err != nil {
				log.WithError(err).Errorf(h, "Failed to tee text line.")
				return err
			}
		}

		// Write to our LogDog stream.
		if err := h.writeBaseStream(s, line); err != nil {
			log.WithError(err).Errorf(p.ctx, "Failed to send line to LogDog.")
			return err
		}
	}

	return err
}

// Finish instructs the Processor to close any outstanding state. This should be
// called when all automatic state updates have completed in case any steps
// didn't properly close their state.
//
// Finish will return the closed annotation state that was accumulated during
// processing.
func (p *Processor) Finish() *annotation.State {
	// Finish our step handlers.
	var closeTime *timestamppb.Timestamp
	if !p.astate.Offline {
		// Note: p.astate.Clock is never nil here, see astate setup in New().
		closeTime = timestamppb.New(p.astate.Clock.Now())
	}
	for _, h := range p.stepHandlers {
		p.finishStepHandler(h, p.o.CloseSteps, closeTime)
	}

	// If we're initialized, shut down our annotation handling.
	if p.annotationStream != nil {
		// Close and reap our annotation meter goroutine.
		close(p.annotationC)
		<-p.annotationFinishedC

		// Close and destruct our annotation stream.
		if err := p.annotationStream.Close(); err != nil {
			log.WithError(err).Errorf(p.ctx, "Failed to close annotation stream.")
		}
		p.annotationStream = nil
	}

	return p.astate
}

func (p *Processor) getStepHandler(step *annotation.Step, create bool) (*stepHandler, error) {
	if h := p.stepHandlers[step]; h != nil {
		return h, nil
	}
	if !create {
		return nil, nil
	}

	h, err := newStepHandler(p, step)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"step":       step,
		}.Errorf(p.ctx, "Failed to create step handler.")
		return nil, err
	}
	p.stepHandlers[step] = h
	return h, nil
}

func (p *Processor) finishStepHandler(h *stepHandler, closeSteps bool, closeTime *timestamppb.Timestamp) {
	// Remove this handler from our list. This will stop us from
	// double-finishing when we call Close() below, which calls the StepClosed
	// callback, which calls finishStepHandler if the step is still in the map.
	delete(p.stepHandlers, h.step)

	if h.finished {
		return
	}

	// Finish the step.
	if closeSteps {
		// Note: this is noop if the step is already closed. In particular, when
		// we end up here through StepClosed callback.
		h.step.Close(closeTime)
	}

	// Close all streams associated with this handler.
	if closeSteps {
		h.closeAllStreams()
	}

	// Notify that the annotation state has updated (closed).
	h.processor.annotationStateUpdated(annotation.UpdateStructural)
	h.finished = true
}

func (p *Processor) annotationStateUpdated(ut annotation.UpdateType) {
	// Serialize our annotation state immediately, as the Step's internal state
	// may change in future annotation processing iterations.
	data, err := proto.Marshal(p.astate.RootStep().Proto())
	if err != nil {
		log.WithError(err).Errorf(p.ctx, "Failed to marshal state.")
		return
	}

	// Send the data to our meter for transmission.
	p.annotationC <- annotationSignal{data, ut}
}

func (p *Processor) runAnnotationMeter(s streamclient.DatagramStream, interval time.Duration) {
	defer close(p.annotationFinishedC)

	var latest []byte
	sendLatest := func() {
		if latest == nil {
			return
		}

		if err := s.WriteDatagram(latest); err != nil {
			log.WithError(err).Errorf(p.ctx, "Failed to write annotation.")
		}
		if p.o.AnnotationUpdated != nil {
			p.o.AnnotationUpdated(latest)
		}
		latest = nil
	}
	// Make sure we send any buffered annotation stream before exiting.
	defer sendLatest()

	// Timer to handle metering (if enabled).
	t := clock.NewTimer(p.ctx)
	timerRunning := false
	defer t.Stop()

	first := true
	for {
		select {
		case as, ok := <-p.annotationC:
			if !ok {
				return
			}

			// Handle the new annotation data.
			latest = as.data
			switch {
			case first, as.updateType == annotation.UpdateStructural, interval == 0:
				// If
				// - This is the first, we always send the first datagram immediately.
				// - This is a structural update, we send it quickly.
				// - We're not metering, so send every annotation immediately.
				sendLatest()
				first = false

			case interval > 0 && !timerRunning:
				// Metering. Start our timer, if it's not already running from a
				// previous annotation.
				t.Reset(interval)
				timerRunning = true
			}

		case <-t.GetC():
			timerRunning = false
			sendLatest()
		}
	}
}

type annotationCallbacks struct {
	*Processor
}

func (c *annotationCallbacks) StepClosed(step *annotation.Step) {
	if h, _ := c.getStepHandler(step, false); h != nil {
		c.finishStepHandler(h, true, nil)
	}
}

func (c *annotationCallbacks) Updated(step *annotation.Step, ut annotation.UpdateType) {
	if h, _ := c.getStepHandler(step, false); h != nil {
		h.updated(ut)
	}
}

func (c *annotationCallbacks) StepLogLine(step *annotation.Step, name types.StreamName, label, line string) {
	h, err := c.getStepHandler(step, true)
	if err != nil {
		return
	}

	s, created, err := h.getTextStream(name)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"step":       h,
			"stream":     name,
		}.Errorf(c.ctx, "Failed to get log substream.")
		return
	}
	if created {
		h.maybeInjectLink(label, "logdog", name)
	}

	if err := writeTextLine(s, line); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"stream":     name,
		}.Errorf(c.ctx, "Failed to export log line.")
	}
}

func (c *annotationCallbacks) StepLogEnd(step *annotation.Step, name types.StreamName) {
	if h, _ := c.getStepHandler(step, true); h != nil {
		h.closeStream(name)
	}
}

// stepHandler handles the steps associated with a specified stream.
type stepHandler struct {
	context.Context

	processor *Processor
	step      *annotation.Step

	client                  *streamclient.Client
	injectedAnnotations     []string
	injectedTextStreamLines []string
	streams                 map[types.StreamName]io.WriteCloser
	finished                bool
	textStrippedNote        bool
}

func newStepHandler(p *Processor, step *annotation.Step) (*stepHandler, error) {
	h := stepHandler{
		Context:   log.SetField(p.ctx, "step", step),
		processor: p,
		step:      step,

		client:  p.o.Client,
		streams: make(map[types.StreamName]io.WriteCloser),
	}

	// Send our initial annotation state.
	h.updated(annotation.UpdateStructural)

	return &h, nil
}

func (h *stepHandler) String() string {
	return h.step.String()
}

func (h *stepHandler) writeBaseStream(s *Stream, line string) error {
	name := h.step.BaseStream(s.Name)
	stream, created, err := h.getTextStream(name)
	if err != nil {
		return err
	}
	if created {
		switch s.Name {
		case STDOUT:
			if h.step.SetSTDOUTStream(&annopb.LogdogStream{Name: string(name)}) {
				h.updated(annotation.UpdateIterative)
			}

		case STDERR:
			if h.step.SetSTDERRStream(&annopb.LogdogStream{Name: string(name)}) {
				h.updated(annotation.UpdateIterative)
			}
		}

		segs := s.Name.Segments()
		h.maybeInjectLink("stdio", segs[len(segs)-1], name)
	}
	return writeTextLine(stream, line)
}

func (h *stepHandler) updated(ut annotation.UpdateType) {
	if !h.finished {
		h.processor.annotationStateUpdated(ut)
	}
}

func (h *stepHandler) getTextStream(name types.StreamName) (s io.WriteCloser, created bool, err error) {
	if h.finished {
		err = fmt.Errorf("refusing to get stream %q for finished handler", name)
		return
	}
	if s = h.streams[name]; s != nil {
		return
	}

	// Create a new stream. Clone the properties archetype and customize.
	s, err = h.client.NewStream(h.processor.ctx, name)
	if err == nil {
		created = true
		h.streams[name] = s
	}
	return
}

func (h *stepHandler) closeStream(name types.StreamName) {
	s := h.streams[name]
	if s != nil {
		h.closeStreamImpl(name, s)
		delete(h.streams, name)
	}
}

func (h *stepHandler) closeAllStreams() {
	for name, s := range h.streams {
		h.closeStreamImpl(name, s)
	}
	h.streams = make(map[types.StreamName]io.WriteCloser)
}

func (h *stepHandler) closeStreamImpl(name types.StreamName, s io.WriteCloser) {
	if err := s.Close(); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"stream":     name,
		}.Errorf(h, "Failed to close step stream.")
	}
}

func (h *stepHandler) flushInjectedAnnotations() []string {
	return flushStringSlice(&h.injectedAnnotations)
}

func (h *stepHandler) flushInjectedTextStreamLines() []string {
	return flushStringSlice(&h.injectedTextStreamLines)
}

func (h *stepHandler) maybeInjectLink(base, text string, names ...types.StreamName) {
	if lg := h.processor.o.LinkGenerator; lg != nil {
		link := lg.GetLink(names...)

		h.injectedAnnotations = append(h.injectedAnnotations, buildAliasAnnotation(base, text, link))
		h.injectedTextStreamLines = append(h.injectedTextStreamLines, fmt.Sprintf("LogDog Link [%s]: %s", base, link))
	}
}

func (h *stepHandler) maybeInjectTextStreamLink(name string, stream types.StreamName) {
	if lg := h.processor.o.LinkGenerator; lg != nil {
	}
}

func buildAliasAnnotation(base, text, link string) string {
	return buildAnnotation("STEP_LINK", fmt.Sprintf("%s-->%s", text, base), link)
}

func flushStringSlice(sp *[]string) []string {
	if sp == nil {
		return nil
	}

	s := *sp
	if len(s) == 0 {
		return nil
	}

	lines := make([]string, len(s))
	copy(lines, s)
	*sp = s[:0]

	return lines
}

// lineReader reads from an input stream and returns the data line-by-line.
//
// We don't use a Scanner because we want to be able to handle lines that may
// exceed the buffer length. We don't use ReadBytes here because we need to
// capture the last line in the stream, even if it doesn't end with a newline.
type lineReader struct {
	r   *bufio.Reader
	buf bytes.Buffer
}

func newLineReader(r io.Reader, bufSize int) *lineReader {
	return &lineReader{
		r: bufio.NewReaderSize(r, bufSize),
	}
}

func (l *lineReader) readLine() (string, error) {
	l.buf.Reset()
	for {
		line, isPrefix, err := l.r.ReadLine()
		if err != nil {
			return "", err
		}
		l.buf.Write(line) // Can't (reasonably) fail.
		if !isPrefix {
			break
		}
	}
	return l.buf.String(), nil
}

func writeTextLine(w io.Writer, line string) error {
	_, err := fmt.Fprintln(w, line)
	return err
}

func extractAnnotation(line string) string {
	line = strings.TrimRightFunc(line, unicode.IsSpace)
	if len(line) <= 6 || !(strings.HasPrefix(line, "@@@") && strings.HasSuffix(line, "@@@")) {
		return ""
	}
	return strings.TrimSpace(line[3 : len(line)-3])
}

func buildAnnotation(name string, params ...string) string {
	v := make([]string, 1, 1+len(params))
	v[0] = name
	return "@@@" + strings.Join(append(v, params...), "@") + "@@@"
}

func isContentAnnotation(a string) bool {
	// Strip out any annotation arguments.
	if idx := strings.IndexRune(a, '@'); idx > 0 {
		a = a[:idx]
	}
	return a == "STEP_LOG_LINE"
}
