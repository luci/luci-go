load("@stdlib//internal/sequence.star", "sequence")

def test_sequence_next():
    assert.eq(sequence.next("a"), 1)
    assert.eq(sequence.next("a"), 2)
    assert.eq(sequence.next("b"), 1)

test_sequence_next()
