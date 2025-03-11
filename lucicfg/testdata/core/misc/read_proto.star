load("//misc/support/test_pb.star", "testproto")

def test_read_proto_ok():
    def check(path, encoding = None):
        msg = io.read_proto(testproto.Msg, path, encoding)
        assert.eq(msg.i, 123)

    check("support/proto.json")
    check("support/proto.textpb")
    check("support/proto.json", "jsonpb")
    check("support/proto.textpb", "textpb")
    check("//misc/support/proto.json")
    check("//misc/support/proto.textpb")

def test_read_proto_fail():
    def call(path, encoding = None):
        io.read_proto(testproto.Msg, path, encoding)

    assert.fails(lambda: call("missing_file.txt"), "no such file")
    assert.fails(lambda: call("//missing_file.txt"), "no such file")
    assert.fails(lambda: call("../../../../README.md"), "outside the package root")
    assert.fails(lambda: call("support/proto.json", "textpb"), "syntax error")
    assert.fails(lambda: call("support/proto.textpb", "jsonpb"), "syntax error")
    assert.fails(lambda: call("support/proto.json", "huh"), 'unknown proto encoding "huh"')

test_read_proto_ok()
test_read_proto_fail()
