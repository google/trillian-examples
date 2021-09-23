// Copyright 2021 Google LLC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// git contains libraries for using github repositories that make serverless
// operations easy to follow.
package git

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/util"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/golang/glog"
	"github.com/google/go-github/v37/github"
	"golang.org/x/oauth2"
)

// RepoID identifies a github project, including owner &repo.
type RepoID struct {
	Owner    string
	RepoName string
}

// String returns "<owner>/<repo>".
func (r RepoID) String() string {
	return fmt.Sprintf("%s/%s", r.Owner, r.RepoName)
}

// NewRepoID creates a new RepoID struct from an owner/repo fragment.
func NewRepoID(or string) (RepoID, error) {
	s := strings.Split(or, "/")
	if l, want := len(s), 2; l != want {
		return RepoID{}, fmt.Errorf("can't split owner/repo %q, found %d parts want %d", or, l, want)
	}
	return RepoID{
		Owner:    s[0],
		RepoName: s[1],
	}, nil
}

// NewServerlessRepo creates a wrapper around a git repository which has a fork owned by
// the user, and an upstream repository configured that PRs can be proposed against.
func NewServerlessRepo(ctx context.Context, upstream, fork RepoID, ghUser, ghEmail, ghToken, clonePath string) (ForkedRepo, error) {
	repo := ForkedRepo{
		upstream: upstream,
		fork:     fork,
		user:     ghUser,
		email:    ghEmail,
	}
	var err error
	repo.ghCli, err = authWithGithub(ctx, ghToken)
	if err != nil {
		return repo, err
	}

	// git clone -o origin "https://${GIT_USERNAME}:${FEEDER_GITHUB_TOKEN}@github.com/${fork_repo}.git" "${temp}"
	cloneOpts := &git.CloneOptions{
		URL: fmt.Sprintf("https://%v:%v@github.com/%s.git", ghUser, ghToken, repo.fork),
	}
	if clonePath == "" {
		repo.git, err = git.Clone(memory.NewStorage(), memfs.New(), cloneOpts)
	} else {
		repo.git, err = git.PlainClone(clonePath, false, cloneOpts)
	}
	if err != nil {
		return repo, fmt.Errorf("failed to clone fork repo %q: %v", cloneOpts.URL, err)
	}

	// Create a remote -> upstreamRepo
	//  git remote add upstream "https://github.com/${upstream_repo}.git"
	upstreamURL := fmt.Sprintf("https://github.com/%v.git", upstream)
	upstreamRemote, err := repo.git.CreateRemote(&config.RemoteConfig{
		Name: "upstream",
		URLs: []string{upstreamURL},
	})
	if err != nil {
		return repo, fmt.Errorf("failed to add remote %q to fork repo: %v", upstreamURL, err)
	}
	glog.V(1).Infof("Added remote upstream->%q for %q:\n%v", upstreamURL, repo.fork, upstreamRemote)

	if err = repo.git.FetchContext(ctx, &git.FetchOptions{}); err != nil && err != git.NoErrAlreadyUpToDate {
		return repo, fmt.Errorf("failed to fetch repo %q: %v", repo.fork, err)
	}
	return repo, err
}

// authWithGithub returns a github client struct which uses the provided OAuth token.
// These tokens can be created using the github -> Settings -> Developers -> Personal Authentication Tokens page.
func authWithGithub(ctx context.Context, ghToken string) (*github.Client, error) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: ghToken})
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc), nil
}

// ForkedRepo represents a github repository that has been forked.
// The original repo allows PRs, and the fork is owned by the user and commits
// are pushed to branches here and proposed against the upstream repository.
type ForkedRepo struct {
	// upstream is the original repository, and fork is the user's clone of it.
	// Changes will be made and pushed to fork, and PRs proposed to upstream.
	upstream, fork RepoID
	user, email    string
	git            *git.Repository
	ghCli          *github.Client
}

// PullAndGetHead ensures that the local files match those in the upstream repository,
// and returns a reference to the latest commit.
func (r *ForkedRepo) PullAndGetHead() (*plumbing.Reference, error) {
	wt, err := r.git.Worktree()
	if err != nil {
		return nil, fmt.Errorf("workTree(%v) err: %v", r, err)
	}
	// Pull the latest commits from remote 'upstream'.
	if err := wt.Pull(&git.PullOptions{RemoteName: "upstream"}); err != nil && err != git.NoErrAlreadyUpToDate {
		return nil, fmt.Errorf("git pull %v upstream err: %v", r, err)
	}
	// Get the master HEAD - we'll need this to branch from later on.
	headRef, err := r.git.Head()
	if err != nil {
		return nil, fmt.Errorf("reading %v HEAD ref err: %v", r, err)
	}
	return headRef, nil
}

