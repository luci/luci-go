Running locally
---------------

Ability to run BQ export code locally is very useful when developing or
debugging it. But this needs to be done carefully to avoid disrupting BQ exports
running for real (in particular on the staging instance of Swarming).

While theoretically one can use a completely separate Cloud Project to host
both a Cloud Datastore database with entities to export and a BigQuery dataset
to export them into, practically this is not very useful, since such Cloud
Datastore will need to be manually populated first.

Instead it is OK to use staging Cloud Datastore (that is already full of
real entities), and export it into some experimental staging BigQuery
dataset. `exporter` code has a couple of tweaks to allow such usage.


Instructions for exporting staging data locally
-----------------------------------------------

To run the exporter locally using `chromium-swarm-dev` staging data:

1. Create a new BigQuery dataset in `chromium-swarm-dev` cloud project
(e.g. by clicking buttons in Cloud Console). Name it `dev_${USER}`, where
`${USER}` is your local username, e.g. `dev_yourname`. Optionally enable
table expiration to avoid accumulating garbage in BigQuery.
2. To create tables (or update schemas of existing ones), run
`server/cmd/update_bq_schema.sh` script. When asked, enter `chromium-swarm-dev`
as the cloud project name (this is the default) and `dev_yourname` as the
dataset name. It will ask for a bunch of confirmations when creating tables.
3. Launch the exporter server locally, optionally limiting it to the one table
you care about (`task_requests` in this case; other options are `bot_events`,
`task_results_run`, `task_results_summary`):
```
cd server/cmd/exporter
go run main.go \
  -cloud-project chromium-swarm-dev \
  -bq-export-dataset "dev_${USER}" \
  -bq-export-only-one-table task_requests
```
4. Go to the server admin UI http://127.0.0.1:8900/admin/portal/cron.
5. Click `Run bq-export now` button to trigger the exporter cron job.

If this is the first run of this job ever, it will just create an initial
`bq.ExportSchedule` entity and will not do anything else at all. Run it again
e.g. 15 sec later: it should launch **one** local TQ task that does the actual
BQ export of real `chromium-swarm-dev` entities (that appeared recently) into
`dev_${USER}` dataset.


Sticky cron job state
---------------------

The state of this dev cron job is stored in `bq.ExportSchedule` entity with ID
like `dev:${USER}:task_requests`. Just like the real cron job, such dev cron job
also processes datastore entities in increments of 15 sec. It means if you click
`Run bq-export now` again, it will "resume" exporting from where it stopped the
last time, regardless of what is the current wall clock time. For example, if
the last time you ran this job was 1 month ago, the cron job will resume
exporting 1 month old data.

To make it start exporting freshest entities go to the Cloud Console, find
`bq.ExportSchedule` entity with ID e.g. `dev:${USER}:task_requests` and delete
it. This resets the state.
