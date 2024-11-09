// Copyright 2016 The LUCI Authors.
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

// Package bqlog provides a mechanism to asynchronously log rows to BigQuery.
//
// Deprecated: this package depends on Pull Task Queues which are deprecated and
// not available from the GAE second-gen runtime or from Kubernetes. The
// replacement is go.chromium.org/luci/server/tq PubSub tasks, plus a PubSub
// push subscription with a handler that simply inserts rows into BigQuery.
package bqlog

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/bigquery"
	bqapi "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/gae/service/taskqueue"
	"go.chromium.org/luci/server/auth"
)

const (
	defaultBatchesPerRequest  = 250
	defaultMaxParallelUploads = 64
	defaultFlushTimeout       = time.Minute
)

var (
	// This can be used to estimate how many events are produced.
	insertedEntryCount = metric.NewCounter(
		"luci/bqlog/inserted_entry_count",
		"Total number of log entries successfully added in Insert(...).",
		nil,
		field.String("table")) // "<projID>/<datasetID>/<tableID>"

	// To track the performance of Insert(...).
	insertLatency = metric.NewCumulativeDistribution(
		"luci/bqlog/insert_latency",
		"Distribution of Insert(...) call latencies.",
		&types.MetricMetadata{Units: types.Milliseconds},
		distribution.DefaultBucketer,
		field.String("table"),  // "<projID>/<datasetID>/<tableID>"
		field.String("status")) // "ok" or "fail"

	// To track the performance of 'Flush'.
	flushLatency = metric.NewCumulativeDistribution(
		"luci/bqlog/flush_latency",
		"Distribution of Flush(...) call latencies.",
		&types.MetricMetadata{Units: types.Milliseconds},
		distribution.DefaultBucketer,
		field.String("table"),  // "<projID>/<datasetID>/<tableID>"
		field.String("status")) // "ok", "fail" or "warning"

	// This is perhaps the most important metric, since it shows a number of rows
	// skipped during the flush due to schema mismatch or other BigQuery errors.
	flushedEntryCount = metric.NewCounter(
		"luci/bqlog/flushed_entry_count",
		"Total number of rows sent to BigQuery (including rejected rows).",
		nil,
		field.String("table"),  // "<projID>/<datasetID>/<tableID>"
		field.String("status")) // "ok" or whatever error reason BigQuery returns

	// Stats of individual BigQuery API calls (including retries).
	bigQueryLatency = metric.NewCumulativeDistribution(
		"luci/bqlog/bigquery_latency",
		"Distribution of BigQuery API call latencies.",
		&types.MetricMetadata{Units: types.Milliseconds},
		distribution.DefaultBucketer,
		field.String("table"),  // "<projID>/<datasetID>/<tableID>"
		field.String("method"), // name of the API method, e.g. "insertAll"
		field.String("status")) // "ok, "http_400", ..., "timeout" or "unknown"

	// This can be used to estimate a queuing backlog.
	pullQueueLen = metric.NewInt(
		"luci/bqlog/pullqueue_len",
		"Number of tasks in the associated Pull Queue (prior to Flush call).",
		nil,
		field.String("table")) // "<projID>/<datasetID>/<tableID>"

	// This estimates queuing delay and any GAE scheduling hickups.
	pullQueueLatency = metric.NewFloat(
		"luci/bqlog/pullqueue_latency",
		"Age of the oldest task in the queue or 0 if the queue is empty.",
		&types.MetricMetadata{Units: types.Milliseconds},
		field.String("table")) // "<projID>/<datasetID>/<tableID>"
)

