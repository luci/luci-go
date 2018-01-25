# Limits

Please see [BigQuery
docs](https://cloud.google.com/bigquery/quotas#streaminginserts) for the most
updated limits for streaming inserts. It is expected that the client is
responsible for ensuring their usage will not exceed these limits through
bq usage. A note on maximum rows per request: Put() batches rows per request,
ensuring that no more than 10,000 rows are sent per request, and allowing for
custom batch size. BigQuery recommends using 500 as a practical limit (so we use
this as a default), and experimenting with your specific schema and data sizes
to determine the batch size with the ideal balance of throughput and latency for
your use case.

# Authentication

Authentication for the Cloud projects happens
[during client creation](https://godoc.org/cloud.google.com/go#pkg-examples).
What form this takes depends on the application.

# Monitoring

You can use [tsmon](https://godoc.org/go.chromium.org/luci/common/tsmon) to
track upload latency and errors.

[Uploader](https://godoc.org/go.chromium.org/luci/common/bq#NewUploader) has a
public field, UploadsMetricName, which will automatically create a counter
metric to track successes and failures. It does not automatically create a
metric for tracking latency (a cumulative distribution metric is recommended),
but could be modified to do so.
