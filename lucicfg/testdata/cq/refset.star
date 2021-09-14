load("@stdlib//internal/luci/lib/cq.star", "cq")

def test_refset_ok():
    def cmp(in_repo, in_refs, in_refs_exclude, out_gob, out_proj, out_refs, out_refs_exclude):
        refset = cq.refset(repo = in_repo, refs = in_refs, refs_exclude = in_refs_exclude)
        assert.eq(refset.__gob_host, out_gob)
        assert.eq(refset.__gob_proj, out_proj)
        assert.eq(refset.__refs, out_refs)
        assert.eq(refset.__refs_exclude, out_refs_exclude)

    cmp(
        "https://example.googlesource.com/repo",
        [".*"],
        None,
        "example",
        "repo",
        [".*"],
        [],
    )
    cmp(
        "https://example-review.googlesource.com/a/zzz/repo",
        None,
        None,
        "example",
        "zzz/repo",
        ["refs/heads/main"],
        [],
    )
    cmp(
        "https://example-review.googlesource.com/repo",
        ["refs/heads/.+"],
        ["refs/heads/ex*"],
        "example",
        "repo",
        ["refs/heads/.+"],
        ["refs/heads/ex*"],
    )

def test_refset_fail():
    def fail(repo, msg):
        assert.fails(lambda: cq.refset(repo = repo), msg)

    fail("example.googlesource.com/repo", 'should match "https://')
    fail("https://github.com/zzz", r"only \*\.googlesource.com repos are supported currently")
    fail("https://-review.googlesource.com/repo", "not a valid repository URL")
    fail("https://example.googlesource.com/a/", "not a valid repository URL")

test_refset_ok()
test_refset_fail()
