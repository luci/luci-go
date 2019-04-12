load("@proto//luci/config/project_config.proto", config_pb="config")

def gen(ctx):
  ctx.output['text.txt'] = 'Just text\n'
  ctx.output['json.json'] = to_json({'hello': 'world'}) + '\n'
  ctx.output['project.cfg'] = config_pb.ProjectCfg(
      name = 'test',
      access = ['group:all'],
  )
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
