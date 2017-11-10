# Machine Database Setup

The Cloud SQL database is configured using [cloudsqlhelper]. Enable Cloud SQL in
your AppEngine project and create a Cloud SQL instance. Edit [dbs.yaml], and use
cloudsqlhelper to create the database and apply any pending [migrations].

[cloudsqlhelper]: https://chromium.googlesource.com/infra/infra/+/master/go/src/infra/tools/cloudsqlhelper/
[dbs.yaml]: ./dbs.yaml
[migrations]: ./migrations
