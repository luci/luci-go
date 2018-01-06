bqschemaupdater is a tool for adding and updating BigQuery table schema.

# Usage

schemas should be written in .proto format.

To create or modify a new table, you need to be authenticated in the Google
Cloud Project project to which that table will belong. To check your
authentication status, ensure that you have the Cloud SDK
[installed](https://cloud.google.com/sdk/docs/quickstarts), then run:

```
gcloud info
```

If you don't see the correct project id, reach out to an
[editor](https://pantheon.corp.google.com/iam-admin/iam) of that project to
request access.

```
bqschemaupdater --help
```

## Supported Uses

The operations supported by this tool include:

* Creating a new table
* Adding NULLABLE or REPEATED columns to an existing table
* Making REQUIRED fields NULLABLE in an existing table

# Standard Practices

Table IDs and Dataset IDs should be underscored delimited, e.g. `test_results`.

## Schema Definitions

Columns in BigQuery tables cannot be modified or deleted once they have been
created. However, it is easy to add columns, so err on the side of not adding a
column if you are unsure.

BigQuery provides useful types such as RECORD and TIMESTAMP. It is a good idea
to take advantage of these types.

Events usually have an associated timestamp field recording the time the event
took place.

BigQuery discourages JOINs and encourages denormalizing data.

Flattening repeated fields can be costly. If you know that a repeated field
will only ever contain 0 or 1 value, consider not making that field repeated.

# BigQuery Limits

Please see [BigQuery
docs](https://cloud.google.com/bigquery/quotas#datasettableupdates) for the most
updated limits for creating and modifying tables. It is not expected that we
will exceed these limits through bqschemaupdater usage. If you are planning
a project which might, please contact the Monitoring Team.
