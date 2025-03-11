resultdb.settings(
    enable = True,
    bq_exports = [
        resultdb.export_test_results(
            bq_table = "a.b.c.d",
        ),
    ],
)
# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/long_resultdb_bigquery_table.star: in <toplevel>
# ...
# Error: bad "bq_table": "a.b.c.d" should match "^([^.]+)\\.([^.]+)\\.([^.]+)$"
