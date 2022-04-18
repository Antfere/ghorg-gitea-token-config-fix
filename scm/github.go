package scm

import (
	"context"
	"net/url"
	"os"
	"strings"

	"github.com/gabrie30/ghorg/colorlog"
	"github.com/google/go-github/v41/github"
	"golang.org/x/oauth2"
)

var (
	_ Client = Github{}
)

func init() {
	registerClient(Github{})
}

type Github struct {
	// extend the github client
	*github.Client
	// perPage contain the pagination item limit
	perPage int
}

func (_ Github) GetType() string {
	return "github"
}

// GetOrgRepos gets org repos
func (c Github) GetOrgRepos(targetOrg string) ([]Repo, error) {

	opt := &github.RepositoryListByOrgOptions{
		Type:        "all",
		ListOptions: github.ListOptions{PerPage: c.perPage},
	}

	// get all pages of results
	var allRepos []*github.Repository
	for {

		repos, resp, err := c.Repositories.ListByOrg(context.Background(), targetOrg, opt)

		if err != nil {
			return nil, err
		}
		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}

		if opt.Page%12 == 0 && opt.Page != 0 {
			colorlog.PrintSubtleInfo("\nEverything is okay, the org just has a lot of repos...")
		}
		opt.Page = resp.NextPage
	}

	return c.filter(allRepos), nil
}

// GetUserRepos gets user repos
func (c Github) GetUserRepos(targetUser string) ([]Repo, error) {

	userToken, _, _ := c.Users.Get(context.Background(), "")

	if targetUser == userToken.GetLogin() {
		colorlog.PrintSubtleInfo("\nCloning all your public/private repos. This process may take a bit longer than other clones, please be patient...")
		targetUser = ""
	}

	if os.Getenv("GHORG_SCM_BASE_URL") != "" {
		c.BaseURL, _ = url.Parse(os.Getenv("GHORG_SCM_BASE_URL"))
	}

	opt := &github.RepositoryListOptions{
		Visibility:  "all",
		ListOptions: github.ListOptions{PerPage: c.perPage},
	}

	// get all pages of results
	var allRepos []*github.Repository

	for {

		// List the repositories for a user. Passing the empty string will list repositories for the authenticated user.
		repos, resp, err := c.Repositories.List(context.Background(), targetUser, opt)

		if err != nil {
			return nil, err
		}

		if targetUser == "" {
			userRepos := []*github.Repository{}

			for _, repo := range repos {
				if *repo.Owner.Type == "User" {
					userRepos = append(userRepos, repo)
				}
			}

			repos = userRepos
		}

		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return c.filter(allRepos), nil
}

// NewClient create new github scm client
func (_ Github) NewClient() (Client, error) {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GHORG_GITHUB_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)

	baseURL := os.Getenv("GHORG_SCM_BASE_URL")
	var c *github.Client

	if baseURL != "" {
		c, _ = github.NewEnterpriseClient(baseURL, baseURL, tc)
	} else {
		c = github.NewClient(tc)
	}

	client := Github{Client: c, perPage: 100}

	return client, nil
}

func (_ Github) addTokenToHTTPSCloneURL(url string, token string) string {
	splitURL := strings.Split(url, "https://")
	return "https://" + token + "@" + splitURL[1]
}

func (c Github) filter(allRepos []*github.Repository) []Repo {
	var repoData []Repo

	for _, ghRepo := range allRepos {

		if os.Getenv("GHORG_SKIP_ARCHIVED") == "true" {
			if *ghRepo.Archived {
				continue
			}
		}

		if os.Getenv("GHORG_SKIP_FORKS") == "true" {
			if *ghRepo.Fork {
				continue
			}
		}

		if !hasMatchingTopic(ghRepo.Topics) {
			continue
		}

		r := Repo{}

		r.Name = *ghRepo.Name

		if os.Getenv("GHORG_BRANCH") == "" {
			defaultBranch := ghRepo.GetDefaultBranch()
			if defaultBranch == "" {
				defaultBranch = "master"
			}
			r.CloneBranch = defaultBranch
		} else {
			r.CloneBranch = os.Getenv("GHORG_BRANCH")
		}

		if os.Getenv("GHORG_CLONE_PROTOCOL") == "https" {
			r.CloneURL = c.addTokenToHTTPSCloneURL(*ghRepo.CloneURL, os.Getenv("GHORG_GITHUB_TOKEN"))
			r.URL = *ghRepo.CloneURL
			repoData = append(repoData, r)
		} else {
			r.CloneURL = *ghRepo.SSHURL
			r.URL = *ghRepo.SSHURL
			repoData = append(repoData, r)
		}

		if ghRepo.GetHasWiki() && os.Getenv("GHORG_CLONE_WIKI") == "true" {
			wiki := Repo{}
			wiki.IsWiki = true
			wiki.CloneURL = strings.Replace(r.CloneURL, ".git", ".wiki.git", 1)
			wiki.URL = strings.Replace(r.URL, ".git", ".wiki.git", 1)
			wiki.CloneBranch = "master"
			repoData = append(repoData, wiki)
		}
	}

	return repoData
}
