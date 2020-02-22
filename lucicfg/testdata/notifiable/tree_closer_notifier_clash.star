lucicfg.enable_experiment('crbug.com/1054172')

luci.tree_closer(
    name = 'some name',
    tree_status_host = 'some-tree.example.com',
)

luci.notifier(
    name = 'some name',
    on_occurrence = ['FAILURE'],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/notifiable/tree_closer_notifier_clash.star: in <toplevel>
#   ...
# Error: luci.notifiable("some name") is redeclared, previous declaration:
# Traceback (most recent call last):
#   //testdata/notifiable/tree_closer_notifier_clash.star: in <toplevel>
#   ...