// Log can be used to insert entries into a BigQuery table.
type Log struct {
	// QueueName is a name of a pull queue to use as a buffer for inserts.
	//
	// Required. It must be defined in queue.yaml file and it must not be used by
	// any other Log object.
	QueueName string

	// ProjectID is Cloud Project that owns the dataset.
	//
	// If empty, will be derived from the current app ID.
	ProjectID string

	// DatasetID identifies the already existing dataset that contains the table.
	//
	// Required.
	DatasetID string

	// TableID identifies the name of the table in the dataset.
	//
	// Required. The table must exist already.
	TableID string

	// BatchesPerRequest is how many batches of entries to send in one BQ insert.
	//
	// A call to 'Insert' generates one batch of entries, thus BatchesPerRequest
	// essentially specifies how many 'Insert's to clump together when sending
	// data to BigQuery. If your Inserts are known to be huge, lowering this value
	// may help to avoid hitting memory limits.
	//
	// Default is 250. It assumes your batches are very small (1-3 rows), which
	// is usually the case if events are generated by online RPC handlers.
	BatchesPerRequest int

	// MaxParallelUploads is how many parallel ops to do when flushing.
	//
	// We limit it to avoid hitting OOM errors on GAE.
	//
	// Default is 64.
	MaxParallelUploads int

	// FlushTimeout is maximum duration to spend in fetching from Pull Queue in
	// 'Flush'.
	//
	// We limit it to make sure 'Flush' has a chance to finish running before
	// GAE kills it by deadline. Next time 'Flush' is called, it will resume
	// flushing from where it left off.
	//
	// Note that 'Flush' can run for slightly longer, since it waits for all
	// pulled data to be flushed before returning.
	//
	// Default is 1 min.
	FlushTimeout time.Duration

	// DumpEntriesToLogger makes 'Insert' log all entries (at debug level).
	DumpEntriesToLogger bool

	// DryRun disables the actual uploads (keeps the local logging though).
	DryRun bool

	// insertMock is used to mock BigQuery insertAll call in tests.
	insertMock func(context.Context, *bqapi.TableDataInsertAllRequest) (*bqapi.TableDataInsertAllResponse, error)
	// beforeSendChunk is used in tests to signal that 'sendChunk' is called.
	beforeSendChunk func(context.Context, []*taskqueue.Task)
}

// rawEntry is a single structured entry in the log.
//
// It gets gob-serialized and put into Task Queue.
type rawEntry struct {
	InsertID string
	Data     map[string]bqapi.JsonValue // more like map[string]json.RawMessage
}

func init() {
	gob.Register(json.RawMessage{})
}

// valuesToJSON converts bigquery.Value map to a simplest JSON-marshallable
// bqapi.JsonValue map.
//
// bigquery.Value can actually be pretty complex (e.g. time.Time, big.Rat),
// which makes it difficult to put into gob-encodable rawEntry.
//
// So instead we convert it to JSON before putting into Gob (bqapi library does
// it later anyway). But since BigQuery API wants a map[string]bqapi.JsonValue
// we return map[string]json.RawMessage instead of complete raw []byte.
func valuesToJSON(in map[string]bigquery.Value) (map[string]bqapi.JsonValue, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(map[string]bqapi.JsonValue, len(in))
	for k, v := range in {
		blob, err := json.Marshal(v)
		if err != nil {
			return nil, errors.Annotate(err, "failed to JSON-serialize key %q", k).Err()
		}
		out[k] = json.RawMessage(blob)
	}
	return out, nil
}

