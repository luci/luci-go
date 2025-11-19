# Gemini Code-Assist Context for the LUCI Go Repository

This document provides essential context for AI models interacting with the
`luci-go` codebase.

## 1. Project Overview

The `luci-go` repository contains the Go source code for the Layered Universal
Continuous Integration (LUCI) system.

## 2. Key Technologies and Libraries

- **Go:** The primary programming language. The specific version is defined in
  the `go.mod` file.
- **Protocol Buffers (Protobuf):** Used for defining data structures and service
  interfaces.
- **pRPC and gRPC:** The primary RPC framework for communication between
  services.
- **Google Cloud Platform (GCP):** The primary deployment target, with extensive
  use of services like Cloud Spanner, Cloud Tasks, Cloud Storage, BigQuery,
  Cloud Pub/Sub, Cloud Scheduler. While Cloud Run is the preferred runtime for
  new services, most still run on AppEngine or Google Kubernetes Engine.
- **Depot Tools:** The primary toolchain for managing the repository, including
  code reviews and presubmit checks.
- **Golangci-lint:** Used for linting Go code. Configuration is in the
  `.golangci.yaml` files located in each service's directory.
- **Staticcheck:** Used for static analysis of Go code. Configuration is in the
  `staticcheck.conf` file.

## 3. Project Structure and Conventions

### Key Directories

The repository is organized into a series of top-level directories, each
corresponding to a specific LUCI service or component. For example, the
`buildbucket` directory contains the source code for the Buildbucket service,
and the `auth` directory contains the authentication libraries.

There are also folders which contain key shared code, notably:

- `common/`: Contains common libraries and utilities shared across multiple
  services.
- `server/`: Contains common LUCI server framework code. This framework takes
  care of many common concerns that would not be handled by net/http, like:
  SIGTERM handling/graceful draining, request deadlines, panic catching, load
  shedding, monitoring, health-checking, authorisation of Cloud Scheduler/Tasks
  pushes.

The main UI for LUCI is served by MILO which can be found in `milo/` directory.
The frontend code lives in `milo/ui/`; see `milo/ui/GEMINI.md` for details
navigating this part of the repo.

### Code Conventions

#### Formatting
Go code is formatted using `gofmt`.

### Linting
Code is linted using `golangci-lint` and `staticcheck`.

### Testing
Tests are written using the standard Go testing framework. Test
files are named with the `_test.go` suffix. Tests use the the
`go.chromium.org/luci/common/testing/ftt` library (see
`common/testing/ftt/doc.go`) for nesting test cases and the `assert` library
from `go.chromium.org/luci/common/testing/truth/assert` for assertions.

See `resultdb/internal/services/recorder/create_root_invocation_test.go` for
an example how these libraries are typically used.

### API Surface

When modifying the API surface, the following style guide applies:

- **API Surface/Protos:** We follow the Google AIPs (API Improvement Proposals)
  available at https://google.aip.dev/{NUMBER}. This includes:

  - AIP-121: Resource-oriented design
  - AIP-122: Resource names
  - AIP-131: Standard methods: Get
  - AIP-132: Standard methods: List (an implementation of the AIP-132 order_by
    clause is available at `common/data/aip132/`)
  - AIP-133: Standard methods: Create
  - AIP-134: Standard methods: Update
  - AIP-135: Standard methods: Delete
  - AIP-136: Custom methods
  - AIP-140: Field names
  - AIP-154: Resource freshness validation (etags for race-free updates)
  - AIP-155: Request identification (for retry-safe Create/Update RPCs)
  - AIP-157: Partial responses (for views/field masks)
  - AIP-158: Pagination
  - AIP-160: Filtering (an implementation of an AIP-160 filter-parser in go is
    available at `common/data/aip160/`).
  - AIP-210: Unicode
  - AIP-211: Authorization checks
  - AIP-231: Batch methods: Get
  - AIP-233: Batch methods: Create

- **Request validation:** ALL fields (except OUTPUT_ONLY fields - see AIP-203)
  in the request should be validated by the server. Validation should comprise
  at MINIMUM:

  - Verifying a value is set, if it must be set.
  - If the field is of a type that allows variable-length data (e.g. a string,
    bytes or a repeated type), the field is within an allowed length limit (if
    no length limit exists, one should be created and documented on the proto).
    In addition to the per-field length limit, a length limit may sometimes also
    be applied at a higher-level, such as on the total size of a test result or
    a set of properties. This can be enforced by setting a limit on the size as
    measured by proto.Size().
  - If the value is a string, that it comes from a known alphabet. At minimum,
    strings should be valid UTF-8 and be in Unicode Normal Form C. In most cases
    they should also be limited to printable Unicode characters only. See
    AIP-210 - Unicode. For identifiers, it is common to restrict to a narrow set
    of inputs such as `[a-z][a-zA-Z0-9]*`, which takes care of most of the
    checks above.
  - It is generally preferable that fields are validated in the order that they
    are specified in the proto, to make it easy to verify that all fields have
    in fact been validated.

