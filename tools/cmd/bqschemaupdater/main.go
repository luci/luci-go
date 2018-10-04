// Copyright 2018 The LUCI Authors.
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
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"

	"google.golang.org/api/option"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/proto/google/descutil"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

var (
	canceledByUser = errors.BoolTag{
		Key: errors.NewTagKey("operation canceled by user"),
	}
	errCanceledByUser = errors.Reason("operation canceled by user").Tag(canceledByUser).Err()
)

type tableDef struct {
	ProjectID              string
	DataSetID              string
	TableID                string
	FriendlyName           string
	Description            string
	PartitioningDisabled   bool
	PartitioningExpiration time.Duration
	PartitioningField      string
	Schema                 bigquery.Schema
}

func updateFromTableDef(ctx context.Context, force bool, ts tableStore, td tableDef) error {
	tableID := fmt.Sprintf("%s.%s.%s", td.ProjectID, td.DataSetID, td.TableID)
	shouldContinue := func() bool {
		if force {
			return true
		}
		return confirm("Continue")
	}

	md, err := ts.getTableMetadata(ctx, td.DataSetID, td.TableID)
	switch {
	case isNotFound(err): // new table
		fmt.Printf("Table %q does not exist.\n", tableID)
		fmt.Println("It will be created with the following schema:")
		fmt.Println(strings.Repeat("=", 80))
		fmt.Println(schemaString(td.Schema))
		fmt.Println(strings.Repeat("=", 80))
		if !shouldContinue() {
			return errCanceledByUser
		}

		md := &bigquery.TableMetadata{
			Name:        td.FriendlyName,
			Description: td.Description,
			Schema:      td.Schema,
		}
		if !td.PartitioningDisabled {
			md.TimePartitioning = &bigquery.TimePartitioning{
				Expiration: td.PartitioningExpiration,
				Field:      td.PartitioningField,
			}
		}
		err := ts.createTable(ctx, td.DataSetID, td.TableID, md)
		if err != nil {
			return err
		}
		fmt.Println("Table is created.")
		fmt.Println("Please update the documentation in https://chromium.googlesource.com/infra/infra/+/master/doc/bigquery_tables.md or the internal equivalent.")
		return nil

	case err != nil:
		return err

	default: // existing table
		fmt.Printf("Updating table %q\n", tableID)

		// add fields missing in td.Schema because BigQuery does not support
		// removing fields anyway.
		addMissingFields(&td.Schema, md.Schema)

		if diff := schemaDiff(md.Schema, td.Schema); diff == "" {
			fmt.Println("No changes to schema detected.")
		} else {
			fmt.Println("The following changes to the schema will be made:")
			fmt.Println(strings.Repeat("=", 80))
			fmt.Println(diff)
			fmt.Println(strings.Repeat("=", 80))
			if !shouldContinue() {
				return errCanceledByUser
			}
		}

		update := bigquery.TableMetadataToUpdate{
			Name:        td.FriendlyName,
			Description: td.Description,
			Schema:      td.Schema,
		}
		if err := ts.updateTable(ctx, td.DataSetID, td.TableID, update); err != nil {
			return err
		}
		fmt.Println("Finished updating the table.")
		return nil
	}
}

type flags struct {
	tableDef
	protoDir    string
	messageName string
	force       bool
	importPaths stringlistflag.Flag
}

func parseFlags() (*flags, error) {
	var f flags
	table := flag.String("table", "", `Table name with format "<project id>.<dataset id>.<table id>"`)
	flag.StringVar(&f.FriendlyName, "friendly-name", "", "Friendly name for the table.")
	flag.StringVar(&f.PartitioningField, "partitioning-field", "", "Name of a timestamp field to use for table partitioning (beta).")
	flag.BoolVar(&f.PartitioningDisabled, "disable-partitioning", false, "Makes the table not time-partitioned.")
	flag.DurationVar(&f.PartitioningExpiration, "partitioning-expiration", 0, "Expiration for partitions. 0 for no expiration.")
	flag.StringVar(&f.protoDir, "message-dir", ".", "path to directory with the .proto file that defines the schema message.")
	flag.BoolVar(&f.force, "force", false, "proceed without a user confirmation.")
	// -I matches protoc's flag and its error message suggesting to pass -I.
	flag.Var(&f.importPaths, "I", "path to directory with the imported .proto file; can be specified multiple times.")

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
		return nil, fmt.Errorf("-message is required (the name must contain the proto package name)")
	case f.PartitioningField != "" && f.PartitioningDisabled:
		return nil, fmt.Errorf("partitioning field cannot be non-empty with disabled partitioning")
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
	file, _, _ := descutil.Resolve(desc, flags.messageName)
	td.Description = fmt.Sprintf(
		"Proto: https://cs.chromium.org/%s\nTable Description:\n%s",
		url.PathEscape(fmt.Sprintf("%s file:%s", flags.messageName, file.GetName())),
		td.Description)

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
	return updateFromTableDef(ctx, flags.force, bqTableStore{c}, td)
}

func main() {
	switch err := run(context.Background()); {
	case canceledByUser.In(err):
		os.Exit(1)
	case err != nil:
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
	grpcProtoPath := ""
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

			grpcPath := filepath.Join(src, "go.chromium.org", "luci", "grpc", "proto")
			if info, err := os.Stat(grpcPath); err == nil && info.IsDir() {
				grpcProtoPath = grpcPath
			}
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
		importPaths := goSources

		// Include gRPC protos.
		if grpcProtoPath != "" {
			importPaths = append(importPaths, grpcProtoPath)
		}
		return importPaths, nil
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

// confirm asks for a user confirmation for an action, with No as default.
// Only "y" or "Y" responses is treated as yes.
func confirm(action string) (response bool) {
	fmt.Printf("%s? [y/N] ", action)
	var res string
	fmt.Scanln(&res)
	return res == "y" || res == "Y"
}
