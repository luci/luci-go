load("@stdlib//internal/luci/lib/cq.star", "cq")

assert.fails(
    lambda: cq.run_mode(name = cq.MODE_DRY_RUN),
    'bad "mode_name": "DRY_RUN" and "FULL_RUN" are reserved by CQ',
)
assert.fails(
    lambda: cq.run_mode(name = cq.MODE_FULL_RUN),
    'bad "mode_name": "DRY_RUN" and "FULL_RUN" are reserved by CQ',
)