- **Errors construction:**

  - Validation errors for an invalid request input should be annotated with the
    path of the problematic input. For example,
    `books: [0]: my_description: non-printable character at byte index 2` for a
    validation issue with the field books[0].my_description. Typically this is
    achieved by a hierarchy of validation methods which annotate one part of the
    path each, e.g. ValidateRequest(request) calls ValidateBooks(request.books)
    and annotates any errors with `errors.Fmt("books: %w", err)`, ValidateBooks
    calls ValidateBook(books[0]) and annotates its error (if any) with
    `errors.Fmt("[0]: %w")`, and so on.

  - For monitoring purposes, API usability, and to ensure clients can correctly
    implement retry logic, it is important that all errors that are not internal
    server errors (codes.Internal) are annotated with the correct status code.
    This allows separating client from server errors, and retriable from
    non-retriable (permanent) errors. Services can achieve this by ensuring
    errors that are not internal server errors are annotated with the
    appropriate code using the appstatus package:

    For example (using `go.chromium.org/luci/grpc/appstatus`):
    `appstatus.Errorf(codes.NotFound, "book %q not found", bookName)`

- **Tests:** All RPCs exposed by a service must have associated tests to ensure
  required access controls are in place, invalid inputs are rejected, and both
  normal and non-normal situations are correctly handled.

  - At minimum the following tests should exist:

    - For security, a test to validate the service enforces authorisation checks
      (e.g. denies access to callers without the required access). These should
      also ensure the returned authorisation errors are understandable (i.e.
      describes the missing permissions or group membership) and have the
      correct status code (codes.PermissionDenied).
    - Tests to validate the service validates the request input, including each
      field (that is not OUTPUT_ONLY). These should ensure returned errors have
      the correct field path (see above), an understandable human-readable error
      and are annotated with codes.InvalidArgument.
    - Tests to validate each non-normal path (e.g. request valid but entity does
      not exist). Each such test should again ensure the presence of the correct
      status code.
    - Tests to validate the happy path.

  - When validating a response that is expected to error, both the error message
    and code should be checked.

    If the appstatus error has already been converted to a grpc code in the
    service postlude using `appstatus.GRPCifyAndLog(ctx, err)`, the code can be
    asserted using:

    `assert.That(t, err, grpccode.ShouldBe(codes.NotFound))`

    Where grpccode is `go.chromium.org/luci/grpc/grpcutil/testing/grpccode`.

    Alternatively for standalone methods that contribute to an RPC response,
    codes can be asserted using an assertion like:
    `assert.That(t, appstatus.Code(err), should.Equal(codes.NotFound))`

  - Test cases should try and keep cause and effect clear by factoring out
    common/repeated test setup. The ideal test case changes only the properties
    the test is trying to exercise and asserts the expected response.

  - A contemperanous (but not perfect) example of how the above can be achieved
    is: `resultdb/internal/services/recorder/create_root_invocation_test.go`.

## 4. Build and Test

### Build

The project is built using the standard Go toolchain.

As builds of the entire repository can take some time, it is best to only
build the directory you need, e.g. if working on ResultDB:

`go build go.chromium.org/luci/resultdb/...`

### Protocol buffers bindings

After changing protos, regenerate proto buffer go bindings by running
`go generate` against the directory, for example:

`go generate go.chromium.org/luci/resultdb/proto/v1`

### Test

Tests are run using the `go test` command. As most tests are integration
tests, you must set the `INTEGRATION_TESTS=1` environment variable to run them.

For example, to run all tests under resultdb/, run the following command:
`INTEGRATION_TESTS=1 go test go.chromium.org/luci/resultdb/...`

Running all tests in `go.chromium.org/luci/...` takes a long time, so it is best
to run only the directory you need.

## 5. Contribution and Code Review

Changes from the current git branch are uploaded for review using the
`git cl upload` command (`git cl` is provided by the `depot_tools` repository).

Git commit messages follow a format like:

```
[ResultDB] Add validation for BatchCreateTestResults RPC.

A longer description appears here.

BUG=b:{BUG_NUMBER}
TEST=INTEGRATION_TESTS=1 go test go.chromium.org/luci/resultdb/...
```
