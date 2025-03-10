def test_package_dir_ok():
    def call(path):
        rel, ok = __native__.package_dir(path, "main/pkg")
        assert.true(ok)
        return rel

    # The target dir is under the main package dir => always relative path.
    assert.eq(call("."), ".")
    assert.eq(call("dir"), "..")
    assert.eq(call("dir/deeper"), "../..")
    assert.eq(call("dir/../deeper"), "..")
    assert.eq(call("../pkg/dir"), "..")  # up and then back down

    # The target dir is outside of the main package dir.
    assert.eq(call(".."), "pkg")
    assert.eq(call("../.."), "main/pkg")
    assert.eq(call("../deeper"), "../pkg")
    assert.eq(call("../../deeper"), "../main/pkg")
    assert.eq(call("../../deeper/deepest"), "../../main/pkg")

def test_package_dir_outside_of_repo():
    rel, ok = __native__.package_dir("../../..", "main/pkg")
    assert.true(not ok)
    assert.eq(rel, "")

def test_package_at_repo_root():
    rel, ok = __native__.package_dir(".", ".")
    assert.true(ok)
    assert.eq(rel, ".")

    rel, ok = __native__.package_dir("dir", ".")
    assert.true(ok)
    assert.eq(rel, "..")

    rel, ok = __native__.package_dir("dir/deeper", ".")
    assert.true(ok)
    assert.eq(rel, "../..")

    # The target dir is outside of the repo => error.
    rel, ok = __native__.package_dir("..", ".")
    assert.true(not ok)
    rel, ok = __native__.package_dir("../dir", ".")
    assert.true(not ok)

test_package_dir_ok()
test_package_dir_outside_of_repo()
test_package_at_repo_root()
