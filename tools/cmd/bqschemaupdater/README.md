bqschemaupdater is a tool for adding and updating BigQuery table schema.

It currently creates tables in the "chrome-infra-events" cloud project by
default. We can add support for different projects when needed in the future.

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

# Standard Practices

Table IDs and Dataset IDs should be underscored delimited, e.g. `test_results`.

# Supported Uses

The operations supported by this tool include:

* Creating a new table
* Adding NULLABLE or REPEATED columns to an existing table
* Making REQUIRED fields NULLABLE in an existing table
