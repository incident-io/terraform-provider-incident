# Releasing

When you want to cut a new release, you can:

1. Merge any of the changes you want in the release to master.
2. Ensure that Terraform acceptance tests have passed.
3. Create a new commit on master that adjusts the CHANGELOG so all unreleased
   changes appear under the new version.
4. Push this commit to master
5. Tag that commit with whatever your release version should be, and push with the tags flag set.

That will trigger the CI pipeline that will publish your provider version to the
Terraform registry.

That ends up looking like this:

```
git commit -m "Changelog for v1.2.3"
git push
git tag v1.2.3
git push --tags
```
