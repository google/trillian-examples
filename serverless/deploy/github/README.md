# Deploying server legs on GitHub

Since serverless logs are just files, we can easily use GitHub to store and
make these available on-line - just check the files in and use GitHub's
`https://raw.githubusercontent.com/...` URLs to serve them.

To update these logs you'd clone the repository containing the log, use the
`sequence` and `integrate` tools to grow the log, and then create a PR with
the deltas.

But can we go further?

## GitHub Actions

We can configure the log repository to use GitHub Actions to automate much of
this process, e.g.:

1. A GitHub Action which examined incoming PRs for ones which contained _only_
   additions of files to a specific "pending" directory. These PRs would be
   automatically merged if the file(s) met some specific validation criteria,
   or rejected otherwise.
2. A subsequent GitHub Action would scan the "pending" directory, sequencing
   and integrating all files it found, and finally creating and merging a PR
   with the updated log files.
