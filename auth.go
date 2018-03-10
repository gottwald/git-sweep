package main

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	gitconfig "gopkg.in/src-d/go-git.v4/plumbing/format/config"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"
	githttp "gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	gitssh "gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

type Authenticator struct {
	osUser *user.User
}

func NewAuthenticator() (*Authenticator, error) {
	osUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("couldn't get current user: %s", err)
	}
	return &Authenticator{osUser: osUser}, nil
}

func (a *Authenticator) GetMethod(url string) (transport.AuthMethod, error) {
	ep, err := transport.NewEndpoint(url)
	if err != nil {
		return nil, fmt.Errorf("could not parse remote URL: %s", err)
	}
	switch ep.Protocol {
	case "ssh":
		return a.getSSHMethod(ep)
	case "file":
		return nil, nil
	case "http", "https":
		return a.getHTTPMethod(ep)
	}
	return nil, fmt.Errorf("no auth method found for remote protocol %q", ep.Protocol)
}

func (a *Authenticator) getHTTPMethod(ep *transport.Endpoint) (transport.AuthMethod, error) {
	user := a.osUser.Name
	// see if the endpoint holds user-info which should precede token auth
	if ep.User != "" {
		user = ep.User
	}
	if ep.Password != "" {
		return &githttp.BasicAuth{
			Username: user,
			Password: ep.Password,
		}, nil
	}
	// try to get a github token from global git config
	f, err := os.Open(filepath.Join(a.osUser.HomeDir, ".gitconfig"))
	if err != nil {
		return nil, fmt.Errorf("couldn't read .gitconfig: %s", err)
	}
	var config gitconfig.Config
	if err := gitconfig.NewDecoder(f).Decode(&config); err != nil {
		return nil, fmt.Errorf("couldn't parse .gitconfig: %s", err)
	}
	ghSection := config.Section("github")
	if ghSection.Option("user") != "" {
		user = ghSection.Option("user")
	}
	if ghSection.Option("token") == "" {
		return nil, errors.New("token not found in .gitconfig github section")
	}
	return &githttp.BasicAuth{
		Username: user,
		Password: ghSection.Option("token"),
	}, nil
}

func (a *Authenticator) getSSHMethod(ep *transport.Endpoint) (transport.AuthMethod, error) {
	agentAuth, err := gitssh.NewSSHAgentAuth("git")
	if err == nil {
		return agentAuth, nil
	}
	keyAuth, err := gitssh.NewPublicKeysFromFile("git", filepath.Join(a.osUser.HomeDir, ".ssh", "id_rsa"), "")
	if err == nil {
		return keyAuth, nil
	}
	return nil, fmt.Errorf("neither ssh agent nor ssh keyfile auth could be built: %s", err)
}
