# Proguard Processor changelog

## Unreleased

## v0.0.6 [beta] - 2025/11/10

- maint: bump dependency to v1.45.0/v0.139.0 (#121) | @TylerHelmuth
- fix: Fix Proguard processor failing to retrieve Proguard UUID (#120) | @jairo-mendoza
- perf: enhance symbolication process with per stacktrace error caching (#117) | @clintonnkemdilim

## v0.0.5 [beta] - 2025/10/21

- chore: reduce log verbosity by changing "Processing logs" from Info to Debug level (#111) | @clintonnkemdilim
- feat: emit processor version and type as attributes from all processors (#107) | @jairo-mendoza
- feat: handle stack frames that don't need symbolication (#106) | @jairo-mendoza
- feat: add telemetry support to proguard symbolicator (#105) | @wolfgangcodes

## v0.0.4 [beta] - 2025/08/20

- chore: allow special negative line number in proguard processor (#102) | @jairo-mendoza
- fix: fix processor not partial symbolicating stacktraces (#101) | @jairo-mendoza
- feat: add exception type and message to outputted stacktraces (#99) | @jairo-mendoza

## v0.0.3 [beta] - 2025/07/24

### 🚧 Maintenance

- chore: improve parity for symbolication failure across the 3 processors (#86) | @jairo-mendoza
- maint(deps): bump the aws group across 3 directories with 1 update (#85)

## v0.0.2 [beta] - 2025/07/02

### 🚧 Maintenance

- maint: Update symbolic-go (#83)

## v0.0.1 [beta] - 2025/07/02

### ✨ Features

- Initial release!
