package main

import (
	"regexp"

	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

var changeIDRegex = regexp.MustCompile(`(?m)^Change-Id: I[0-9a-f]{8,}.*$`)

type GerritCleaner struct {
	repo *git.Repository
}

func NewGerritCleaner(repo *git.Repository) *GerritCleaner {
	return &GerritCleaner{
		repo: repo,
	}
}

func (gc *GerritCleaner) MergedBranches() ([]*plumbing.Reference, error) {
	branchIter, err := gc.repo.Branches()
	if err != nil {
		return nil, err
	}
	mergedBranches := []*plumbing.Reference{}
	for {
		branch, err := branchIter.Next()
		if err != nil {
			return mergedBranches, nil
		}
		if branch.Name() == plumbing.Master {
			continue
		}
		// TODO handle this error
		isMerged, _ := gc.branchIsMerged(branch)
		if isMerged {
			mergedBranches = append(mergedBranches, branch)
		}
	}
}

func (gc *GerritCleaner) branchIsMerged(ref *plumbing.Reference) (bool, error) {
	isMerged := false
	commit, err := gc.repo.CommitObject(ref.Hash())
	if err != nil {
		return isMerged, err
	}
	changeID, ok := getChangeID(commit)
	if !ok {
		// this is not a branch with gerrit commits => ignore
		return false, nil
	}
	isMerged = gc.findChangeInMaster(changeID, commit)
	return isMerged, nil
}

func (gc *GerritCleaner) findChangeInMaster(changeID string, commit *object.Commit) bool {
	master, err := gc.repo.Reference(plumbing.Master, true)
	if err != nil {
		// TODO handle this
		panic(err)
	}
	masterIter, err := gc.repo.Log(&git.LogOptions{
		From: master.Hash(),
	})
	if err != nil {
		// TODO handle this
		panic(err)
	}
	for {
		mCommit, err := masterIter.Next()
		if err != nil {
			// walked whole master, we're done
			return false
		}
		mChangeID, ok := getChangeID(mCommit)
		if !ok {
			continue
		}
		if changeID == mChangeID {
			return true
		}
	}
	return false
}

// getChangeID takes a commit and tries to get a gerrit change ID from the
// commit message. It returns the found change ID as a string and a boolean to
// indicate whether something was found.
func getChangeID(commit *object.Commit) (string, bool) {
	changeIDStr := changeIDRegex.FindString(commit.Message)
	// the returned string usually looks like:
	// Change-Id: Ibd6b990eb12f6b1464b1d139fb51c0898e5b073e
	// The random string part has to be at least 8 characters after the I prefix
	if len(changeIDStr) < 20 {
		return "", false
	}
	return changeIDStr[11:], true
}
