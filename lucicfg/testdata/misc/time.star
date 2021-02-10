load("@stdlib//internal/time.star", "time")

sec = lambda v: time.duration(v * 1000)

def test_duration_type():
    # Basic starlark stuff.
    assert.eq(type(sec(1)), "duration")
    assert.eq(str(sec(3600)), "1h0m0s")

    # Comparisons.
    assert.true(sec(123) == sec(123))
    assert.true(sec(123) != sec(456))
    assert.true(sec(123) < sec(456))
    assert.true(sec(456) > sec(123))
    assert.true(sec(123) <= sec(123))
    assert.true(sec(123) >= sec(123))

    # duration +- duration binary ops.
    assert.eq(sec(1) + sec(2), sec(3))
    assert.eq(sec(2) - sec(1), sec(1))

    # duration */ int binary op.
    assert.eq(sec(1) * 10, sec(10))
    assert.eq(10 * sec(1), sec(10))
    assert.eq(sec(10) / 10, sec(1))
    assert.eq(time.duration(7500) % sec(1), 500)

    # duration / duration binary op.
    assert.eq(sec(100) / sec(5), 20)

    # +- unary ops.
    assert.eq(+sec(1), sec(1))
    assert.eq(-sec(1), sec(-1))

def test_type_mismatches():
    # Not compatible with ints, by design.
    assert.fails(lambda: sec(1) + 1, "unknown binary op")
    assert.fails(lambda: 1 + sec(1), "unknown binary op")
    assert.fails(lambda: sec(1) > 1, "not implemented")
    assert.fails(lambda: 1 > sec(1), "not implemented")

    # Can't multiply two durations.
    assert.fails(lambda: sec(1) * sec(2), "unknown binary op")

    # Can't divide ints by duration.
    assert.fails(lambda: 10 / sec(1), "unknown binary op")

def test_helpers():
    assert.eq(60 * time.second + time.hour, 61 * time.minute)
    assert.eq(3600 * time.second / time.hour, 1)
    assert.eq(time.second * 3600 / time.hour, 1)
    assert.eq(time.truncate(2 * time.hour + time.minute, time.hour), 2 * time.hour)

def test_epoch():
    assert.eq(time.epoch(time.short_date, "2020-01-13 16:21:33", "America/Denver"), 1578957693)
    assert.eq(time.epoch(time.short_date, "2020-01-09 19:00:00", "America/Denver"), 1578621600)
    assert.eq(time.epoch(time.short_date, "2020-01-10 18:21:25", "America/Denver"), 1578705685)
    assert.eq(time.epoch(time.short_date, "1969-12-31 17:00:00", "America/Denver"), 0)
    assert.eq(time.epoch(time.short_date, "1969-12-31 16:00:00", "America/Los_Angeles"), 0)

def test_days_of_week():
    good_cases = {
        "": [],
        "   ": [],
        ",,,": [],
        "Mon": [1],
        "moN": [1],
        " Mon, ": [1],
        "Sun": [7],
        "Tue-Wed": [2, 3],
        " ,Tue - Wed ,": [2, 3],
        "Sun,Tue-Wed": [2, 3, 7],
        "Mon,Tue,Wed,Thu,Fri,Sat,Sun": [1, 2, 3, 4, 5, 6, 7],
        "Fri,Mon,Tue,Wed,Thu,Sat,Sun": [1, 2, 3, 4, 5, 6, 7],
        "Tue,Tue, Mon-Wed": [1, 2, 3],
    }
    for spec, out in good_cases.items():
        assert.eq(time.days_of_week(spec), out)

    bad_cases = {
        "huh": '"huh" is not a valid 3-char abbreviated day of the week',
        "mon-huh": '"huh" is not a valid 3-char abbreviated day of the week',
        "Sun-Mon": 'bad range "Sun-Mon" - "Sun" is later than "Mon"',
    }
    for spec, err in bad_cases.items():
        assert.fails(lambda: time.days_of_week(spec), err)

test_duration_type()
test_type_mismatches()
test_helpers()
test_epoch()
test_days_of_week()
