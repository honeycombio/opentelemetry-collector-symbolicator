# Symbolicator Processor changelog

## v0.0.3 [beta] - 02/11/2025

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