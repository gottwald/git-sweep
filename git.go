package main

import (
	"fmt"

	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

type GitCleaner struct {
	repo *git.Repository
}

func NewGitCleaner(repo *git.Repository) *GitCleaner {
	return &GitCleaner{
		repo: repo,
	}
}

func (gc *GitCleaner) MergedBranches() ([]*plumbing.Reference, error) {
	branchIter, err := gc.repo.Branches()
	if err != nil {
		return nil, err
	}
	// TODO handle this error
	orphaned, _ := gc.getOrphanedTrackingBranches()
	mergedBranches := []*plumbing.Reference{}
	branchIter.ForEach(func(ref *plumbing.Reference) error {
		for _, oBranch := range orphaned {
			if oBranch == ref.Name().Short() {
				mergedBranches = append(mergedBranches, ref)
			}
		}
		return nil
	})
	return mergedBranches, nil
}

func (gc *GitCleaner) getOrphanedTrackingBranches() ([]string, error) {
	cfg, err := gc.repo.Config()
	if err != nil {
		return nil, fmt.Errorf("could not get git config: %s", err)
	}
	orphanedBranches := []string{}
	for _, cfgBranch := range cfg.Raw.Section("branch").Subsections {
		remote := cfgBranch.Option("remote")
		merge := plumbing.ReferenceName(cfgBranch.Option("merge"))
		if remote == "" || merge == "" {
			// not a tracking branch
			continue
		}
		remoteRefName := plumbing.ReferenceName(fmt.Sprintf("refs/remotes/%s/%s", remote, merge.Short()))
		_, err := gc.repo.Reference(remoteRefName, false)
		if err == nil {
			// ref exists - keep it locally
			continue
		}
		if err != plumbing.ErrReferenceNotFound {
			// unknown error - ignore branch
			// TODO log in verbose mode
			continue
		}
		orphanedBranches = append(orphanedBranches, cfgBranch.Name)
	}
	return orphanedBranches, nil
}
