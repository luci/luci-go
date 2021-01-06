// Copyright 2020 The LUCI Authors.
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

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/storage"
	"github.com/maruel/subcommands"
	"golang.org/x/time/rate"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/rts/presubmit/eval/history"
	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

type baseHistoryRun struct {
	out          string
	startTime    time.Time
	endTime      time.Time
	builderRegex string
	testIDRegex  string

	authOpt       *auth.Options
	authenticator *auth.Authenticator
	http          *http.Client
}

func (r *baseHistoryRun) RegisterBaseFlags(fs *flag.FlagSet) {
	fs.StringVar(&r.out, "out", "", "Path to the output directory")
	fs.Var(luciflag.Date(&r.startTime), "from", "Fetch results starting from this date; format: 2020-01-15")
	fs.Var(luciflag.Date(&r.endTime), "to", "Fetch results until this date; format: 2020-01-15")
	fs.StringVar(&r.builderRegex, "builder", ".*", "A regular expression for builder. Implicitly wrapped with ^ and $.")
	fs.StringVar(&r.testIDRegex, "test", ".*", "A regular expression for test. Implicitly wrapped with ^ and $.")
}

func (r *baseHistoryRun) ValidateBaseFlags() error {
	switch {
	case r.out == "":
		return errors.New("-out is required")
	case r.startTime.IsZero():
		return errors.New("-from is required")
	case r.endTime.IsZero():
		return errors.New("-to is required")
	case r.endTime.Before(r.startTime):
		return errors.New("the -to date must not be before the -from date")
	default:
		return nil
	}
}

// Init initializes r. Validates flags as well.
func (r *baseHistoryRun) Init(ctx context.Context) error {
	if err := r.ValidateBaseFlags(); err != nil {
		return err
	}

	r.authenticator = auth.NewAuthenticator(ctx, auth.InteractiveLogin, *r.authOpt)
	var err error
	r.http, err = r.authenticator.Client()
	return err
}

// runAndFetchResults runs the BigQuery query and saves results to r.out directory,
// as GZIP-compressed JSON Lines files.
func (r *baseHistoryRun) runAndFetchResults(ctx context.Context, sql string, extraParams ...bigquery.QueryParameter) error {
	fmt.Printf("starting a BigQuery query...\n")
	table, err := r.runQuery(ctx, sql, extraParams...)
	if err != nil {
		return err
	}

	logging.Infof(ctx, "extracting results...\n")
	return r.fetchResults(ctx, table)
}

// runQuery runs the query and returns the table with results.
func (r *baseHistoryRun) runQuery(ctx context.Context, sql string, extraParams ...bigquery.QueryParameter) (*bigquery.Table, error) {
	client, err := newBQClient(ctx, r.authenticator)
	if err != nil {
		return nil, err
	}

	// Prepare the query.

	prepRe := func(rgx string) string {
		if rgx == "" || rgx == ".*" {
			return ""
		}
		return fmt.Sprintf("^(%s)$", rgx)
	}

	q := client.Query(sql)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "startTime", Value: r.startTime},
		{Name: "endTime", Value: r.endTime},
		{Name: "builder_regexp", Value: prepRe(r.builderRegex)},
		{Name: "test_id_regexp", Value: prepRe(r.testIDRegex)},
	}
	q.Parameters = append(q.Parameters, extraParams...)

	// Run the query.

	job, err := q.Run(ctx)
	if err != nil {
		return nil, err
	}
	if err := waitForSuccess(ctx, job); err != nil {
		return nil, err
	}

	cfg, err := job.Config()
	if err != nil {
		return nil, err
	}
	return cfg.(*bigquery.QueryConfig).Dst, nil
}

