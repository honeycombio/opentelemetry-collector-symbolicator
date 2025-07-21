# Releasing

- `Add steps to prepare release`
- Update `processorVersion` in the `sourcemapprocessor` package to the upcoming release
- Update relevant `CHANGELOG.md` files in each processor directory with the changes since the last release.
- Commit changes, push, and open a release preparation pull request for review.
- Once the pull request is merged, fetch the updated `main` branch.
- Apply a tag for the new version(s) on the merged commit (e.g. `git tag -a sourcemapprocessor/v1.2.3 -m "sourcemapprocessor/v1.2.3"`)
  - The tag name & version should correspond to the processor being released (ie. if releasing the dSYM processor, tag with `dsymprocessor/v1.2.3`)
  - It is important to prepend the tag with the processor name because these [modules are defined in their own subdirectories](https://go.dev/ref/mod#vcs-version). In order for users to install this module as `github.com/honeycombio/opentelemetry-collector-symbolicator/sourcemapprocessor` it needs to be tagged specifically.
- Push the tag upstream (this will kick off the release pipeline in CI) e.g. `git push origin sourcemapprocessor/v1.2.3`
- Copy changelog entries for newest version into draft GitHub release created as part of CI publish steps.
  - Make sure to "generate release notes" in github for full changelog notes and any new contributors
- Publish the github draft release.
