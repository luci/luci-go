bqschemaupdater is a tool for adding and updating BigQuery table schema.

# Usage

schemas should be written in .proto format.

Bqschemaupdater uses protoc found in $PATH.  Please make the latest
[protobuf library](https://github.com/google/protobuf) available in $PATH.

More information can be had with:

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
