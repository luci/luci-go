// Copyright 2021 The LUCI Authors.
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

package pubsub

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/pubsub"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPullingBatchProcessor(t *testing.T) {
	t.Parallel()
	Convey("PBP works ", t, func() {
		ct := cvtesting.Test{MaxDuration: time.Minute}
		ctx := ct.SetUp(t)

		psSrv, err := NewTestPSServer(ctx)
		defer func() { _ = psSrv.Close() }()
		So(err, ShouldBeNil)

		processed := make(chan string, 100)
		defer close(processed)

		// Use this to inject errors.
		batchErrChan := make(chan error, 1)
		defer close(batchErrChan)

		pump := &PullingBatchProcessor{
			ProcessBatch: makeTestProcessBatch(processed, batchErrChan),
			ProjectID:    psSrv.ProjID,
			SubID:        psSrv.SubID,
		}

		Convey("non-erroring payload", func() {
			Convey("with concurrent batches", func() {
				pump.Options.ConcurrentBatches = 5
			})
			Convey("without concurrent batches", func() {
				pump.Options.ConcurrentBatches = 1
			})
			So(pump.Validate(), ShouldBeNil)
			nMessages := pump.Options.MaxBatchSize + 1

			sent := psSrv.PublishTestMessages(nMessages)
			So(sent, ShouldHaveLength, nMessages)
			expected := len(sent)
			received := make(stringset.Set, len(sent))

			processCtx, cancelProcess := context.WithCancel(ctx)
			done := runProcessAsync(processCtx, psSrv.Client, pump.process)
			for len(received) < expected {
				select {
				case <-ctx.Done():
					panic(fmt.Errorf("expected %d messages, only received %d after too long", expected, len(received)))
				case msg := <-processed:
					received.Add(msg)
				}
			}

			// There should be no more messages in processed.
			select {
			case msg := <-processed:
				panic(fmt.Sprintf("This test fails because there should be no more messages in this channel, but there was %q", msg))
			default:
			}
			cancelProcess()
			So(<-done, ShouldBeNil)
			So(received, ShouldResemble, sent)
		})

		Convey("With error", func() {
			// Configure the pump to handle one message at a time s.t. a single
			// batch results in the injected error.
			pump.Options.MaxBatchSize = 1
			pump.Options.ConcurrentBatches = 1
			So(pump.Validate(), ShouldBeNil)

			var processingError error
			var deliveriesExpected, processedMessagesExpected int
			var processErrorAssertion, sentVSReceivedAssertion func(actual any, expected ...any) string
			Convey("permanent", func() {
				processingError = fmt.Errorf("non-transient error")
				// Each of the messages should be delivered at least once.
				deliveriesExpected = 2
				// The error should be surfaced by .Process()
				processErrorAssertion = ShouldBeError

				processedMessagesExpected = 1

				sentVSReceivedAssertion = ShouldNotResemble
			})
			Convey("transient", func() {
				processingError = transient.Tag.Apply(fmt.Errorf("transient error"))
				// The message that failed transiently should be re-delivered at least once.
				deliveriesExpected = 3
				// The error should not be surfaced by .Process()
				processErrorAssertion = ShouldBeNil

				processedMessagesExpected = 2

				sentVSReceivedAssertion = ShouldResemble
			})

			// Publish two messages.
			sent := psSrv.PublishTestMessages(2)
			// Inject error to be returned by the processing of the first
			// batch-of-one-message.
			batchErrChan <- processingError

			// Run the pump.
			processCtx, cancelProcess := context.WithCancel(ctx)
			done := runProcessAsync(processCtx, psSrv.Client, pump.process)

			// Receive exactly as many distinct messages as expected.
			received := make(stringset.Set, processedMessagesExpected)
			for len(received) < processedMessagesExpected {
				select {
				case <-ctx.Done():
					panic(fmt.Errorf("expected %d messages, only received %d after too long", processedMessagesExpected, len(received)))
				case msg := <-processed:
					received.Add(msg)
				}
			}
			cancelProcess()

			// Ensure there were at least as many message deliveries as expected.
			actualDeliveries := 0
			for _, m := range psSrv.Messages() {
				actualDeliveries += m.Deliveries
			}
			So(actualDeliveries, ShouldBeGreaterThanOrEqualTo, deliveriesExpected)

			// Compare sent and received messages.
			So(sent, sentVSReceivedAssertion, received)

			// Check that the pump surfaced (or not) the error, as appropriate.
			So(<-done, processErrorAssertion)
		})

		Convey("Can enforce runTime", func() {
			// Publish a lot of messages
			sent := psSrv.PublishTestMessages(100)
			received := make(stringset.Set, len(sent))

			pump.Options.MaxBatchSize = 1
			pump.Options.ConcurrentBatches = 1
			pump.ProcessBatch = func(ctx context.Context, msgs []*pubsub.Message) error {
				// As soon as the first message is received, move the clock
				// past the runTime deadline.
				ct.Clock.Add(pump.Options.ReceiveDuration)
				for _, msg := range msgs {
					received.Add(string(msg.Data))
				}
				return nil
			}
			So(pump.Validate(), ShouldBeNil)

			done := runProcessAsync(ctx, psSrv.Client, pump.process)

			select {
			case <-ctx.Done():
				panic("This test failed because .Process() did not respect .runTime")
			case err := <-done:
				So(err, ShouldBeNil)
			}
			So(len(received), ShouldBeLessThan, len(sent))
		})
	})
}

func runProcessAsync(ctx context.Context, client *pubsub.Client, payload func(context.Context, *pubsub.Client) error) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		done <- payload(ctx, client)
	}()
	return done
}

// makeTestProcessBatch returns a trivial batch processing function (a closure).
//
// It puts every message in the given output channel, and if an error is
// available at batchErrChan, returns that instead of nil.
func makeTestProcessBatch(processed chan<- string, batchErrChan <-chan error) func(ctx context.Context, msgs []*pubsub.Message) error {
	return func(ctx context.Context, msgs []*pubsub.Message) error {

		select {
		case err := <-batchErrChan:
			return err
		default:

		}
		for _, msg := range msgs {
			processed <- string(msg.Data)
		}
		return nil
	}
}
