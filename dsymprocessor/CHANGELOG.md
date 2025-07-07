# dSYM Processor changelog

## v0.0.5 [beta] - 2025/07/02
### 🚧 Maintenance
- maint(deps): bump the aws group across 2 directories with 1 update (#79)
- maint(deps): bump the otel group across 3 directories with 5 updates (#81)
- maint: Update symbolic-go (#83)


## v0.0.4 [beta] - 2025/06/27
### ✨ Features

- feat: include error message on the log when symbolication fails (#77) | @mustafahaddara

### 🐛 Fixes

- fix: get app.debug.build_uuid and app.bundle.executable from the resource attributes, not the log attributes (#77) | @mustafahaddara
- fix: do not crash when a line of the stack trace doesn't match our expected regex (#77) | @mustafahaddara

## v0.0.3 [beta] - 2025/06/26
### ✨ Features

- feat: add exception.type and exception.message attributes to metrickit crashes (#76) | @mustafahaddara

## v0.0.2 [beta] - 2025/06/25
### ✨ Features

- feat: symbolicate generic stack traces (#73) | @mustafahaddara

## v0.0.1 [beta] - 2025/06/16
### ✨ Features

- feat: support symbolicating metrickit stack traces with dSYMs (#53) | @mustafahaddara
