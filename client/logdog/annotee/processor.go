// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package annotee

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/client/logdog/annotee/annotation"
	"github.com/luci/luci-go/client/logdog/butlerlib/streamclient"
	"github.com/luci/luci-go/client/logdog/butlerlib/streamproto"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/clockflag"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/parallel"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"golang.org/x/net/context"
)

const (
	// DefaultBufferSize is the Stream BufferSize value that will be used if no
	// buffer size is provided.
	DefaultBufferSize = 8192
)

var (
	textStreamArchetype = streamproto.Flags{
		ContentType: string(types.ContentTypeText),
		Type:        streamproto.StreamType(logpb.StreamType_TEXT),
	}

	metadataStreamArchetype = streamproto.Flags{
		ContentType: string(types.ContentTypeAnnotations),
		Type:        streamproto.StreamType(logpb.StreamType_DATAGRAM),
	}
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

	// Annotate, if true, causes annotations in this Stream to be captured and
	// an annotation LogDog stream to be emitted.
	Annotate bool
	// StripAnnotations, if true, causes all encountered annotations to be
	// stripped from incoming stream data.
	StripAnnotations bool

	// BufferSize is the size of the read buffer that will be used when processing
	// this stream's data.
	BufferSize int
}

// Processor consumes data from a list of Stream entries and
type Processor struct {
	// Base is the base log stream name. This is prepended to every log name, as
	// well as any generate log names.
	Base types.StreamName
	// Context is the Context instance to use.
	Context context.Context
	// Client is the LogDog Butler Client to use for stream creation.
	Client streamclient.Client
	// Execution describes the current applicaton's execution parameters. This
	// will be used to construct annotation state.
	Execution *annotation.Execution
	// MetadataUpdateInterval is the amount of time to wait after stream metadata
	// updates to push the updated metadata protobuf to the metadata stream.
	//
	//	- If this is < 0, metadata will only be pushed at the beginning and end of
	//	  a step.
	//	- If this equals 0, metadata will be pushed every time it's updated.
	//	- If this is 0, DefaultMetadataUpdateInterval will be used.
	MetadataUpdateInterval time.Duration

	astate       *annotation.State
	stepHandlers map[string]*stepHandler
}

// RunStreams executes the Processor, consuming data from its configured streams
// and forwarding it to LogDog. Run will block until all streams have
// terminated.
//
// If a stream terminates with an error, or if there is an error processing the
// stream data, Run will return an error. If multiple Streams fail with errors,
// an errors.MultiError will be returned.
func (p *Processor) RunStreams(streams []*Stream) error {
	// Close and cleanup resources after streams are complete.
	p.Reset()
	defer p.Finish()

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
						}.Errorf(p.Context, "Failed to ingest line.")
					}
				}
			}
		}
	})
}

// Reset clears the current state of the Processor.
func (p *Processor) Reset() {
	p.stepHandlers = make(map[string]*stepHandler)
	p.astate = &annotation.State{
		LogNameBase: p.Base,
		Callbacks:   &annotationCallbacks{p},
		Execution:   p.Execution,
		Clock:       clock.Get(p.Context),
	}
}

// IngestLine ingests a single line of text from an input stream, responding to
// any annotations encountered.
func (p *Processor) IngestLine(s *Stream, line string) error {
	if p.astate == nil {
		p.Reset()
	}

	a := extractAnnotation(line)
	if a != "" {
		log.Debugf(p.Context, "Annotation: %q", a)
	}

	var step *annotation.Step
	var h *stepHandler
	if s.Annotate {
		if a != "" {
			// Append our annotation to the annotation state. This may cause our
			// annotation callbacks to be invoked.
			if err := p.astate.Append(a); err != nil {
				log.Fields{
					log.ErrorKey: err,
					"stream":     s.Name,
					"annotation": a,
				}.Errorf(p.Context, "Failed to process annotation.")
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

	// If configured, tee to our tee stream.
	if s.Tee != nil && (a == "" || !s.StripAnnotations) {
		// Tee this to the Stream's configured Tee output.
		if err := writeTextLine(s.Tee, line); err != nil {
			log.WithError(err).Errorf(p.Context, "Failed to tee line.")
			return err
		}
	}

	// Write to our LogDog stream.
	if err := h.writeBaseStream(s, line); err != nil {
		log.WithError(err).Errorf(p.Context, "Failed to send line to LogDog.")
		return err
	}

	return err
}

// State returns the current annotation state.
func (p *Processor) State() *annotation.State {
	return p.astate
}

func (p *Processor) getStepHandler(step *annotation.Step, create bool) (*stepHandler, error) {
	name := step.CanonicalName()
	if h := p.stepHandlers[name]; h != nil {
		return h, nil
	}
	if !create {
		return nil, nil
	}

	h, err := newStepHandler(p, step)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"step":       name,
		}.Errorf(p.Context, "Failed to create step handler.")
		return nil, err
	}
	p.stepHandlers[name] = h
	return h, nil
}

func (p *Processor) closeStep(step *annotation.Step) {
	if h, _ := p.getStepHandler(step, false); h != nil {
		p.closeStepHandler(h)
	}
}

func (p *Processor) closeStepHandler(h *stepHandler) {
	// Remove this handler from our list. This will stop us from
	// double-finishing when finish() calls Close(), which calls the StepClosed
	// callback.
	delete(p.stepHandlers, h.String())

	// Finish the step.
	h.finish()
}

// Finish instructs the Processor to close any outstanding state. This should be
// called when all automatic state updates have completed in case any steps
// didn't properly close their state.
func (p *Processor) Finish() {
	// Close our step handlers.
	for _, h := range p.stepHandlers {
		p.closeStepHandler(h)
	}
}

type annotationCallbacks struct {
	*Processor
}

func (c *annotationCallbacks) StepClosed(step *annotation.Step) {
	c.closeStep(step)
}

func (c *annotationCallbacks) Updated(step *annotation.Step) {
	if h, _ := c.getStepHandler(step, false); h != nil {
		h.updated()
	}
}

func (c *annotationCallbacks) StepLogLine(step *annotation.Step, name types.StreamName, line string) {
	h, err := c.getStepHandler(step, true)
	if err != nil {
		return
	}

	s, err := h.getStream(name, &textStreamArchetype)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"step":       h,
			"stream":     name,
		}.Errorf(c.Context, "Failed to get log substream.")
		return
	}

	if err := writeTextLine(s, line); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"stream":     name,
		}.Errorf(c.Context, "Failed to export log line.")
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

	step *annotation.Step

	client              streamclient.Client
	streams             map[types.StreamName]streamclient.Stream
	annotationC         chan []byte
	annotationFinishedC chan struct{}
	closed              bool
}

