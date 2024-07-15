# Releasing

When you want to cut a new release, you can:

1. Merge any of the changes you want in the release to master.
2. Ensure that Terraform acceptance tests have passed.
3. Create a new commit on master that adjusts the CHANGELOG so all unreleased
   changes appear under the new version.
4. Push that commit and tag it with whatever your release version should be.

That will trigger the CI pipeline that will publish your provider version to the
terraform registry.
