# Symbolicator Processor changelog

## Unreleased

- feat: Add support for react native stacktraces & native frames (#137) | @jairo-mendoza
- feat: Emit parity check telemetry (for internal testing) (#136) | @jairo-mendoza
- feat: add optional language check to processors (#133) | @jairo-mendoza
- feat(sourcemap-processor): Implement collector-side parsing (#132) | @jairo-mendoza
- refactor: improve parity between processors' configs (#131) | @jairo-mendoza

## v0.0.14 [beta] - 2025/11/19

### ✨ Features

- feat: Add log support to the source map processor (#123) | @jairo-mendoza

### 💡 Enhancements

- refactor: prevent running sourcemap processor on traces/logs without exception attributes (#126) | @jairo-mendoza

### 🚨 Breaking Changes

- renamed top-level config key from `output_stack_trace_key` to `stack_trace_attribute_key`

## v0.0.13 [beta] - 2025/11/10

- maint: bump dependency to v1.45.0/v0.139.0 (#121) | @TylerHelmuth
- perf: enhance symbolication process with per stacktrace error caching (#116) | @clintonnkemdilim

## v0.0.12 [beta] - 2025/10/22

- feat: if `app.debug.source_map_uuid` is in the resource attributes, include it in source paths (#113) | @beekhc

## v0.0.11 [beta] - 2025/10/21

- chore: reduce log verbosity by changing "Processing traces" from Info to Debug level (#111) | @clintonnkemdilim
- feat: emit processor version and type as attributes from all processors (#107) | @jairo-mendoza

## v0.0.10 [beta] - 2025/07/24

### ✨ Features

- feat: Add internal processor telemetry to symbolicatorprocessor (#80) | @jairo-mendoza

### 🚨 Breaking Changes

- chore: rename symbolicatorprocessor to sourcemapprocessor (#89) | @mustafahaddara
- chore: rename top-level config key from `symbolicator` to `source_map_symbolicator` (#89) | @mustafahaddara

### 🚧 Maintenance

- chore: improve parity for symbolication failure across the 3 processors (#86) | @jairo-mendoza
- maint(deps): bump the aws group across 3 directories with 1 update (#85)

## v0.0.9 [beta] - 2025/07/02

### 🚧 Maintenance

- maint(deps): bump the aws group across 2 directories with 1 update (#79)
- maint(deps): bump the otel group across 3 directories with 5 updates (#81)
- maint: Update symbolic-go (#83)

## v0.0.8 [beta] - 2025/06/27

### ✨ Features

- feat: include error message on the log when symbolication fails (#77) | @mustafahaddara

## v0.0.7 [beta] - 2025/06/16

### 🚧 Maintenance

- chore: update dependencies and golang versions | @martin308

## v0.0.6 [beta] - 2025/02/28

### ✨ Features

- feat: Remove path when looking for source files (#36) | @martin308

## v0.0.5 [beta] - 2025/02/21

### ✨ Features

- feat: Keep query params in url path (#32) | @jairo-mendoza

### 🐛 Fixes

- fix: update s3 example to include s3_source_maps option (#33) | @jairo-mendoza

## v0.0.4 [beta] - 2025/02/13

### ✨ Features

- feat(processor): Handle symbolication errors (#31) | @jairo-mendoza
- feat: GCS support (#26) | @martin308

### 🚧 Maintenance

- chore: go work sync (#27) | @martin308

### 🐛 Fixes

- fix: Update path for releases (#25) | @pkanal
- fix: Dockerfile path (#30) | @martin308

## v0.0.3 [beta] - 2025/02/11

### ✨ Features

- feat: Add a simple LRU cache (#22) | @pkanal
- feat: Update documentation (#21) | @martin308
- feat: Update for repo visibility change (#19) | @martin308
- feat: Load source maps from S3 (#17) | @martin308
- feat: cache source map objects (#16) | @martin308
- feat: development collector configuration (#15) | @martin308
- feat: Copy dependencies (#14) | @martin308
- feat(build): Add Dockerfile to build and distribute collector distro (#9) | @pkanal
- feat: Add stack message and type to output stack trace (#10) | @jairo-mendoza
- feat(processor): Add extra symbolicated attributes (#8) | @jairo-mendoza

### 🐛 Fixes

- fix: Provide path for docker build command (#20) | @martin308
- fix: Update source map URL regex (#18) | @pkanal
- fix: Switch from alpine to distroless (#13) | @pkanal
- fix: Only use url path to resolve files (#12) | @martin308
