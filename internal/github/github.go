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

// github contains libraries for using github repositories that make serverless
// operations easy to follow.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/golang/glog"
	gh_api "github.com/google/go-github/v39/github"
	"golang.org/x/oauth2"
)

// RepoID identifies a github project, including owner & repo.
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

// NewRepository creates a wrapper around a git repository which has a fork owned by
// the user, and an upstream repository configured that PRs can be proposed against.
func NewRepository(ctx context.Context, upstream RepoID, upstreamBranch string, fork RepoID, ghUser, ghEmail, ghToken string) (Repository, error) {
	repo := Repository{
		upstream:       upstream,
		upstreamBranch: upstreamBranch,
		fork:           fork,
		user:           ghUser,
		email:          ghEmail,
	}
	var err error
	repo.ghCli, err = authWithGithub(ctx, ghToken)
	if err != nil {
		return repo, err
	}
	return repo, nil
}

// authWithGithub returns a github client struct which uses the provided OAuth token.
// These tokens can be created using the github -> Settings -> Developers -> Personal Authentication Tokens page.
func authWithGithub(ctx context.Context, ghToken string) (*gh_api.Client, error) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: ghToken})
	tc := oauth2.NewClient(ctx, ts)
	return gh_api.NewClient(tc), nil
}

// Repository represents a github repository with a working area and an upstream.
// The upstream repo must allow PRs. The working fork is usually owned by the user;
// commits are pushed to branches here and proposed against the upstream repository.
type Repository struct {
	// upstream is the original repository, and fork is the user's clone of it.
	// Changes will be made and pushed to fork, and PRs proposed to upstream.
	upstream, fork RepoID
	// upstreamBranch is the name of the upstreamBranch/main branch that PRs will be proposed against.
	upstreamBranch string
	user, email    string
	ghCli          *gh_api.Client
}

func (r Repository) String() string {
	return fmt.Sprintf("%s → %s", r.fork, r.upstream)
}

// CreateOrUpdateBranch attempts to create a new branch on the fork repo if it doesn't already exist, or
// rebase it onto HEAD if it does.
func (r *Repository) CreateOrUpdateBranch(ctx context.Context, branchName string) error {
	baseRef, _, err := r.ghCli.Git.GetRef(ctx, r.upstream.Owner, r.upstream.RepoName, "refs/heads/"+r.upstreamBranch)
	if err != nil {
		return fmt.Errorf("failed to get %s ref: %v", r.upstreamBranch, err)
	}
	branch := "refs/heads/" + branchName
	newRef := &gh_api.Reference{Ref: gh_api.String(branch), Object: baseRef.Object}

	if _, rsp, err := r.ghCli.Git.GetRef(ctx, r.fork.Owner, r.fork.RepoName, branch); err != nil {
		if rsp == nil || rsp.StatusCode != 404 {
			return fmt.Errorf("failed to check for existing branch %q: %v", branchName, err)
		}
		// Branch doesn't exist, so we'll create it:
		_, _, err = r.ghCli.Git.CreateRef(ctx, r.fork.Owner, r.fork.RepoName, newRef)
		return err
	}
	// The branch already exists, so we'll update it:
	_, _, err = r.ghCli.Git.UpdateRef(ctx, r.fork.Owner, r.fork.RepoName, newRef, true)
	return err
}

// ReadFile returns the contents of the specified path from the configured upstream repo.
func (r *Repository) ReadFile(ctx context.Context, path string) ([]byte, error) {
	f, _, resp, err := r.ghCli.Repositories.GetContents(ctx, r.upstream.Owner, r.upstream.RepoName, path, nil)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("failed to GetContents(%q): %v", path, err)
	}
	s, err := f.GetContent()
	return []byte(s), err
}

// CommitFile creates a commit on a repo's worktree which overwrites the specified file path
// with the provided bytes.
func (r *Repository) CommitFile(ctx context.Context, path string, raw []byte, branch, commitMsg string) error {
	opts := &gh_api.RepositoryContentFileOptions{
		Message: gh_api.String(commitMsg),
		Content: raw,
		Branch:  gh_api.String(branch),
	}
	cRsp, _, err := r.ghCli.Repositories.CreateFile(ctx, r.fork.Owner, r.fork.RepoName, path, opts)
	if err != nil {
		return fmt.Errorf("failed to CreateFile(%q): %v", path, err)
	}
	glog.V(2).Infof("Commit %s updated %q on %s", cRsp.Commit, path, branch)
	return nil
}

// CreatePR creates a pull request.
// Based on: https://godoc.org/github.com/google/go-github/github#example-PullRequestsService-Create
func (r *Repository) CreatePR(ctx context.Context, title, commitBranch string) error {
	if title == "" {
		return errors.New("missing `title`, won't create PR")
	}

	newPR := &gh_api.NewPullRequest{
		Title:               gh_api.String(title),
		Head:                gh_api.String(r.fork.Owner + ":" + commitBranch),
		Base:                gh_api.String(r.upstreamBranch),
		MaintainerCanModify: gh_api.Bool(true),
	}

	prJSON, err := json.Marshal(newPR)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON for new PR request: %v", err)
	}
	glog.V(2).Infof("Creating PR:\n%s", prJSON)

	pr, _, err := r.ghCli.PullRequests.Create(ctx, r.upstream.Owner, r.upstream.RepoName, newPR)
	if err != nil {
		return err
	}

	glog.V(1).Infof("PR created: %q %s", title, pr.GetHTMLURL())
	return nil
}
