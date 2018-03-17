package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/pflag"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

var flagDryRun bool

type remoteRefLoader struct {
	cache map[string][]*plumbing.Reference
	auth  *Authenticator
}

func newRemoteRefLoader(auth *Authenticator) *remoteRefLoader {
	return &remoteRefLoader{
		cache: make(map[string][]*plumbing.Reference),
		auth:  auth,
	}
}

func (l *remoteRefLoader) listRefs(remote *git.Remote) ([]*plumbing.Reference, error) {
	refs, ok := l.cache[remote.String()]
	if ok {
		return refs, nil
	}
	if len(remote.Config().URLs) == 0 {
		return nil, errors.New("remote malformed: missing fetch URL")
	}
	authMethod, err := l.auth.GetMethod(remote.Config().URLs[0])
	if err != nil {
		return nil, fmt.Errorf("unable to get auth method for remote %q: %s", remote.Config().Name, err)
	}
	refs, err = remote.List(&git.ListOptions{
		Auth: authMethod,
	})
	if err != nil {
		return nil, fmt.Errorf("could not retrieve references: %s", err)
	}
	l.cache[remote.String()] = refs
	return refs, nil
}

func (l *remoteRefLoader) findRef(remote *git.Remote, ref string) (*plumbing.Reference, error) {
	gitRefs, err := l.listRefs(remote)
	if err != nil {
		return nil, fmt.Errorf("could not list references: %s", err)
	}
	for _, gitRef := range gitRefs {
		if gitRef.Name().String() == ref {
			return gitRef, nil
		}
	}
	return nil, errRefNotFound
}

type trackingBranch struct {
	name      string
	remote    *git.Remote
	remoteRef *plumbing.Reference
}

func getTrackingBranches(repo *git.Repository, refLoader *remoteRefLoader) ([]trackingBranch, error) {
	cfg, err := repo.Config()
	if err != nil {
		return nil, fmt.Errorf("could not get git config: %s", err)
	}
	tbs := []trackingBranch{}
	for _, cfgBranch := range cfg.Raw.Section("branch").Subsections {
		remote := cfgBranch.Option("remote")
		merge := cfgBranch.Option("merge")
		if remote == "" || merge == "" {
			// not a tracking branch
			continue
		}
		gitRemote, err := repo.Remote(remote)
		if err != nil {
			gitRemote = nil
		}
		tb := trackingBranch{
			name:   cfgBranch.Name,
			remote: gitRemote,
		}
		gitRef, err := refLoader.findRef(gitRemote, merge)
		if err != nil {
			if err == errRefNotFound {
				tbs = append(tbs, tb)
				continue
			}
			return nil, fmt.Errorf("reference loader error: %s", err)
		}
		tb.remoteRef = gitRef
		tbs = append(tbs, tb)
	}
	return tbs, nil
}

var errRefNotFound = errors.New("reference not found")

func orphanedTrackingBranches(tbs []trackingBranch) []trackingBranch {
	orphaned := []trackingBranch{}
	for _, tb := range tbs {
		if tb.remote == nil || tb.remoteRef == nil {
			orphaned = append(orphaned, tb)
		}
	}
	return orphaned
}

func deleteBranchFromRepo(repo *git.Repository, tb trackingBranch) error {
	iter, err := repo.Branches()
	if err != nil {
		return fmt.Errorf("could not get local branches: %s", err)
	}
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().Short() == tb.name {
			return repo.Storer.RemoveReference(ref.Name())
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("could not remove branch %q: %s", tb.name, err)
	}
	err = removeBranchFromConfig(repo, tb.name)
	if err != nil {
		return fmt.Errorf("could not remove branch %q from config: %s", tb.name, err)
	}
	return nil
}

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

	gc := NewGerritCleaner(repo)
	branches, err := gc.MergedBranches()
	if err != nil {
		errExit(fmt.Errorf("getting gerrit branches failed: %s\n", err))
	}
	for _, branchRef := range branches {
		if flagDryRun {
			fmt.Printf("would delete %q\n", branchRef.Name())
		} else {
			repo.Storer.RemoveReference(branchRef.Name())
		}
	}

	auth, err := NewAuthenticator()
	if err != nil {
		errExit(fmt.Errorf("Could not build authenticator: %s", err))
	}

	refLoader := newRemoteRefLoader(auth)
	tbs, err := getTrackingBranches(repo, refLoader)
	if err != nil {
		errExit(fmt.Errorf("could not get tracking branches: %s\n", err))
	}

	orphanedTBs := orphanedTrackingBranches(tbs)
	for _, tb := range orphanedTBs {
		if flagDryRun {
			fmt.Printf("would delete %q\n", tb.name)
		} else {
			deleteBranchFromRepo(repo, tb)
		}
	}
}
