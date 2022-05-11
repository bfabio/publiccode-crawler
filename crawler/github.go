package crawler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"net/url"
	"path"
	"strings"

	httpclient "github.com/italia/httpclient-lib-go"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/google/go-github/v43/github"
)

func githubBasicAuth(domain Domain) string {
	if len(domain.BasicAuth) > 0 {
		auth := domain.BasicAuth[rand.Intn(len(domain.BasicAuth))]
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
	}
	return ""
}

// RegisterGithubAPI register the crawler function for Github API.
// It get the list of repositories on "link" url.
// If a next page is available return its url.
// Otherwise returns an empty ("") string.
func RegisterGithubAPI() OrganizationHandler {
	return func(domain Domain, url url.URL, repositories chan Repository, publisher Publisher) (*url.URL, error) {
		// Set BasicAuth header
		headers := make(map[string]string)
		headers["Authorization"] = githubBasicAuth(domain)

		// Set domain host to new host.
		domain.Host = url.Hostname()

		client := github.NewClient(nil)
		repos, _, err := client.Repositories.ListByOrg(context.Background(), url.Path, nil)

		// Add repositories to the channel that will perform the check on everyone.
		for _, r := range repos {
			if *r.Private || *r.Archived {
				log.Warnf("Skipping %s: repo is private or archived", *r.FullName)
				continue
			}

			file, _, _, err := client.Repositories.GetContents(context.Background(), *r.Owner.Name, *r.Name, "publiccode.yml", nil)
			if file != nil {
				repositories <- Repository{
					Name:        *r.FullName,
					Hostname:    domain.Host,
					FileRawURL:  file.DownloadURL,
					GitCloneURL: *r.CloneURL,
					GitBranch:   *r.DefaultBranch,
					Domain:      domain,
					Publisher:   publisher,
					Headers:     headers,
					Metadata:    metadata,
				}
			}
		}

		// Return next url.
		nextLink := httpclient.HeaderLink(resp.Headers.Get("Link"), "next")

		// if last page for this organization, the nextLink is empty or equal to actual link.
		if nextLink == "" || nextLink == url.String() {
			return nil, nil
		}

		u, _ := url.Parse(nextLink)
		return u, nil
	}
}

// RegisterSingleGithubAPI register the crawler function for single repository Github API.
// Return nil if the repository was successfully added to repositories channel.
// Otherwise return the generated error.
func RegisterSingleGithubAPI() SingleRepoHandler {
	return func(domain Domain, url url.URL, repositories chan Repository, publisher Publisher) error {
		// Set BasicAuth header.
		headers := make(map[string]string)
		headers["Authorization"] = githubBasicAuth(domain)

		// Set domain host to new host.
		domain.Host = url.Hostname()

		url.Path = path.Join("repos", url.Path)
		url.Path = strings.Trim(url.Path, "/")
		url.Host = "api." + url.Host

		// Get List of repositories.
		resp, err := httpclient.GetURL(url.String(), headers)
		if err != nil {
			return err
		}
		if resp.Status.Code != http.StatusOK {
			log.Warnf("Request returned: %s", string(resp.Body))
			return errors.New("request returned an incorrect http.Status: " + resp.Status.Text)
		}

		var v GithubRepo
		err = json.Unmarshal(resp.Body, &v)
		if err != nil {
			return err
		}

		if v.Private || v.Archived {
			log.Warnf("Skipping %s: repo is private or archived", v.FullName)
			return errors.New("Skipping private or archived repo")
		}

		// Marshal all the repository metadata.
		metadata, err := json.Marshal(v)
		if err != nil {
			log.Errorf("github metadata: %v", err)
			return err
		}
		contents := strings.Replace(v.ContentsURL, "{+path}", "", -1)

		// Get List of files.
		resp, err = httpclient.GetURL(contents, headers)
		if err != nil {
			return err
		}
		if resp.Status.Code != http.StatusOK {
			log.Infof("Request returned an invalid status code: %s", string(resp.Body))
			return err
		}
		// Fill response as list of values (repositories data).
		var files GithubFiles
		err = json.Unmarshal(resp.Body, &files)
		if err != nil {
			log.Infof("Repository is empty: %s", url.String())
		}

		foundIt := false
		// Search a file with a valid name and a downloadURL.
		for _, f := range files {
			if f.Name == viper.GetString("CRAWLED_FILENAME") && f.DownloadURL != "" {
				// Add repository to channel.
				repositories <- Repository{
					Name:        v.FullName,
					Hostname:    url.Hostname(),
					FileRawURL:  f.DownloadURL,
					GitCloneURL: v.CloneURL,
					GitBranch:   v.DefaultBranch,
					Domain:      domain,
					Publisher:   publisher,
					Headers:     headers,
					Metadata:    metadata,
				}
				foundIt = true
			}
		}
		if !foundIt {
			return errors.New("Repository does not contain " + viper.GetString("CRAWLED_FILENAME"))
		}
		return nil
	}
}

// addGithubProjectsToRepositories adds the projects from api response to repository channel.
	// Search a file with a valid name and a downloadURL.
	for _, f := range files {
		if f.Name == viper.GetString("CRAWLED_FILENAME") && f.DownloadURL != "" {
			// Add repository to channel.
			repositories <- Repository{
				Name:        fullName,
				Hostname:    hostname,
				FileRawURL:  f.DownloadURL,
				GitCloneURL: cloneURL,
				GitBranch:   defaultBranch,
				Domain:      domain,
				Publisher:   publisher,
				Headers:     headers,
				Metadata:    metadata,
			}
		}
	}

	return nil
}

// GenerateGithubAPIURL returns the api url of given Gitlab organization link.
// IN: https://github.com/italia
// OUT:https://api.github.com/orgs/italia/repos,https://api.github.com/users/italia/repos
func GenerateGithubAPIURL() GeneratorAPIURL {
	return func(in url.URL) (out []url.URL, err error) {
		u := *&in
		u.Path = path.Join("orgs", u.Path, "repos")
		u.Path = strings.Trim(u.Path, "/")
		u.Host = "api." + u.Host
		out = append(out, u)

		u2 := *&in
		u2.Path = path.Join("users", u2.Path, "repos")
		u2.Path = strings.Trim(u2.Path, "/")
		u2.Host = "api." + u2.Host
		out = append(out, u2)

		return
	}
}

// IsGithub returns "true" if the url can use Github API.
func IsGithub(link string) bool {
	if len(link) == 0 {
		log.Errorf("IsGithub: empty link %s.", link)
		return false
	}

	u, err := url.Parse(link)
	if err != nil {
		log.Errorf("IsGithub: impossible to parse %s.", link)
		return false
	}
	u.Path = "rate_limit"
	u.Host = "api." + u.Host

	resp, err := httpclient.GetURL(u.String(), nil)
	if err != nil {
		log.Debugf("can %s use Github API? No.", link)
		return false
	}
	if resp.Status.Code != http.StatusOK {
		log.Debugf("can %s use Github API? No.", link)
		return false
	}

	log.Debugf("can %s use Github API? Yes.", link)
	return true
}