func newStepHandler(p *Processor, step *annotation.Step) (*stepHandler, error) {
	h := stepHandler{
		Context: log.SetField(p.Context, "step", step.CanonicalName()),
		step:    step,

		client:              p.Client,
		streams:             make(map[types.StreamName]streamclient.Stream),
		annotationC:         make(chan []byte),
		annotationFinishedC: make(chan struct{}),
	}

	// Create our annotation stream immediately. We do this here because we want
	// to fail immediately if we can't create this stream.
	//
	// This stream will be used exclusively by our meter goroutine.
	astr, err := h.getStream(step.AnnotationStream(), &metadataStreamArchetype)
	if err != nil {
		log.WithError(err).Errorf(h, "Failed to create annotation stream.")
		return nil, err
	}

	// Run our annotation meter in a separate goroutine.
	go h.runAnnotationMeter(astr, p.MetadataUpdateInterval)

	// Send our initial annotation state.
	h.updated()

	return &h, nil
}

func (h *stepHandler) String() string {
	return h.step.CanonicalName()
}

func (h *stepHandler) runAnnotationMeter(s streamclient.Stream, interval time.Duration) {
	defer close(h.annotationFinishedC)

	var latest []byte
	sendLatest := func() {
		if latest == nil {
			return
		}

		if err := s.WriteDatagram(latest); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"step":       h.String(),
			}.Errorf(h, "Failed to write annotation.")
		}
		latest = nil
	}
	// Make sure we send any buffered annotation stream before exiting.
	defer sendLatest()

	// Timer to handle metering (if enabled).
	t := clock.NewTimer(h)
	timerRunning := false
	defer t.Stop()

	first := true
	for {
		select {
		case d, ok := <-h.annotationC:
			if !ok {
				return
			}

			// Handle the new annotation data.
			latest = d
			switch {
			case first:
				// Always send the first datagram immediately.
				sendLatest()
				first = false

			case interval == 0:
				// Not metering, send every annotation immediately.
				sendLatest()

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

func (h *stepHandler) finish() {
	if h.closed {
		return
	}

	// Close the handler. This may send one last annotation to summarize the
	// state if closing changed it.
	if h.step.Close() {
		// Manually mark it updated, since Close callbacks will have unregistered
		// us from the standard Updated() reporting loop.
		h.updated()
	}

	// Close and reap our meter goroutine.
	close(h.annotationC)
	<-h.annotationFinishedC

	// Close all streams associated with this handler.
	h.closeAllStreams()

	h.closed = true
}

func (h *stepHandler) writeBaseStream(s *Stream, line string) error {
	stream, err := h.getStream(h.step.BaseStream(s.Name), &textStreamArchetype)
	if err != nil {
		return err
	}
	return writeTextLine(stream, line)
}

func (h *stepHandler) updated() {
	// Ignore updates after the step has closed.
	if h.closed {
		return
	}

	// Serialize immediately, as the Step's internal state may change in future
	// annotation runs.
	p := h.step.Proto()

	data, err := proto.Marshal(p)
	if err != nil {
		log.WithError(err).Errorf(h, "Failed to marshal state.")
		return
	}
	h.annotationC <- data
}

func (h *stepHandler) getStream(name types.StreamName, flags *streamproto.Flags) (streamclient.Stream, error) {
	if h.closed {
		return nil, fmt.Errorf("refusing to get stream %q for closed handler", name)
	}
	if s := h.streams[name]; s != nil {
		return s, nil
	}
	if flags == nil {
		return nil, nil
	}

	// Create a new stream. Clone the properties archetype and customize.
	f := *flags
	f.Timestamp = clockflag.Time(clock.Now(h))
	f.Name = streamproto.StreamNameFlag(name)
	s, err := h.client.NewStream(f)
	if err != nil {
		return nil, err
	}
	h.streams[name] = s
	return s, nil
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
	h.streams = make(map[types.StreamName]streamclient.Stream)
}

func (h *stepHandler) closeStreamImpl(name types.StreamName, s streamclient.Stream) {
	if err := s.Close(); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"stream":     name,
		}.Errorf(h, "Failed to close step stream.")
	}
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
	line = strings.TrimSpace(line)
	if len(line) <= 6 || !(strings.HasPrefix(line, "@@@") && strings.HasSuffix(line, "@@@")) {
		return ""
	}
	return strings.TrimSpace(line[3 : len(line)-3])
}
