# Releasing

When you want to cut a new release, you can:

1. Merge any of the changes you want in the release to master.
2. Ensure that Terraform acceptance tests have passed.
3. Create a new branch that adjusts the CHANGELOG so all unreleased changes
   appear under the new version.
4. Open a pull request with this change, get it reviewed, and merge it to master.
5. Once merged, tag the resulting master commit with whatever your release
   version should be, and push with the tags flag set.

That will trigger the CI pipeline that will publish your provider version to the
Terraform registry.

That ends up looking like this:

```
git checkout -b changelog-v1.2.3
git commit -m "Changelog for v1.2.3"
git push -u origin changelog-v1.2.3
# open a PR for the branch, get it reviewed and merged, then:
git checkout master
git pull
git tag v1.2.3
git push --tags
```
