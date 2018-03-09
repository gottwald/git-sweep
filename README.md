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

What's up next?
-----------------

This was hacked together in an afternoon to scratch my own itch.
It certainly could use some unit tests.
I also want to integrate a solution for gerrit where this approach doesn't work.
