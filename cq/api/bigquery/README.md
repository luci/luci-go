# CQ BigQuery Schema

This includes the schema for rows in the CQ attempts table, as well as related
scripts to create or update the schema.

## Creating tables

All tables belong to a dataset, which has a name and description. Datasets can
be created with the gcloud SDK `bq` command, which must be installed first.

Datasets can be created with command that look like:

```
bq --location=US mk --dataset --description "Attempts" commit-queue:attempts
```

Schemas can be updated with bqschemaupdater. You should make sure you run an
up-to-date bqschemaupdater:

```
go install go.chromium.org/luci/tools/cmd/bqschemaupdater
```

This tool takes a proto message and table and updates the schema based on the
compiled proto message. To compile the proto message and update the schema:

```
go generate
bqschemaupdater -message bigquery.Attempt -table commit-queue.attempts.completed
```