// Returns a function which will delete the local branch when called.
func (r *ForkedRepo) CreateLocalBranch(headRef *plumbing.Reference, branchName string) (func(), error) {
	// Create a git branch for the witnessed checkpoint to be added, named
	// using hex(checkpoint hash).  Construct a fully-specified
	// reference name for the new branch.
	branchRefName := plumbing.ReferenceName(branchName)
	branchHashRef := plumbing.NewHashReference(branchRefName, headRef.Hash())

	workTree, err := r.git.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get work tree: %v", err)
	}

	glog.V(1).Infof("git checkout -b %q", branchHashRef.Name())
	if err := workTree.Checkout(&git.CheckoutOptions{
		Branch: branchRefName,
		Create: true,
	}); err != nil {
		return nil, fmt.Errorf("git checkout %v: %v", branchRefName, err)
	}
	d := func() {
		if err := r.git.Storer.RemoveReference(branchHashRef.Name()); err != nil {
			glog.Errorf("failed to delete branch hashref for %q: %v", branchHashRef.Name(), err)
		}
		glog.V(1).Infof("git branch -D %v", branchHashRef.Name())
		if err := workTree.Checkout(&git.CheckoutOptions{
			Branch: "refs/heads/master",
		}); err != nil {
			glog.Errorf("failed to git checkout master: %v", err)
			return
		}
		glog.V(1).Info("git checkout master")
	}

	return d, nil
}

// ReadFile behaves as `util.ReadFile` on the active branch.
// Encapsulating this inside the repo avoids clients needing to depend on
// git filesystems directly.
func (r *ForkedRepo) ReadFile(path string) ([]byte, error) {
	workTree, err := r.git.Worktree()
	if err != nil {
		return nil, fmt.Errorf("workTree(%v) err: %v", r, err)
	}
	return util.ReadFile(workTree.Filesystem, path)
}

// CommitFile creates a commit on a repo's worktree which overwrites the specified file path
// with the provided bytes.
func (r *ForkedRepo) CommitFile(path string, raw []byte, commitMsg string) error {
	workTree, err := r.git.Worktree()
	if err != nil {
		return fmt.Errorf("workTree(%v) err: %v", r, err)
	}
	glog.Infof("Writing checkpoint (%d bytes) to %q", len(raw), path)
	if err := util.WriteFile(workTree.Filesystem, path, raw, 0644); err != nil {
		return fmt.Errorf("failed to write to %q: %v", path, err)
	}

	glog.V(1).Infof("git add %s", path)
	if _, err := workTree.Add(path); err != nil {
		return fmt.Errorf("failed to git add %q: %v", path, err)
	}

	glog.V(1).Info("git commit")
	if _, err := workTree.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			// Name, Email required despite what we set up in git config earlier.
			Name:  r.user,
			Email: r.email,
			When:  time.Now(),
		},
	}); err != nil {
		return fmt.Errorf("git commit: %v", err)
	}
	return nil
}

// Push forces any local commits on the active branch to the user's fork.
func (r *ForkedRepo) Push() error {
	glog.V(1).Infof("git push -f")
	if err := r.git.Push(&git.PushOptions{
		Force: true,
	}); err != nil {
		return fmt.Errorf("git push -f: %v", err)
	}
	return nil
}

// CreatePR creates a pull request.
// Based on: https://godoc.org/github.com/google/go-github/github#example-PullRequestsService-Create
func (r *ForkedRepo) CreatePR(ctx context.Context, title, commitBranch, prBranch string) error {
	if title == "" {
		return errors.New("missing `title`, won't create PR")
	}

	newPR := &github.NewPullRequest{
		Title:               github.String(title),
		Head:                github.String(r.fork.Owner + ":" + commitBranch),
		Base:                github.String(prBranch),
		MaintainerCanModify: github.Bool(true),
	}

	prJSON, err := json.Marshal(newPR)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON for new PR request: %v", err)
	}
	glog.V(1).Infof("Creating PR:\n%s", prJSON)

	pr, _, err := r.ghCli.PullRequests.Create(ctx, r.upstream.Owner, r.upstream.RepoName, newPR)
	if err != nil {
		return err
	}

	glog.Infof("PR created: %s", pr.GetHTMLURL())
	return nil
}
