resultdb.settings(
    enable = True,
    bq_exports = [
        resultdb.export_test_results(
            bq_table = "a.b",
        ),
    ],
)
# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/short_resultdb_bigquery_table.star: in <toplevel>
# ...
# Error: bad "bq_table": "a.b" should match "^([^.]+)\\.([^.]+)\\.([^.]+)$"
