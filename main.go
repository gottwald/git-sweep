package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/user"
	"path/filepath"

	"github.com/spf13/pflag"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	gitconfig "gopkg.in/src-d/go-git.v4/plumbing/format/config"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"
	githttp "gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	gitssh "gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

var flagDryRun bool

type remoteRefLoader struct {
	cache map[string][]*plumbing.Reference
}

func newRemoteRefLoader() *remoteRefLoader {
	return &remoteRefLoader{
		cache: make(map[string][]*plumbing.Reference),
	}
}

func (l *remoteRefLoader) listRefs(remote *git.Remote) ([]*plumbing.Reference, error) {
	refs, ok := l.cache[remote.String()]
	if ok {
		return refs, nil
	}
	authMethod, err := authMethodForRemote(remote)
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

func authMethodForRemote(remote *git.Remote) (transport.AuthMethod, error) {
	if len(remote.Config().URLs) == 0 {
		return nil, errors.New("remote malformed: missing fetch URL")
	}
	fetchURL, err := url.Parse(remote.Config().URLs[0])
	if err != nil {
		// decorate error and store it for later
		err = fmt.Errorf("could not parse the remote fetch URL: %s", err)
	}
	if fetchURL == nil {
		// try ssh
		return authMethodForSSHRemote(remote)
	}
	switch fetchURL.Scheme {
	case "http", "https":
		return authMethodForHTTPRemote(remote, fetchURL)
	case "ssh":
		return authMethodForSSHRemote(remote)
	}
	return nil, fmt.Errorf("no auth method found for remote scheme %q", fetchURL.Scheme)
}

func authMethodForHTTPRemote(remote *git.Remote, fetchURL *url.URL) (transport.AuthMethod, error) {
	// see if the parsed url holds user-info which should precede token auth
	if fetchURL.User != nil {
		pw, _ := fetchURL.User.Password()
		return &githttp.BasicAuth{
			Username: fetchURL.User.Username(),
			Password: pw,
		}, nil
	}
	// try to get a github token from global git config
	osUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("couldn't get current user: %s", err)
	}
	f, err := os.Open(filepath.Join(osUser.HomeDir, ".gitconfig"))
	if err != nil {
		return nil, fmt.Errorf("couldn't read .gitconfig: %s", err)
	}
	var config gitconfig.Config
	if err := gitconfig.NewDecoder(f).Decode(&config); err != nil {
		return nil, fmt.Errorf("couldn't parse .gitconfig: %s", err)
	}
	ghSection := config.Section("github")
	if ghSection.Option("user") == "" {
		return nil, errors.New("user not found in .gitconfig github section")
	}
	if ghSection.Option("token") == "" {
		return nil, errors.New("token not found in .gitconfig github section")
	}
	return &githttp.BasicAuth{
		Username: ghSection.Option("user"),
		Password: ghSection.Option("token"),
	}, nil
}

func authMethodForSSHRemote(remote *git.Remote) (transport.AuthMethod, error) {
	agentAuth, err := gitssh.NewSSHAgentAuth("git")
	if err == nil {
		return agentAuth, nil
	}
	osUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("couldn't get current user: %s", err)
	}
	keyAuth, err := gitssh.NewPublicKeysFromFile("git", filepath.Join(osUser.HomeDir, "ssh", "id_rsa"), "")
	if err == nil {
		return keyAuth, nil
	}
	return nil, fmt.Errorf("neither ssh agent nor ssh keyfile auth could be built: %s", err)
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

func main() {
	pflag.BoolVarP(&flagDryRun, "dry-run", "", false, "does not actually delete branches")
	pflag.Parse()

	repo, err := openCurPathRepo()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	refLoader := newRemoteRefLoader()
	tbs, err := getTrackingBranches(repo, refLoader)
	if err != nil {
		fmt.Printf("could not get tracking branches: %s\n", err)
		os.Exit(1)
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