// fetchResults fetches the table to GZIP-compressed JSON Lines files in r.out
// directory.
func (r *baseHistoryRun) fetchResults(ctx context.Context, table *bigquery.Table) error {
	// The fetching processing is done in two phases:
	// 1. Extract the table to GCS.
	//    This also takes care of the converting from tabular format to a file format.
	// 2. Download the prepared files to the destination directory.

	if err := r.prepareOutDir(); err != nil {
		return err
	}

	// Extract the table to GCS.
	bucketName := "chrome-rts"
	dirName := fmt.Sprintf("tmp/extract-%s", table.TableID)
	gcsRef := &bigquery.GCSReference{
		// Start the object name with a date, so that the user can merge
		// data directories if needed.
		URIs:              []string{fmt.Sprintf("gs://%s/%s/%s-*.jsonl.gz", bucketName, dirName, r.startTime.Format("2006-01-02"))},
		DestinationFormat: bigquery.JSON,
		Compression:       bigquery.Gzip,
	}
	job, err := table.ExtractorTo(gcsRef).Run(ctx)
	if err != nil {
		return err
	}
	if err := waitForSuccess(ctx, job); err != nil {
		return errors.Annotate(err, "extract job %q failed", job.ID()).Err()
	}

	// Fetch the extracted files from GCS to the file system.
	gcs, err := storage.NewClient(ctx, option.WithHTTPClient(r.http))
	if err != nil {
		return err
	}
	bucket := gcs.Bucket(bucketName)
	// Find all files in the temp GCS dir and fetch them all, concurrently.
	iter := bucket.Objects(ctx, &storage.Query{Prefix: dirName})
	return parallel.WorkPool(100, func(work chan<- func() error) {
		for {
			objAttrs, err := iter.Next()
			switch {
			case err == iterator.Done:
				return
			case err != nil:
				work <- func() error { return err }
				return
			}

			// Fetch the file.
			work <- func() error {
				// Prepare the source.
				rd, err := bucket.Object(objAttrs.Name).NewReader(ctx)
				if err != nil {
					return err
				}
				defer rd.Close()

				// Prepare the sink.
				f, err := os.Create(filepath.Join(r.out, path.Base(objAttrs.Name)))
				if err != nil {
					return err
				}
				defer f.Close()

				_, err = io.Copy(f, rd)
				return err
			}
		}
	})
}

// prepareOutDir ensures that r.out dir exists and does not have .jsonl.gz
// files.
func (r *baseHistoryRun) prepareOutDir() error {
	if err := os.MkdirAll(r.out, 0777); err != nil {
		return err
	}

	// Remove existing .jsonl.gz files.
	existing, err := filepath.Glob(filepath.Join(r.out, "*.jsonl.gz"))
	if err != nil {
		return err
	}
	for _, f := range existing {
		if err := os.Remove(f); err != nil {
			return errors.Annotate(err, "failed to remove %q", f).Err()
		}
	}
	return nil
}

func waitForSuccess(ctx context.Context, job *bigquery.Job) error {
	st, err := job.Wait(ctx)
	if err != nil {
		return err
	}
	return st.Err()
}

// TODO(nodir): transform the code below into a new subcommand fetch-rejections
// based on baseHistoryRun.

