package main

import (
	"crypto/tls"
	"net/http"

	"github.com/xanzy/go-gitlab"
)

func createGitlabClient() (*gitlab.Client, error) {
	transCfg := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{
		Transport: transCfg,
	}

	git, err := gitlab.NewClient(gitlabAPIToken, gitlab.WithBaseURL(ciAPIV4URL), gitlab.WithHTTPClient(httpClient))
	if err != nil {
		return nil, err
	}
	return git, nil
}
