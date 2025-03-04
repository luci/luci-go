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
	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cv/internal/cvtesting"
)

func TestPullingBatchProcessor(t *testing.T) {
	t.Parallel()
	ftt.Run("PBP works ", t, func(t *ftt.Test) {
		ct := cvtesting.Test{MaxDuration: time.Minute}
		ctx := ct.SetUp(t)

		psSrv, err := NewTestPSServer(ctx)
		defer func() { _ = psSrv.Close() }()
		assert.NoErr(t, err)

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

		t.Run("non-erroring payload", func(t *ftt.Test) {
			t.Run("with concurrent batches", func(t *ftt.Test) {
				pump.Options.ConcurrentBatches = 5
			})
			t.Run("without concurrent batches", func(t *ftt.Test) {
				pump.Options.ConcurrentBatches = 1
			})
			assert.NoErr(t, pump.Validate())
			nMessages := pump.Options.MaxBatchSize + 1

			sent := psSrv.PublishTestMessages(nMessages)
			assert.Loosely(t, sent, should.HaveLength(nMessages))
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
			assert.NoErr(t, <-done)
			assert.That(t, received, should.Match(sent))
		})

		t.Run("With error", func(t *ftt.Test) {
			// Configure the pump to handle one message at a time s.t. a single
			// batch results in the injected error.
			pump.Options.MaxBatchSize = 1
			pump.Options.ConcurrentBatches = 1
			assert.NoErr(t, pump.Validate())

			doCheck := func(t testing.TB, processingError error, deliveriesExpected, processedMessagesExpected int, processErrorAssertion comparison.Func[error], sentVSReceivedAssertion func(stringset.Set, ...cmp.Option) comparison.Func[stringset.Set]) {
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
				assert.Loosely(t, actualDeliveries, should.BeGreaterThanOrEqual(deliveriesExpected))

				// Compare sent and received messages.
				assert.Loosely(t, sent, sentVSReceivedAssertion(received))

				// Check that the pump surfaced (or not) the error, as appropriate.
				assert.Loosely(t, <-done, processErrorAssertion)
			}

			t.Run("permanent", func(t *ftt.Test) {
				doCheck(
					t, fmt.Errorf("non-transient error"),
					// Each of the messages should be delivered at least once.
					2, // deliveriesExpected
					1, // processedMessagesExpected
					// The error should be surfaced by .Process()
					should.ErrLike("non-transient error"),
					should.NotMatch[stringset.Set],
				)
			})

			t.Run("transient", func(t *ftt.Test) {
				doCheck(
					t, transient.Tag.Apply(fmt.Errorf("transient error")),
					// The message that failed transiently should be re-delivered at least once.
					3, // deliveriesExpected
					2, // processedMessagesExpected
					// The error should be surfaced by .Process()
					should.ErrLike(nil),
					should.Match[stringset.Set],
				)
			})

		})

		t.Run("Can enforce runTime", func(t *ftt.Test) {
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
			assert.NoErr(t, pump.Validate())

			done := runProcessAsync(ctx, psSrv.Client, pump.process)

			select {
			case <-ctx.Done():
				panic("This test failed because .Process() did not respect .runTime")
			case err := <-done:
				assert.NoErr(t, err)
			}
			assert.Loosely(t, len(received), should.BeLessThan(len(sent)))
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
