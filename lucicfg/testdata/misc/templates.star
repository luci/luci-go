load("@stdlib//internal/strutil.star", "strutil")

def test_template_works():
    t = strutil.template("{{.arg1}} {{.arg2.inner}}")
    assert.true(t)
    assert.eq(type(t), "template")
    assert.eq(str(t), "template(...)")
    assert.eq(t.render(arg1 = "hello", arg2 = {"inner": "world"}), "hello world")

def test_iteration_works():
    t = strutil.template("""
{{- range .slice}}
* {{.}}
{{- end}}
""")
    assert.eq(t.render(slice = ["1", 2, ["3"]]), """
* 1
* 2
* [3]
""")

def test_template_cache():
    t1 = strutil.template("{{.arg1}}")
    t2 = strutil.template("{{.arg1}}")
    t3 = strutil.template("{{.another}}")
    assert.true(t1 == t2)
    assert.true(t1 != t3)

def test_template_hash():
    d = {strutil.template("{{.arg1}}"): 123}
    assert.eq(d[strutil.template("{{.arg1}}")], 123)

def test_bad_syntax():
    assert.fails(
        lambda: strutil.template("{{ zzzz"),
        'template: <str>:1: function "zzzz" not defined',
    )

def test_bad_execution():
    t = strutil.template("{{range .number}}{{end}}")
    assert.fails(lambda: t.render(number = 123), "range can't iterate over 123")

test_template_works()
test_iteration_works()
test_template_cache()
test_template_hash()
test_bad_syntax()
test_bad_execution()
