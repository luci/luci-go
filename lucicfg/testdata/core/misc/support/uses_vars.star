load("//misc/support/shared_vars.star", "shared_vars")

shared_vars.set_b("from inner")

sees_vars = [shared_vars.a.get(), shared_vars.b.get()]
