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
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/proto/google/descutil"
	"go.chromium.org/luci/common/proto/protoc"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

var (
	errCanceledByUser = errors.New("operation canceled by user")
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
	PartitioningType       string
	Schema                 bigquery.Schema
	ClusteringFields       []string
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
		fmt.Println(bq.SchemaString(td.Schema))
		fmt.Println(strings.Repeat("=", 80))
		if !shouldContinue() {
			return errCanceledByUser
		}

		md = &bigquery.TableMetadata{
			Name:        td.FriendlyName,
			Description: td.Description,
			Schema:      td.Schema,
		}
		if !td.PartitioningDisabled {
			md.TimePartitioning = &bigquery.TimePartitioning{
				Expiration: td.PartitioningExpiration,
				Field:      td.PartitioningField,
				Type:       bigquery.TimePartitioningType(td.PartitioningType),
			}
		}
		if len(td.ClusteringFields) > 0 {
			md.Clustering = &bigquery.Clustering{Fields: td.ClusteringFields}
		}
		if err = ts.createTable(ctx, td.DataSetID, td.TableID, md); err != nil {
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
		bq.AddMissingFields(&td.Schema, md.Schema)

		if diff := bq.SchemaDiff(md.Schema, td.Schema); diff == "" {
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
	verbose     bool
	importPaths stringlistflag.Flag
	noGoMode    bool
	goModules   stringlistflag.Flag
}

func parseFlags() (*flags, error) {
	var f flags
	table := flag.String("table", "", `Table name with format "<project id>.<dataset id>.<table id>". Use "-" to dump the schema to stdout only.`)
	flag.StringVar(&f.FriendlyName, "friendly-name", "", "Friendly name for the table.")
	flag.StringVar(&f.PartitioningField, "partitioning-field", "", "Name of a timestamp field to use for table partitioning (beta).")
	// See: https://pkg.go.dev/cloud.google.com/go/bigquery#TimePartitioning
	flag.StringVar(&f.PartitioningType, "partitioning-type", "DAY", "One of HOUR, DAY, MONTH and YEAR.")
	flag.BoolVar(&f.PartitioningDisabled, "disable-partitioning", false, "Makes the table not time-partitioned.")
	flag.DurationVar(&f.PartitioningExpiration, "partitioning-expiration", 0, "Expiration for partitions. 0 for no expiration.")
	flag.Var(luciflag.StringSlice(&f.ClusteringFields), "clustering-field", "Optional, one or more clustering fields. Can be specified multiple times and order is significant.")
	flag.StringVar(&f.protoDir, "message-dir", ".", "Path to directory with the .proto file that defines the schema message.")
	flag.BoolVar(&f.noGoMode, "no-go-mode", false, "Don't try to recognize active Go module based on cwd.")
	flag.Var(&f.goModules, "go-module", "Make protos in the given module available in proto import path. Can be specified multiple times.")
	flag.BoolVar(&f.force, "force", false, "Proceed without a user confirmation.")
	flag.BoolVar(&f.verbose, "verbose", false, "Print more information in the log.")
	// -I matches protoc's flag and its error message suggesting to pass -I.
	flag.Var(&f.importPaths, "I", "Path to directory with the imported .proto file; can be specified multiple times.")

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
	case f.noGoMode && len(f.goModules) > 0:
		return nil, fmt.Errorf("-no-go-mode and -go-module flags are not compatible")
	}
	if parts := strings.Split(*table, "."); len(parts) == 3 {
		f.ProjectID = parts[0]
		f.DataSetID = parts[1]
		f.TableID = parts[2]
	} else if *table == "-" {
		f.TableID = "-"
	} else {
		return nil, fmt.Errorf("table name %q must be '-' or have exactly 2 dots", *table)
	}

	return &f, nil
}

func run(ctx context.Context) error {
	flags, err := parseFlags()
	if err != nil {
		return errors.Annotate(err, "failed to parse flags").Err()
	}

	if flags.verbose {
		ctx = logging.SetLevel(ctx, logging.Debug)
	} else {
		ctx = logging.SetLevel(ctx, logging.Error)
	}

	td := flags.tableDef

	desc, err := loadProtoDescription(ctx, flags.protoDir, !flags.noGoMode, flags.goModules, flags.importPaths)
	if err != nil {
		return errors.Annotate(err, "failed to load proto descriptor").Err()
	}
	td.Schema, td.Description, err = schemaFromMessage(desc, flags.messageName)
	if err != nil {
		return errors.Annotate(err, "could not derive schema from message %q at path %q", flags.messageName, flags.protoDir).Err()
	}
	if td.TableID == "-" {
		return dumpSchemaJSON(&td.Schema)
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
	ctx := gologger.StdConfig.Use(context.Background())
	switch err := run(ctx); {
	case errors.Is(err, errCanceledByUser):
		os.Exit(1)
	case err != nil:
		log.Fatal(err)
	}
}

// schemaFromMessage loads a message by name from .proto files in dir
// and converts the message to a bigquery schema.
func schemaFromMessage(desc *descriptorpb.FileDescriptorSet, messageName string) (schema bigquery.Schema, description string, err error) {
	conv := bq.SchemaConverter{
		Desc:           desc,
		SourceCodeInfo: make(map[*descriptorpb.FileDescriptorProto]bq.SourceCodeInfoMap, len(desc.File)),
	}
	for _, f := range desc.File {
		conv.SourceCodeInfo[f], err = descutil.IndexSourceCodeInfo(f)
		if err != nil {
			return nil, "", errors.Annotate(err, "failed to index source code info in file %q", f.GetName()).Err()
		}
	}
	return conv.Schema(messageName)
}

// checkGoMode returns true if `go` executable is in PATH and `dir` is in
// a Go module.
//
// Note that GOPATH mode is not supported. Returns an error if it sees GOPATH
// env var.
func checkGoMode(dir string) (bool, error) {
	cmd := exec.Command("go", "list", "-m")
	cmd.Dir = dir
	buf, err := cmd.CombinedOutput()
	if err == nil {
		// When `dir` is not a Go package, `go -list -m` returns
		// "command-line-arguments". See https://github.com/golang/go/issues/36793.
		return strings.TrimSpace(string(buf)) != "command-line-arguments", nil
	}
	if os.Getenv("GO111MODULE") != "off" && os.Getenv("GOPATH") != "" {
		return false, errors.Reason("GOPATH mode is not supported").Err()
	}
	return false, nil
}

// prepInputs prepares inputs for protoc depending on Go vs non-Go mode.
func prepInputs(ctx context.Context, dir string, allowGoMode bool, goModules, importPaths []string) (*protoc.StagedInputs, error) {
	useGo := allowGoMode && len(goModules) > 0
	if !useGo && allowGoMode {
		var err error
		if useGo, err = checkGoMode(dir); err != nil {
			return nil, err
		}
	}
	if useGo {
		logging.Infof(ctx, "Running in Go mode: importing *.proto from Go source tree")
		return protoc.StageGoInputs(ctx, dir, goModules, nil, importPaths)
	}
	logging.Infof(ctx, "Running in generic mode: importing *.proto from explicitly given paths only")
	return protoc.StageGenericInputs(ctx, dir, importPaths)
}

// loadProtoDescription compiles .proto files in the dir
// and returns their descriptor.
func loadProtoDescription(ctx context.Context, dir string, allowGoMode bool, goModules, importPaths []string) (*descriptorpb.FileDescriptorSet, error) {
	// Stage all requested Go modules under a single root.
	inputs, err := prepInputs(ctx, dir, allowGoMode, goModules, importPaths)
	if err != nil {
		return nil, err
	}
	defer inputs.Cleanup()

	// Prep the temp directory for the resulting descriptor file.
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)
	descFile := filepath.Join(tempDir, "desc")

	// Compile protos to get the descriptor.
	err = protoc.Compile(ctx, &protoc.CompileParams{
		Inputs:              inputs,
		OutputDescriptorSet: descFile,
	})
	if err != nil {
		return nil, err
	}

	// Read the resulting descriptor.
	descBytes, err := os.ReadFile(descFile)
	if err != nil {
		return nil, err
	}
	var desc descriptorpb.FileDescriptorSet
	err = proto.Unmarshal(descBytes, &desc)
	return &desc, err
}

// confirm asks for a user confirmation for an action, with No as default.
// Only "y" or "Y" responses is treated as yes.
func confirm(action string) (response bool) {
	fmt.Printf("%s? [y/N] ", action)
	var res string
	fmt.Scanln(&res)
	return res == "y" || res == "Y"
}

// dumpSchema dumps the table schema to stdout in JSON.
func dumpSchemaJSON(s *bigquery.Schema) error {
	b, err := s.ToJSONFields()
	if err != nil {
		return errors.Annotate(err, "failed to dump table schema to JSON").Err()
	}
	fmt.Println(string(b))
	return nil
}