// Insert adds a bunch of entries to the buffer of pending entries.
//
// It will reuse existing datastore transaction (if any). This allows to
// log entries transactionally when changing something in the datastore.
//
// Empty inserts IDs will be replaced with autogenerated ones (they start with
// 'bqlog:'). Entries not matching the schema are logged and skipped during
// the flush.
func (l *Log) Insert(ctx context.Context, rows ...bigquery.ValueSaver) (err error) {
	if len(rows) == 0 {
		return nil
	}

	entries := make([]rawEntry, len(rows))
	for i, r := range rows {
		values, iid, err := r.Save()
		if err != nil {
			return errors.Annotate(err, "failure when saving row #%d", i).Err()
		}
		if entries[i].Data, err = valuesToJSON(values); err != nil {
			return errors.Annotate(err, "failure when serializing row #%d", i).Err()
		}
		entries[i].InsertID = iid
	}

	if l.DumpEntriesToLogger && logging.IsLogging(ctx, logging.Debug) {
		for idx, entry := range entries {
			blob, err := json.MarshalIndent(entry.Data, "", "  ")
			if err != nil {
				logging.WithError(err).Errorf(ctx, "Failed to serialize the row #%d", idx)
			} else {
				logging.Debugf(ctx, "BigQuery row #%d for %s/%s:\n%s", idx, l.DatasetID, l.TableID, blob)
			}
		}
	}

	if l.DryRun {
		return nil
	}

	// We need tableRef to report the metrics, thus an error to get tableRef is
	// NOT reported to tsmon. It happens only if TableID or DatasetID are
	// malformed.
	tableRef, err := l.tableRef(ctx)
	if err != nil {
		return err
	}

	startTime := clock.Now(ctx)
	defer func() {
		dt := clock.Since(ctx, startTime)
		status := "fail"
		if err == nil {
			status = "ok"
			insertedEntryCount.Add(ctx, int64(len(entries)), tableRef)
		}
		insertLatency.Add(ctx, float64(dt.Nanoseconds()/1e6), tableRef, status)
	}()

	buf := bytes.Buffer{}
	if err := gob.NewEncoder(&buf).Encode(entries); err != nil {
		return err
	}
	return taskqueue.Add(ctx, l.QueueName, &taskqueue.Task{
		Method:  "PULL",
		Payload: buf.Bytes(),
	})
}

// Flush pulls buffered rows from Pull Queue and sends them to BigQuery.
//
// Must be called periodically from some cron job. It is okay to call 'Flush'
// concurrently from multiple processes to speed up the upload.
//
// It succeeds if all entries it attempted to send were successfully handled by
// BigQuery. If some entries are malformed, it logs the error and skip them,
// so they don't get stuck in the pending buffer forever. This corresponds to
// 'skipInvalidRows=true' in 'insertAll' BigQuery call.
//
// Returns number of rows sent to BigQuery. May return both non zero number of
// rows and an error if something bad happened midway.
func (l *Log) Flush(ctx context.Context) (int, error) {
	tableRef, err := l.tableRef(ctx)
	if err != nil {
		return 0, err
	}
	ctx = logging.SetFields(ctx, logging.Fields{"table": tableRef})
	logging.Infof(ctx, "Flush started")

	startTime := clock.Now(ctx)

	softDeadline := startTime.Add(l.flushTimeout()) // when to stop pulling tasks
	hardDeadline := softDeadline.Add(time.Minute)   // when to abort all calls

	softDeadlineCtx, cancel := clock.WithDeadline(ctx, softDeadline)
	defer cancel()
	hardDeadlineCtx, cancel := clock.WithDeadline(ctx, hardDeadline)
	defer cancel()

	stats, err := taskqueue.Stats(ctx, l.QueueName)
	if err != nil {
		logging.WithError(err).Warningf(ctx, "Failed to query stats of queue %q", l.QueueName)
	} else {
		var age time.Duration
		if eta := stats[0].OldestETA; !eta.IsZero() {
			age = clock.Now(ctx).Sub(eta)
		}
		pullQueueLatency.Set(ctx, float64(age.Nanoseconds()/1e6), tableRef)
		pullQueueLen.Set(ctx, int64(stats[0].Tasks), tableRef)
	}

	// Lease pending upload tasks, split them into 'BatchesPerRequest' chunks,
	// upload all chunks in parallel (limiting the number of concurrent
	// uploads).
	flusher := asyncFlusher{
		Context:  hardDeadlineCtx,
		TableRef: tableRef,
		Insert:   l.insert,
	}
	flusher.start(l.maxParallelUploads())

	// We lease batches until we run out of time or there's nothing more to lease.
	// On errors or RPC deadlines we slow down, but carry on. We lease until hard
	// deadline. Note that losing a lease is not a catastrophic event: BigQuery
	// still should be able to remove duplicates based on insertID.
	var lastLeaseErr error
	sleep := time.Second
	for clock.Now(ctx).Before(softDeadline) {
		rpcCtx, cancel := clock.WithTimeout(softDeadlineCtx, 15*time.Second) // RPC timeout
		tasks, err := taskqueue.Lease(rpcCtx, l.batchesPerRequest(), l.QueueName, hardDeadline.Sub(clock.Now(ctx)))
		cancel()
		if err != nil {
			lastLeaseErr = err
			if clock.Now(ctx).Add(sleep).After(softDeadline) {
				logging.Warningf(ctx, "Error while leasing, giving up: %s", err)
				break
			}
			logging.Warningf(ctx, "Error while leasing, sleeping %s: %s", err, sleep)
			clock.Sleep(clock.Tag(softDeadlineCtx, "lease-retry"), sleep)
			sleep *= 2
			continue
		}
		sleep = time.Second
		if len(tasks) == 0 {
			break
		}
		if l.beforeSendChunk != nil {
			l.beforeSendChunk(ctx, tasks)
		}
		flusher.sendChunk(chunk{
			Tasks: tasks,
			Done: func(ctx context.Context) {
				logging.Infof(ctx, "Deleting %d tasks from the task queue", len(tasks))
				ctx, cancel := clock.WithTimeout(ctx, 30*time.Second) // RPC timeout
				defer cancel()
				if err := taskqueue.Delete(ctx, l.QueueName, tasks...); err != nil {
					logging.WithError(err).Errorf(ctx, "Failed to delete some tasks")
				}
			},
		})
	}

	sent, err := flusher.waitAll()
	logging.Infof(ctx, "Flush finished, sent %d rows", sent)

	if err == nil {
		err = lastLeaseErr
	}

	dt := clock.Since(ctx, startTime)
	status := "ok"
	switch {
	case err != nil && sent == 0:
		status = "fail"
	case err != nil && sent != 0:
		status = "warning"
	}
	flushLatency.Add(ctx, float64(dt.Nanoseconds()/1e6), tableRef, status)

	return sent, err
}

