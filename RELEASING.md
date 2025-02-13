# Releasing

- `Add steps to prepare release`
- Update `CHANGELOG.md` with the changes since the last release.
- Commit changes, push, and open a release preparation pull request for review.
- Once the pull request is merged, fetch the updated `main` branch.
- Apply a tag for the new version on the merged commit (e.g. `git tag -a symbolicatorprocessor/v1.2.3 -m "symbolicatorprocessor/v1.2.3"`)
  - It is important to prepend the tag with `symbolicatorprocessor` because this [module is defined in a subdirectory](https://go.dev/ref/mod#vcs-version). In order for users to install this module as `github.com/honeycombio/opentelemetry-collector-symbolicator/symbolicatorprocessor` it needs to be tagged specifically.
- Push the tag upstream (this will kick off the release pipeline in CI) e.g. `git push origin symbolicatorprocessor/v1.2.3`
- Copy change log entry for newest version into draft GitHub release created as part of CI publish steps.
  - Make sure to "generate release notes" in github for full changelog notes and any new contributors
- Publish the github draft release and this will kick off publishing to GitHub and the NPM registry.