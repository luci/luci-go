// Copyright 2017 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	"golang.org/x/net/context"
	"google.golang.org/api/option"

	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/google/descutil"

	"infra/libs/infraenv"
)

type tableDef struct {
	ProjectID              string
	DataSetID              string
	TableID                string
	FriendlyName           string
	Description            string
	PartitioningDisabled   bool
	PartitioningExpiration time.Duration
	Schema                 bigquery.Schema
}

func updateFromTableDef(ctx context.Context, ts tableStore, td tableDef) error {
	_, err := ts.getTableMetadata(ctx, td.DataSetID, td.TableID)
	if isNotFound(err) {
		var options []bigquery.CreateTableOption
		if !td.PartitioningDisabled {
			options = append(options, bigquery.TimePartitioning{Expiration: td.PartitioningExpiration})
		}
		if err = ts.createTable(ctx, td.DataSetID, td.TableID, options...); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return ts.updateTable(ctx, td.DataSetID, td.TableID, bigquery.TableMetadataToUpdate{
		Name:        td.FriendlyName,
		Description: td.Description,
		Schema:      td.Schema,
	})
}

type flags struct {
	tableDef
	protoDir    string
	messageName string
	dryRun      bool
}

func parseFlags() (*flags, error) {
	var f flags
	flag.BoolVar(&f.dryRun, "dry-run", false, "Only performs non-mutating operations; logs what would happen otherwise")

	table := flag.String("table", "", `Table name with format "<project id>.<dataset id>.<table id>"`)
	flag.StringVar(&f.FriendlyName, "friendly-name", "", "Friendly name for the table")
	flag.BoolVar(&f.PartitioningDisabled, "disable-partitioning", false, "Makes the table not time-partitioned")
	flag.DurationVar(&f.PartitioningExpiration, "partition-expiration", 0, "Expiration for partitions. 0 for no expiration.")
	flag.StringVar(&f.protoDir, "proto-dir", ".", "path to directory with the .proto file")
	flag.StringVar(&f.messageName,
		"message",
		"",
		"Full name of the protobuf message that defines the table schema. The name must contain proto package name.")

	flag.Parse()

	switch {
	case len(flag.Args()) > 0:
		return nil, fmt.Errorf("unexpected arguments: %q", flag.Args())
	case *table == "":
		return nil, fmt.Errorf("-table is required")
	case f.messageName == "":
		return nil, fmt.Errorf("-message is required")
	}
	if parts := strings.Split(*table, "."); len(parts) == 3 {
		f.ProjectID = parts[0]
		f.DataSetID = parts[1]
		f.TableID = parts[2]
	} else {
		return nil, fmt.Errorf("expected exactly 2 dots in table name %q", *table)
	}

	return &f, nil
}

func run(ctx context.Context) error {
	flags, err := parseFlags()
	if err != nil {
		return errors.Annotate(err, "failed to parse flags").Err()
	}

	td := flags.tableDef

	desc, err := loadProtoDescription(flags.protoDir)
	if err != nil {
		return errors.Annotate(err, "failed to load proto descriptor").Err()
	}
	td.Schema, td.Description, err = schemaFromMessage(desc, flags.messageName)
	if err != nil {
		return errors.Annotate(err, "could not derive schema from message %q at path %q", flags.messageName, flags.protoDir).Err()
	}

	// Create an Authenticator and use it for BigQuery operations.
	authOpts := infraenv.DefaultAuthOptions()
	authOpts.Scopes = []string{bigquery.Scope}
	authenticator := auth.NewAuthenticator(ctx, auth.InteractiveLogin, authOpts)

	authTS, err := authenticator.TokenSource()
	if err != nil {
		return errors.Annotate(err, "could not get authentication credentials").Err()
	}

	c, err := bigquery.NewClient(ctx, td.ProjectID, option.WithTokenSource(authTS))
	if err != nil {
		return errors.Annotate(err, "could not create BigQuery client").Err()
	}
	var ts tableStore = bqTableStore{c}
	if flags.dryRun {
		ts = dryRunTableStore{ts: ts, w: os.Stdout}
	}

	log.Printf("Updating table `%s.%s.%s`...", td.ProjectID, td.DataSetID, td.TableID)
	if err = updateFromTableDef(ctx, ts, td); err != nil {
		return errors.Annotate(err, "failed to update table").Err()
	}
	log.Println("Finished updating table.")
	return nil
}

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

// schemaFromMessage loads a message by name from .proto files in dir
// and converts the message to a bigquery schema.
func schemaFromMessage(desc *descriptor.FileDescriptorSet, messageName string) (schema bigquery.Schema, description string, err error) {
	conv := schemaConverter{
		desc:           desc,
		sourceCodeInfo: make(map[*descriptor.FileDescriptorProto]sourceCodeInfoMap, len(desc.File)),
	}
	for _, f := range desc.File {
		conv.sourceCodeInfo[f], err = descutil.IndexSourceCodeInfo(f)
		if err != nil {
			return nil, "", errors.Annotate(err, "failed to index source code info in file %q", f.GetName()).Err()
		}
	}
	return conv.schema(messageName)
}

// loadProtoDescription compiles .proto files in the dir
// and returns their descriptor.
func loadProtoDescription(dir string) (*descriptor.FileDescriptorSet, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, errors.Annotate(err, "could make path %q absolute", dir).Err()
	}

	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)
	descFile := filepath.Join(tempDir, "desc")
	args := []string{
		"--descriptor_set_out=" + descFile,
		"--include_imports",
		"--include_source_info",
	}

	// Include all $GOPATH/src directories because we like
	// go-style absolute import paths,
	// e.g. "go.chromium.org/luci/logdog/api/logpb/log.proto"
	for _, p := range strings.Split(os.Getenv("GOPATH"), string(filepath.ListSeparator)) {
		src := filepath.Join(p, "src")
		switch info, err := os.Stat(src); {
		case os.IsNotExist(err):
			continue
		case err != nil:
			return nil, err
		case !info.IsDir():
			continue
		}
		args = append(args, "--proto_path="+src)
	}

	protoFiles, err := filepath.Glob(filepath.Join(dir, "*.proto"))
	if err != nil {
		return nil, err
	}
	if len(protoFiles) == 0 {
		return nil, fmt.Errorf("no .proto files found in directory %q", dir)
	}
	args = append(args, protoFiles...)

	protoc := exec.Command("protoc", args...)
	protoc.Stderr = os.Stderr
	if err := protoc.Run(); err != nil {
		return nil, errors.Annotate(err, "protoc run failed").Err()
	}

	descBytes, err := ioutil.ReadFile(descFile)
	if err != nil {
		return nil, err
	}
	var desc descriptor.FileDescriptorSet
	err = proto.Unmarshal(descBytes, &desc)
	return &desc, err
}