func (l *Log) batchesPerRequest() int {
	if l.BatchesPerRequest > 0 {
		return l.BatchesPerRequest
	}
	return defaultBatchesPerRequest
}

func (l *Log) maxParallelUploads() int {
	if l.MaxParallelUploads > 0 {
		return l.MaxParallelUploads
	}
	return defaultMaxParallelUploads
}

func (l *Log) flushTimeout() time.Duration {
	if l.FlushTimeout > 0 {
		return l.FlushTimeout
	}
	return defaultFlushTimeout
}

func (l *Log) insert(ctx context.Context, r *bqapi.TableDataInsertAllRequest) (*bqapi.TableDataInsertAllResponse, error) {
	if l.insertMock != nil {
		return l.insertMock(ctx, r)
	}
	return l.doInsert(ctx, r)
}

// projID returns ProjectID or a GAE app ID if ProjectID is "".
func (l *Log) projID(ctx context.Context) string {
	if l.ProjectID == "" {
		return info.TrimmedAppID(ctx)
	}
	return l.ProjectID
}

// tableRef returns an identifier of the table in BigQuery.
//
// Returns an error if Log is misconfigred.
func (l *Log) tableRef(ctx context.Context) (string, error) {
	projID := l.projID(ctx)
	if projID == "" || strings.ContainsRune(projID, '/') {
		return "", fmt.Errorf("invalid project ID %q", projID)
	}
	if l.DatasetID == "" || strings.ContainsRune(l.DatasetID, '/') {
		return "", fmt.Errorf("invalid dataset ID %q", l.DatasetID)
	}
	if l.TableID == "" || strings.ContainsRune(l.TableID, '/') {
		return "", fmt.Errorf("invalid table ID %q", l.TableID)
	}
	return fmt.Sprintf("%s/%s/%s", projID, l.DatasetID, l.TableID), nil
}

// bigQuery constructs an instance of BigQuery API client with proper auth.
func (l *Log) bigQuery(ctx context.Context) (*bqapi.Service, error) {
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(bqapi.BigqueryScope))
	if err != nil {
		return nil, err
	}
	return bqapi.New(&http.Client{Transport: tr})
}