func cmdPresubmitHistory(authOpt *auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `presubmit-history`,
		ShortDesc: "fetch presubmit history",
		LongDesc: text.Doc(`
			Fetch presubmit history, suitable for RTS evaluation.
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &presubmitHistoryRun{authOpt: authOpt}
			r.Flags.StringVar(&r.out, "out", "", "Path to the output file")
			r.Flags.Var(luciflag.Date(&r.startTime), "from", "Fetch results starting from this date; format: 2020-01-02")
			r.Flags.Var(luciflag.Date(&r.endTime), "to", "Fetch results until this date; format: 2020-01-02")
			r.Flags.StringVar(&r.builderRegex, "builder", ".*", "A regular expression for builder. Implicitly wrapped with ^ and $.")
			r.Flags.StringVar(&r.testIDRegex, "test", ".*", "A regular expression for test. Implicitly wrapped with ^ and $.")
			r.Flags.IntVar(&r.minCLFlakes, "min-cl-flakes", 5, text.Doc(`
				In order to conlude that a test variant is flaky and exclude it from analysis,
				it must have mixed results in <min-cl-flakes> unique CLs.
			`))
			return r
		},
	}
}

type presubmitHistoryRun struct {
	baseCommandRun
	out          string
	startTime    time.Time
	endTime      time.Time
	builderRegex string
	testIDRegex  string
	minCLFlakes  int

	gerrit *gerritClient

	authenticator *auth.Authenticator
	authOpt       *auth.Options

	mu                    sync.Mutex
	w                     *history.Writer
	recordsWrote          int
	recordCountNextReport time.Time
}

func (r *presubmitHistoryRun) validateFlags() error {
	switch {
	case r.out == "":
		return errors.New("-out is required")
	case r.startTime.IsZero():
		return errors.New("-from is required")
	case r.endTime.IsZero():
		return errors.New("-to is required")
	case r.endTime.Before(r.startTime):
		return errors.New("the -to date must not be before the -from date")
	default:
		return nil
	}
}

func (r *presubmitHistoryRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if len(args) != 0 {
		return r.done(errors.New("unexpected positional arguments"))
	}

	if err := r.validateFlags(); err != nil {
		return r.done(err)
	}

	r.authenticator = auth.NewAuthenticator(ctx, auth.InteractiveLogin, *r.authOpt)

	var err error
	if r.gerrit, err = r.newGerritClient(); err != nil {
		return r.done(errors.Annotate(err, "failed to init Gerrit client").Err())
	}

	// Create the history file.
	if r.w, err = history.CreateFile(r.out); err != nil {
		return r.done(errors.Annotate(err, "failed to create the output file").Err())
	}
	defer r.w.Close()

	fmt.Printf("starting BigQuery queries...\n")

	var rejections int
	err = r.rejections(ctx, func(frag *evalpb.RejectionFragment) error {
		if frag.Terminal {
			rejections++
		}
		return r.write(&evalpb.Record{
			Data: &evalpb.Record_RejectionFragment{RejectionFragment: frag},
		})
	})
	if err != nil {
		return r.done(err)
	}

	r.mu.Lock()
	fmt.Printf("total: %d rejections, %d records\n", rejections, r.recordsWrote)
	r.mu.Unlock()

	return r.done(r.w.Close())
}

// write write rec to the output file.
// Occasionally prints out progress.
func (r *presubmitHistoryRun) write(rec *evalpb.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.w.Write(rec); err != nil {
		return err
	}

	// Occasionally print progress.
	r.recordsWrote++
	now := time.Now()
	if r.recordCountNextReport.Before(now) {
		if !r.recordCountNextReport.IsZero() {
			fmt.Printf("wrote %d records\n", r.recordsWrote)
		}
		r.recordCountNextReport = now.Add(time.Second)
	}
	return nil
}

func (r *presubmitHistoryRun) bqQuery(ctx context.Context, sql string) (*bigquery.Query, error) {
	client, err := newBQClient(ctx, r.authenticator)
	if err != nil {
		return nil, err
	}

	prepRe := func(rgx string) string {
		if rgx == "" || rgx == ".*" {
			return ""
		}
		return fmt.Sprintf("^(%s)$", rgx)
	}

	q := client.Query(sql)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "startTime", Value: r.startTime},
		{Name: "endTime", Value: r.endTime},
		{Name: "builder_regexp", Value: prepRe(r.builderRegex)},
		{Name: "test_id_regexp", Value: prepRe(r.testIDRegex)},
	}
	return q, nil
}

var errPatchsetDeleted = errors.New("patchset deleted")

// populateChangedFiles populates ps.ChangedFiles.
// TODO(nodir): delete this function, gerrit.go and cache.go in February 2021,
// when we have enough BigQuery data with gerrit info.
func (r *presubmitHistoryRun) populateChangedFiles(ctx context.Context, ps *evalpb.GerritPatchset) error {
	changedFiles, err := r.gerrit.ChangedFiles(ctx, ps)
	switch {
	case err != nil:
		return err
	case len(changedFiles) == 0:
		return errPatchsetDeleted
	}

	repo := fmt.Sprintf("https://%s/%s", ps.Change.Host, strings.TrimSuffix(ps.Change.Project, ".git"))

	ps.ChangedFiles = make([]*evalpb.SourceFile, len(changedFiles))
	for i, path := range changedFiles {
		ps.ChangedFiles[i] = &evalpb.SourceFile{
			Repo: repo,
			Path: "//" + path,
		}
	}
	return nil
}

// newGerritClient creates a new gitiles client. Does not mutate r.
func (r *presubmitHistoryRun) newGerritClient() (*gerritClient, error) {
	transport, err := r.authenticator.Transport()
	if err != nil {
		return nil, err
	}

	ucd, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{Transport: transport}
	return &gerritClient{
		listFilesRPC: func(ctx context.Context, host string, req *gerritpb.ListFilesRequest) (*gerritpb.ListFilesResponse, error) {
			client, err := gerrit.NewRESTClient(httpClient, host, true)
			if err != nil {
				return nil, errors.Annotate(err, "failed to create a Gerrit client").Err()
			}
			return client.ListFiles(ctx, req)
		},
		fileListCache: cache{
			dir:       filepath.Join(ucd, "chrome-rts", "gerrit-changed-files"),
			memory:    lru.New(1024),
			valueType: reflect.TypeOf(changedFiles{}),
		},
		limiter: rate.NewLimiter(10, 1),
	}, nil
}
