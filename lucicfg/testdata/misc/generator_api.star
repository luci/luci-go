load("@stdlib//internal/luci/proto.star", "config_pb")

def gen(ctx):
    ctx.output["text.txt"] = "Just text\n"
    ctx.output["json.json"] = json.encode({"hello": "world"}) + "\n"
    ctx.output["project.cfg"] = config_pb.ProjectCfg(
        name = "test",
        access = ["group:all"],
    )
    ctx.declare_config_set("testing/set", ".")

lucicfg.generator(impl = gen)

# Expect configs:
#
# === json.json
# {"hello":"world"}
# ===
#
# === project.cfg
# name: "test"
# access: "group:all"
# ===
#
# === text.txt
# Just text
# ===