// doInsert does the actual BigQuery call.
//
// It is mocked in tests.
func (l *Log) doInsert(ctx context.Context, req *bqapi.TableDataInsertAllRequest) (*bqapi.TableDataInsertAllResponse, error) {
	ctx, cancel := clock.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	logging.Infof(ctx, "Sending %d rows to BigQuery", len(req.Rows))
	bq, err := l.bigQuery(ctx)
	if err != nil {
		return nil, err
	}
	call := bq.Tabledata.InsertAll(l.projID(ctx), l.DatasetID, l.TableID, req)
	return call.Context(ctx).Do()
}

// asyncFlusher implements parallel flush to BigQuery.
type asyncFlusher struct {
	Context  context.Context // the root context
	TableRef string          // for monitoring metrics
	Insert   func(context.Context, *bqapi.TableDataInsertAllRequest) (*bqapi.TableDataInsertAllResponse, error)

	index int32 // incremented in each 'sendChunk' call

	chunks chan chunk
	wg     sync.WaitGroup

	mu       sync.Mutex
	errs     errors.MultiError // collected errors from all 'sendChunk' ops
	rowsSent int               // total number of rows sent to BigQuery
}

// chunk is a bunch of pendingBatches flushed together.
type chunk struct {
	Tasks []*taskqueue.Task
	Done  func(context.Context) // called in a goroutine after successful upload

	index int32 // used only for logging, see sendChunk
}

// start launches internal goroutines that upload data.
func (f *asyncFlusher) start(numParallel int) {
	f.chunks = make(chan chunk)
	for i := 0; i < numParallel; i++ {
		f.wg.Add(1)
		go func() {
			defer f.wg.Done()
			f.uploaderLoop()
		}()
	}
}

// waitAll waits for completion of all pending 'sendChunk' calls and stops all
// internal goroutines.
//
// Returns total number of rows sent and all the errors.
func (f *asyncFlusher) waitAll() (int, error) {
	close(f.chunks)
	f.wg.Wait()
	if len(f.errs) == 0 {
		return f.rowsSent, nil
	}
	return f.rowsSent, f.errs
}

// ctx returns a context to use for logging operations happening to some chunk.
func (f *asyncFlusher) ctx(chunkIndex int32) context.Context {
	return logging.SetField(f.Context, "chunk", chunkIndex)
}

// sendChunk starts an asynchronous operation to upload data to BigQuery.
//
// Can block if too many parallel uploads are already underway. Panics if called
// before 'start' or after 'waitAll'.
//
// On successful upload it deletes the tasks from Pull Queue.
func (f *asyncFlusher) sendChunk(c chunk) {
	c.index = atomic.AddInt32(&f.index, 1)
	logging.Infof(f.ctx(c.index), "Chunk with %d batches queued", len(c.Tasks))
	f.chunks <- c
}

// uploaderLoop runs in a separate goroutine.
func (f *asyncFlusher) uploaderLoop() {
	for chunk := range f.chunks {
		ctx := f.ctx(chunk.index)
		logging.Infof(ctx, "Chunk flush starting")
		sent, err := f.upload(ctx, chunk)
		f.mu.Lock()
		if err == nil {
			f.rowsSent += sent
		} else {
			f.errs = append(f.errs, err)
		}
		f.mu.Unlock()
		logging.Infof(ctx, "Chunk flush finished")
	}
}

