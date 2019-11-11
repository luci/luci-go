package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

type graphFile struct {
	path   string
	commit string
	graph
}

func (f *graphFile) Load() error {
	// TODO(nodir): implement.
	return errors.Reason("unimplemented").Err()
}

func (f *graphFile) Save() error {
	// TODO(nodir): implement.
	return errors.Reason("unimplemented").Err()

}

// loadGraph loads a filegraph from a git repository.
func loadGraph(ctx context.Context, repoDir string) (*graph, error) {
	gitDir, err := gitDirPath(repoDir)
	if err != nil {
		return nil, err
	}

	file := graphFile{
		path: filepath.Join(gitDir, "filegraph.v0"),
	}

	start := time.Now()
	switch err := file.Load(); {
	case os.IsNotExist(err):
		logging.Infof(ctx, "populating cache; this may take minutes...")
	case err != nil:
		logging.Warningf(ctx, "cache is corrupted; populating cache; this may take minutes...")
	default:
		logging.Infof(ctx, "loaded cache in %s", time.Since(start))
	}

	processed := 0
	dirty := false
	err = readCommits(ctx, repoDir, file.commit, func(c commit) error {
		if err := file.AddCommit(c); err != nil {
			return err
		}
		dirty = true
		file.commit = c.Hash

		processed++
		if processed%10000 == 0 {
			if err := file.Save(); err != nil {
				logging.Errorf(ctx, "failed to save cache: %s", err)
			}
			fmt.Printf("processed %d commits\n", processed)
			dirty = false
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if dirty {
		if err := file.Save(); err != nil {
			logging.Errorf(ctx, "failed to save cache: %s", err)
		}
	}

	return &file.graph, nil
}
