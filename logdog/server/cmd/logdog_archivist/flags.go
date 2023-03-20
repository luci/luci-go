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

package main

import (
	"flag"
	"time"

	"go.chromium.org/luci/common/errors"
)

// CommandLineFlags contains archivist service configuration.
//
// It is exposed via CLI flags.
type CommandLineFlags struct {
	// StagingBucket is the name of the Google Storage bucket to use for staging
	// logs.
	//
	// Required.
	StagingBucket string

	// MaxConcurrentTasks is the maximum number of archival tasks to process
	// concurrently or 0 for unlimited.
	//
	// Default is 1.
	MaxConcurrentTasks int

	// LeaseBatchSize is the number of archival tasks to lease from the taskqueue
	// per one cycle.
	//
	// TaskQueue has a limit of 10qps for leasing tasks, so the batch size must
	// be set to:
	// LeaseBatchSize * 10 > (max expected stream creation QPS)
	//
	// In 2020, max stream QPS is approximately 1000 QPS.
	//
	// Default is 500.
	LeaseBatchSize int

	// LeaseTime is the amount of time to to lease the batch of tasks for.
	//
	// We need:
	// (time to process avg stream) * LeaseBatchSize < LeaseTime
	//
	// As of 2020, 90th percentile process time per stream is ~5s, 95th
	// percentile of the loop time is 25m.
	//
	//
	// Default is 40 min.
	LeaseTime time.Duration

	// Entries below specify defaults for archive indexes generation. Individual
	// projects may override these default values in their project configs.

	// ArchiveIndexStreamRange, if not zero, is the maximum number of stream
	// indexes between index entries.
	//
	// Default is 0.
	ArchiveIndexStreamRange int

	// ArchiveIndexPrefixRange, if not zero, is the maximum number of prefix
	// indexes between index entries.
	//
	// Default is 0.
	ArchiveIndexPrefixRange int

	// ArchiveIndexByteRange, if not zero, is the maximum number of log data bytes
	// between index entries.
	//
	// Default is 0.
	ArchiveIndexByteRange int

	// CloudLoggingExportBufferLimit, is the maximum number of bytes that
	// the CloudLogger will keep in memory per concurrent-task before flushing
	// them out.
	//
	// Default is 10MiB.
	// Must be > 0.
	CloudLoggingExportBufferLimit int
}

// DefaultCommandLineFlags returns CommandLineFlags with populated defaults.
func DefaultCommandLineFlags() CommandLineFlags {
	return CommandLineFlags{
		MaxConcurrentTasks:            1,
		LeaseBatchSize:                500,
		LeaseTime:                     40 * time.Minute,
		CloudLoggingExportBufferLimit: 10 * 1024 * 1024,
	}
}

// Register registers flags in the flag set.
func (f *CommandLineFlags) Register(fs *flag.FlagSet) {
	fs.StringVar(&f.StagingBucket, "staging-bucket", f.StagingBucket,
		"GCE bucket name to use for staging logs.")
	fs.IntVar(&f.MaxConcurrentTasks, "max-concurrent-tasks", f.MaxConcurrentTasks,
		"Maximum number of archival tasks to process concurrently or 0 for unlimited.")
	fs.IntVar(&f.LeaseBatchSize, "lease-batch-size", f.LeaseBatchSize,
		"How many tasks to lease per cycle.")
	fs.DurationVar(&f.LeaseTime, "lease-time", f.LeaseTime,
		"For how long to lease archival tasks.")
	fs.IntVar(&f.ArchiveIndexStreamRange, "archive-index-stream-range", f.ArchiveIndexStreamRange,
		"The maximum number of stream indexes between index entries.")
	fs.IntVar(&f.ArchiveIndexPrefixRange, "archive-index-prefix-range", f.ArchiveIndexPrefixRange,
		"The maximum number of prefix indexes between index entries.")
	fs.IntVar(&f.ArchiveIndexByteRange, "archive-index-byte-range", f.ArchiveIndexByteRange,
		"The maximum number of log data bytes between index entries.")
	fs.IntVar(&f.CloudLoggingExportBufferLimit, "cloud-logging-export-buffer-limit", f.CloudLoggingExportBufferLimit,
		"Maximum number of bytes that the Cloud Logger will keep in memory before flushing out.")
}

// Validate returns an error if some parsed flags have invalid values.
func (f *CommandLineFlags) Validate() error {
	if f.StagingBucket == "" {
		return errors.New("-staging-bucket is required")
	}
	if f.CloudLoggingExportBufferLimit == 0 {
		return errors.New("-cloud-logging-export-buffer-limit must be > 0")
	}
	return nil
}
