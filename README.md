git-sweep
=========

git-sweep cleans up your local git branches once they're squashed and merged into master.

It's intended to work with a very specific workflow (see below)

Status
------
This is super alpha so no guarantees whatsoever.

Installation
------------

```
go get -u github.com/gottwald/git-sweep
```

Workflow
--------

This is a typical workflow where git-sweep can help:

Checkout your feature branch and commit something.

```
$ git checkout -b my-feature-branch
$ git add my-changes.go
$ git commit -m "changed things"
```

Push your work to remote and use branch tracking (important!)

```
$ git push -u origin my-feature-branch
```

Open your pull request.
Squash merge it and delete the remote branch through the UI.

A normal `git pull --prune` will only remove your remote reference but not your local branch. The local branch will stick around and even a `git branch -d my-feature-branch` will complain because the feature branch doesn't share a common history with master anymore due to the rebase/squash.

This is where `git-sweep` comes in.
`git-sweep` will search through your tracking branches and see if the tracked remote branch is still there. If it's gone then it will delete the local branch.
The flag `--dry-run` will show you which branches it would delete.

```
$ git sweep --dry-run
would delete "my-feature-branch"
$ git sweep
```

Please note `git pull --prune` or `git fetch --prune` is important because it removes the local references to the remote branches which `git-sweep` takes as hint that this is now merged.
You can enable automatic pruning for pull and fetch for your remote by using: `git config remote.origin.prune true`.

Gerrit Workflow
---------------

Gerrit works a bit differently.
The `git-sweep` command stays exactly the same but the workflow differs a bit.
In gerrit you typically work with a local branch and push it to a remote magic ref like `refs/for/master` which would then generate a patch set on the gerrit server. When something is then merged into master it is usually rebased or cherry-picked.

But gerrit has a commit hook that assigns change IDs to a patch which `git-sweep` can rely on.
In all local branches it tries to find such a change ID as a marker that this is a gerrit branch. It then uses this ID and walks the master branch back to find whether this change ID is already in some commit of the master tree. If it is, then the branch was already merged of cherry-picked and `git-sweep` can remove it.

What's up next?
---------------

This was hacked together in an afternoon to scratch my own itch.
It certainly could use some unit tests.
