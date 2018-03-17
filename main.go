package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/pflag"
	git "gopkg.in/src-d/go-git.v4"
)

var flagDryRun bool

func removeBranchFromConfig(repo *git.Repository, branch string) error {
	config, err := repo.Config()
	if err != nil {
		return fmt.Errorf("couldn't get git config: %s", err)
	}
	newRawConfig := config.Raw.RemoveSubsection("branch", branch)
	config.Raw = newRawConfig
	return repo.Storer.SetConfig(config)
}

func openCurPathRepo() (*git.Repository, error) {
	trypath, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("could not get current working directory: %s", err)
	}
	for len(trypath) > 1 {
		repo, err := git.PlainOpen(trypath)
		if err == git.ErrRepositoryNotExists {
			trypath = filepath.Join(trypath, "../")
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("could not open git repository: %s", err)
		}
		return repo, nil
	}
	return nil, fmt.Errorf("cwd is not a git repository")
}

func errExit(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func main() {
	pflag.BoolVarP(&flagDryRun, "dry-run", "", false, "does not actually delete branches")
	pflag.Parse()

	repo, err := openCurPathRepo()
	if err != nil {
		errExit(err)
	}

	gerritC := NewGerritCleaner(repo)
	gerritBranches, err := gerritC.MergedBranches()
	if err != nil {
		errExit(fmt.Errorf("getting merged gerrit branches failed: %s\n", err))
	}

	gitC := NewGitCleaner(repo)
	gitBranches, err := gitC.MergedBranches()
	if err != nil {
		errExit(fmt.Errorf("getting merged git branches failed: %s\n", err))
	}

	branches := append(gerritBranches, gitBranches...)
	for _, branchRef := range branches {
		if flagDryRun {
			fmt.Printf("would delete %q\n", branchRef.Name())
		} else {
			repo.Storer.RemoveReference(branchRef.Name())
			removeBranchFromConfig(repo, branchRef.Name().Short())
		}
	}
}
