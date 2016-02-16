# appengine/static/

Exposes common static files under `/static/common/`.

All frontend GAE modules should symlink this dir as 
`<module_dir>/includes/static`, where `<module_dir>` is where *.yaml is.
Then include it in the yaml file: 

```yaml
includes:
  - includes/static
```