// upload sends the rows to BigQuery.
func (f *asyncFlusher) upload(ctx context.Context, chunk chunk) (int, error) {
	// Give up right away if the context is already dead.
	if err := ctx.Err(); err != nil {
		logging.WithError(err).Errorf(ctx, "Skipping upload")
		return 0, err
	}

	// Collect all pending data into an array of rows. We cheat here when
	// unpacking gob-serialized entries and use 'bigquery.JsonValue' instead of
	// 'biquery.Value{}' in Data. They are compatible. This cheat avoids to do
	// a lot of allocations just to appease the type checker.
	var rows []*bqapi.TableDataInsertAllRequestRows
	var entries []rawEntry

	for _, task := range chunk.Tasks {
		ctx := logging.SetField(ctx, "name", task.Name)

		if err := gob.NewDecoder(bytes.NewReader(task.Payload)).Decode(&entries); err != nil {
			logging.WithError(err).Errorf(ctx, "Failed to gob-decode pending batch, it will be skipped")
			continue
		}

		for i, entry := range entries {
			insertID := entry.InsertID
			if insertID == "" {
				// The task names are autogenerated and guaranteed to be unique.
				// Use them as a base for autogenerated insertID.
				insertID = fmt.Sprintf("bqlog:%s:%d", task.Name, i)
			}
			rows = append(rows, &bqapi.TableDataInsertAllRequestRows{
				InsertId: insertID,
				Json:     entry.Data,
			})
			// We need to nil Data maps here to be able to reuse 'entries' array
			// capacity later. Otherwise gob decode "discovers" maps and overwrites
			// their data in-place, spoiling 'rows'.
			entries[i].Data = nil
		}

		entries = entries[:0]
	}

	if len(rows) == 0 {
		chunk.Done(ctx)
		return 0, nil
	}

	// Now actually send all the entries with retries.
	var lastResp *bqapi.TableDataInsertAllResponse
	taggedCtx := clock.Tag(ctx, "insert-retry") // used by tests
	err := retry.Retry(taggedCtx, transient.Only(f.retryParams), func() error {
		startTime := clock.Now(ctx)
		var err error
		lastResp, err = f.Insert(ctx, &bqapi.TableDataInsertAllRequest{
			SkipInvalidRows: true, // they will be reported in lastResp.InsertErrors
			Rows:            rows,
		})
		code := 0
		status := "ok"
		if gerr, _ := err.(*googleapi.Error); gerr != nil {
			code = gerr.Code
			status = fmt.Sprintf("http_%d", code)
		} else if ctx.Err() != nil {
			status = "timeout"
		} else if err != nil {
			status = "unknown"
		}
		dt := clock.Since(ctx, startTime)
		bigQueryLatency.Add(ctx, float64(dt.Nanoseconds()/1e6), f.TableRef, "insertAll", status)
		if code >= 500 {
			return transient.Tag.Apply(err)
		}
		return err
	}, func(err error, wait time.Duration) {
		logging.Fields{
			logging.ErrorKey: err,
			"wait":           wait,
		}.Warningf(ctx, "Failed to send data to BigQuery")
	})
	if err != nil {
		logging.WithError(err).Errorf(ctx, "Failed to send data to BigQuery")
		if !transient.Tag.In(err) && err != context.DeadlineExceeded {
			chunk.Done(ctx)
		}
		return 0, err
	}

	if success := len(rows) - len(lastResp.InsertErrors); success > 0 {
		flushedEntryCount.Add(ctx, int64(success), f.TableRef, "ok")
	}

	if len(lastResp.InsertErrors) != 0 {
		// Use only first error as a sample. Dumping them all is impractical.
		blob, _ := json.MarshalIndent(lastResp.InsertErrors[0].Errors, "", "  ")
		logging.Errorf(ctx, "%d rows weren't accepted, sample error:\n%s", len(lastResp.InsertErrors), blob)

		// Categorize errors by reason to dump them to monitoring. We look only
		// at first suberror.
		perReason := make(map[string]int64, 5)
		for _, err := range lastResp.InsertErrors {
			reason := "unknown"
			if len(err.Errors) > 0 {
				reason = err.Errors[0].Reason // usually just "invalid"
			}
			perReason[reason]++
		}
		for reason, count := range perReason {
			flushedEntryCount.Add(ctx, count, f.TableRef, reason)
		}
	}

	chunk.Done(ctx)
	return len(rows), nil
}

// retryParams defines retry strategy for handling BigQuery transient errors.
func (f *asyncFlusher) retryParams() retry.Iterator {
	return &retry.ExponentialBackoff{
		Limited: retry.Limited{
			Delay:    50 * time.Millisecond,
			Retries:  50,
			MaxTotal: 45 * time.Second,
		},
		Multiplier: 2,
	}
}
