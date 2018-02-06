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

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/proto/google/descutil"
	"go.chromium.org/luci/hardcoded/chromeinfra"
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
	switch _, err := ts.getTableMetadata(ctx, td.DataSetID, td.TableID); {
	case isNotFound(err):
		md := &bigquery.TableMetadata{
			Name:        td.FriendlyName,
			Description: td.Description,
			Schema:      td.Schema,
		}
		if !td.PartitioningDisabled {
			md.TimePartitioning = &bigquery.TimePartitioning{
				Expiration: td.PartitioningExpiration,
			}
		}
		return ts.createTable(ctx, td.DataSetID, td.TableID, md)
	case err != nil:
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
	importPaths stringlistflag.Flag
}

func parseFlags() (*flags, error) {
	var f flags
	flag.BoolVar(&f.dryRun, "dry-run", false, "Only performs non-mutating operations; logs what would happen otherwise")

	table := flag.String("table", "", `Table name with format "<project id>.<dataset id>.<table id>"`)
	flag.StringVar(&f.FriendlyName, "friendly-name", "", "Friendly name for the table")
	flag.BoolVar(&f.PartitioningDisabled, "disable-partitioning", false, "Makes the table not time-partitioned")
	flag.DurationVar(&f.PartitioningExpiration, "partition-expiration", 0, "Expiration for partitions. 0 for no expiration.")
	flag.StringVar(&f.protoDir, "message-dir", ".", "path to directory with the .proto file that defines the schema message")
	// -I matches protoc's flag and its error message suggesting to pass -I.
	flag.Var(&f.importPaths, "I", "path to directory with the imported .proto file; can be specified multiple times")

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

	desc, err := loadProtoDescription(flags.protoDir, flags.importPaths)
	if err != nil {
		return errors.Annotate(err, "failed to load proto descriptor").Err()
	}
	td.Schema, td.Description, err = schemaFromMessage(desc, flags.messageName)
	if err != nil {
		return errors.Annotate(err, "could not derive schema from message %q at path %q", flags.messageName, flags.protoDir).Err()
	}

	// Create an Authenticator and use it for BigQuery operations.
	authOpts := chromeinfra.DefaultAuthOptions()
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

func protoImportPaths(dir string, userDefinedImportPaths []string) ([]string, error) {
	// In Go mode, import paths are all $GOPATH/src directories because we like
	// go-style absolute import paths,
	// e.g. "go.chromium.org/luci/logdog/api/logpb/log.proto"
	var goSources []string
	inGopath := false
	for _, p := range goPaths() {
		src := filepath.Join(p, "src")
		switch info, err := os.Stat(src); {
		case os.IsNotExist(err):

		case err != nil:
			return nil, err

		case !info.IsDir():

		default:
			goSources = append(goSources, src)
			// note: does not respect case insensitive file systems (e.g. on windows)
			inGopath = inGopath || strings.HasPrefix(dir, src)
		}
	}

	switch {
	case !inGopath:
		// Python mode.

		// loadProtoDescription passes absolute paths to .proto files,
		// so unless we pass -I with a directory containing them,
		// protoc will complain. Do that for the user.
		return append([]string{dir}, userDefinedImportPaths...), nil

	case len(userDefinedImportPaths) > 0:
		return nil, fmt.Errorf(
			"%q is in $GOPATH. "+
				"Please do not use -I flag. "+
				"Use go-style absolute paths to imported .proto files, "+
				"e.g. github.com/user/repo/path/to/file.proto", dir)
	default:
		return goSources, nil
	}
}

// loadProtoDescription compiles .proto files in the dir
// and returns their descriptor.
func loadProtoDescription(dir string, importPaths []string) (*descriptor.FileDescriptorSet, error) {
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

	importPaths, err = protoImportPaths(dir, importPaths)
	if err != nil {
		return nil, err
	}

	args := []string{
		"--descriptor_set_out=" + descFile,
		"--include_imports",
		"--include_source_info",
	}
	for _, p := range importPaths {
		args = append(args, "-I="+p)
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

func goPaths() []string {
	gopath := strings.TrimSpace(os.Getenv("GOPATH"))
	if gopath == "" {
		return nil
	}
	return strings.Split(gopath, string(filepath.ListSeparator))
}
