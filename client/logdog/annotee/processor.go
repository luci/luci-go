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
	"github.com/luci/luci-go/common/logdog/protocol"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/meter"
	"github.com/luci/luci-go/common/parallel"
	"golang.org/x/net/context"
)

const (
	// DefaultBufferSize is the Stream BufferSize value that will be used if no
	// buffer size is provided.
	DefaultBufferSize = 8192

	// DefaultMetadataUpdateInterval is the interval when updated metadata will
	// be pushed.
	DefaultMetadataUpdateInterval = (30 * time.Second)
)

var (
	textStreamArchetype = streamproto.Flags{
		ContentType: string(types.ContentTypeText),
		Type:        streamproto.StreamType(protocol.LogStreamDescriptor_TEXT),
	}

	metadataStreamArchetype = streamproto.Flags{
		ContentType: string(types.ContentTypeAnnotations),
		Type:        streamproto.StreamType(protocol.LogStreamDescriptor_DATAGRAM),
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
	// If this is < 0, metadata will be pushed every time it's updated.
	// If this is 0, DefaultMetadataUpdateInterval will be used.
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
	p.stepHandlers = map[string]*stepHandler{}
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

	annotation := extractAnnotation(line)
	h := (*stepHandler)(nil)
	if s.Annotate {
		if annotation != "" {
			// Append our annotation to the annotation state. This may cause our
			// annotation callbacks to be invoked.
			if err := p.astate.Append(annotation); err != nil {
				log.Fields{
					log.ErrorKey: err,
					"stream":     s.Name,
					"annotation": annotation,
				}.Errorf(p.Context, "Failed to process annotation.")
			}
		}

		// Use the step handler for the current step.
		h = p.getStepHandler(p.astate.CurrentStep(), true)
	} else {
		// Not handling annotations. Use our root step handler.
		h = p.getStepHandler(p.astate.RootStep(), true)
	}
	err := parallel.FanOutIn(func(taskC chan<- func() error) {
		// Write to our LogDog stream.
		taskC <- func() error {
			return h.writeBaseStream(s, line)
		}

		// If configured, tee to our tee stream.
		if s.Tee != nil && (annotation == "" || !s.StripAnnotations) {
			// Tee this to the Stream's configured Tee output.
			taskC <- func() error {
				return writeTextLine(s.Tee, line)
			}
		}
	})
	return err
}

// State returns the current annotation state.
func (p *Processor) State() *annotation.State {
	return p.astate
}

func (p *Processor) getStepHandler(step *annotation.Step, create bool) *stepHandler {
	name := step.CanonicalName()
	if h, ok := p.stepHandlers[name]; ok {
		return h
	}
	if !create {
		return nil
	}
	h := newStepHandler(p, step)
	p.stepHandlers[name] = h
	return h
}

func (p *Processor) closeStepHandler(step *annotation.Step) {
	h := p.getStepHandler(step, false)
	if h != nil {
		h.finish()
		delete(p.stepHandlers, step.CanonicalName())
	}
}

// Finish instructs the Processor to close any outstanding state. This should be
// called when all automatic state updates have completed in case any steps
// didn't properly close their state.
func (p *Processor) Finish() {
	// Close our step handlers.
	for _, h := range p.stepHandlers {
		h.finish()
	}
	p.stepHandlers = nil
}

type annotationCallbacks struct {
	*Processor
}

func (c *annotationCallbacks) StepClosed(step *annotation.Step) {
	c.closeStepHandler(step)
}

func (c *annotationCallbacks) Updated(step *annotation.Step) {
	h := c.getStepHandler(step, false)
	if h != nil {
		h.updated()
	}
}

func (c *annotationCallbacks) StepLogLine(step *annotation.Step, name types.StreamName, line string) {
	h := c.getStepHandler(step, true)
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
	h := c.getStepHandler(step, true)
	h.closeStream(name)
}

// stepHandler handles the steps associated with a specified stream.
type stepHandler struct {
	*annotation.Step

	p *Processor

	closed          bool
	annotationMeter meter.Meter
	streams         map[types.StreamName]streamclient.Stream
}

func newStepHandler(p *Processor, step *annotation.Step) *stepHandler {
	return &stepHandler{
		Step:    step,
		p:       p,
		streams: map[types.StreamName]streamclient.Stream{},
	}
}

func (h *stepHandler) String() string {
	return h.CanonicalName()
}

func (h *stepHandler) finish() {
	// Send one last annotation to summarize the state (will be no-op if
	// generation hasn't been updated), then flush our work queue.
	if err := h.sendAnnotationState(); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"step":       h,
		}.Errorf(h.p.Context, "Failed to send last annotation state.")
	}
	if h.annotationMeter != nil {
		h.annotationMeter.Stop()
		h.annotationMeter = nil
	}

	h.closeAllStreams()
	h.closed = true
}

func (h *stepHandler) writeBaseStream(s *Stream, line string) error {
	stream, err := h.getStream(h.BaseStream(s.Name), &textStreamArchetype)
	if err != nil {
		return err
	}
	return writeTextLine(stream, line)
}

func (h *stepHandler) writeAnnotationStream(d []byte) error {
	s, err := h.getStream(h.AnnotationStream(), &metadataStreamArchetype)
	if err != nil {
		return err
	}
	return s.WriteDatagram(d)
}

func (h *stepHandler) updated() {
	// Ignore updates after the step has closed.
	if h.closed {
		return
	}

	if err := h.sendAnnotationState(); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"step":       h,
		}.Errorf(h.p.Context, "Failed to send annotation state on update.")
	}
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
	f.Timestamp = clockflag.Time(clock.Now(h.p.Context))
	f.Name = streamproto.StreamNameFlag(name)
	s, err := h.p.Client.NewStream(f)
	if err != nil {
		return nil, err
	}
	h.streams[name] = s
	return s, nil
}

func (h *stepHandler) closeStream(name types.StreamName) {
	s := h.streams[name]
	if s != nil {
		if err := s.Close(); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"step":       h,
				"stream":     name,
			}.Errorf(h.p.Context, "Failed to close step stream.")
		}
		delete(h.streams, name)
	}
}

func (h *stepHandler) closeAllStreams() {
	for name, s := range h.streams {
		if err := s.Close(); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"step":       h,
				"stream":     name,
			}.Errorf(h.p.Context, "Failed to close step stream.")
		}
	}
	h.streams = nil
}

func (h *stepHandler) annotationMeterFlush(work []interface{}) {
	// Grab the latest update request.
	if len(work) == 0 {
		return
	}
	w := work[len(work)-1].([]byte)
	if err := h.writeAnnotationStream(w); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"step":       h,
		}.Errorf(h.p.Context, "Failed to write annotation state on meter flush.")
	}
}

func (h *stepHandler) sendAnnotationState() error {
	p := h.Step.Proto()
	data, err := proto.Marshal(p)
	if err != nil {
		return err
	}

	if h.annotationMeter == nil {
		// First update, force an immediate push.
		if err := h.writeAnnotationStream(data); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"step":       h,
			}.Errorf(h.p.Context, "Failed to write initial annotation state.")
		}
		h.annotationMeter = meter.New(h.p.Context, meter.Config{
			Delay:    h.p.MetadataUpdateInterval,
			Callback: h.annotationMeterFlush,
		})
	} else {
		// Otherwise, enqueue with our meter for dispatch.
		h.annotationMeter.AddWait(data)
	}
	return nil
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
