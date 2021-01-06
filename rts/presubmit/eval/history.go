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

package eval

import (
	"bufio"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/common/sync/parallel"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

// readTestDurations reads test duration records from a directory.
func readTestDurations(ctx context.Context, dir string, dest chan<- *evalpb.TestDurationRecord) error {
	return readHistoryRecords(dir, func(entry []byte) error {
		td := &evalpb.TestDurationRecord{}
		if err := protojson.Unmarshal(entry, td); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
		case dest <- td:
		}
		return ctx.Err()
	})
}

// readHistoryRecords reads JSON values from .jsonl.gz files in the given
// directory.
func readHistoryRecords(dir string, callback func(entry []byte) error) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl.gz"))
	if err != nil {
		return err
	}

	return parallel.WorkPool(100, func(work chan<- func() error) {
		for _, fileName := range files {
			fileName := fileName
			work <- func() error {
				// Open the file.
				f, err := os.Open(fileName)
				if err != nil {
					return err
				}
				defer f.Close()

				// Decompress as GZIP.
				gz, err := gzip.NewReader(f)
				if err != nil {
					return err
				}
				defer gz.Close()

				// Split by line.
				scan := bufio.NewScanner(gz)
				scan.Buffer(nil, 1e7) // 10 MB.
				for scan.Scan() {
					if err := callback(scan.Bytes()); err != nil {
						return err
					}
				}
				return scan.Err()
			}
		}
	})
}
