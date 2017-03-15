# BigQuery

Token Server uses BigQuery for storing structured logs that contain details
of all generated tokens, for debugging and audit purposes.

This directory contains definitions of BigQuery tables, and a small tool that
knows how to transform these definitions into real BigQuery tables.

Prerequisites for using the tool:
  * 'bq' is in PATH. It's part of Google Cloud SDK.
  * gcloud application default credentials are configured.

To update BigQuery tables used by Token Server ('tokens' dataset):

./bq_util.py -p <app_id> -d tokens update-tables ./tables/*.schema
