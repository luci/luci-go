load("@stdlib//internal/luci/lib/cq.star", "cq")

assert.fails(
    lambda: cq.run_mode(name = "DRY_RUN"),
    'bad "mode_name": "DRY_RUN", "FULL_RUN" and "NEW_PATCHSET_RUN" are reserved by CQ',
)
assert.fails(
    lambda: cq.run_mode(name = "FULL_RUN"),
    'bad "mode_name": "DRY_RUN", "FULL_RUN" and "NEW_PATCHSET_RUN" are reserved by CQ',
)
assert.fails(
    lambda: cq.run_mode(name = "NEW_PATCHSET_RUN"),
    'bad "mode_name": "DRY_RUN", "FULL_RUN" and "NEW_PATCHSET_RUN" are reserved by CQ',
)
