def test_ok():
    ctx = __native__.new_gen_ctx()
    ctx.declare_config_set("testing/1", ".")
    ctx.declare_config_set("testing/1", ".")  # redeclaring is OK
    ctx.declare_config_set("testing/1", "subdir/..")  # like this too
    ctx.declare_config_set("testing/2", ".")  # intersecting sets are OK
    ctx.declare_config_set("testing/3", "subdir")  # nested is fine

def test_redeclaration():
    ctx = __native__.new_gen_ctx()
    ctx.declare_config_set("testing", "abc")
    assert.fails(
        lambda: ctx.declare_config_set("testing", "def"),
        'set "testing" has already been declared',
    )

def test_bad_path():
    ctx = __native__.new_gen_ctx()
    assert.fails(
        lambda: ctx.declare_config_set("testing", "/abs"),
        "is not allowed",
    )
    assert.fails(
        lambda: ctx.declare_config_set("testing", ".."),
        "is not allowed",
    )
    assert.fails(
        lambda: ctx.declare_config_set("testing", "../."),
        "is not allowed",
    )
    assert.fails(
        lambda: ctx.declare_config_set("testing", "../dir"),
        "is not allowed",
    )

test_ok()
test_redeclaration()
test_bad_path()
