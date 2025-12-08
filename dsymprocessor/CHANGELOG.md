# dSYM Processor changelog

## Unreleased

- feat: add optional language check to processors (#133) | @jairo-mendoza
- refactor: improve parity between processors' configs (#131) | @jairo-mendoza

## v0.0.9 [beta] - 2025/11/19

### 💡 Enhancements

- refactor: move telemetry in dsym processor to only symbolicated logs (#128) | @jairo-mendoza
- perf: enhance symbolication process with per stacktrace error caching (#119) | @clintonnkemdilim

## v0.0.8 [beta] - 2025/11/10

- maint: bump dependency to v1.45.0/v0.139.0 (#121) | @TylerHelmuth

## v0.0.7 [beta] - 2025/10/21

- chore: reduce log verbosity by changing "Processing logs" from Info to Debug level (#111) | @clintonnkemdilim
- feat: add internal telemetry to dsym processor (#108) | @jairo-mendoza
- feat: emit processor version and type as attributes from all processors (#107) | @jairo-mendoza

## v0.0.6 [beta] - 2025/07/24

### 🐛 Fixes

- fix: stack traces no longer have extra `()` characters (#88) | @mustafahaddara

### 🚧 Maintenance

- chore: improve parity for symbolication failure across the 3 processors (#86) | @jairo-mendoza
- maint(deps): bump the aws group across 3 directories with 1 update (#85)

### 🚨 Breaking Changes

- chore: rename top-level config key from `dsymprocessor` to `dsym_symbolicator` (#89) | @mustafahaddara

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
