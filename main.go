package main

import (
	"crypto/tls"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/adrg/frontmatter"
	"github.com/robfig/cron/v3"
	"github.com/xanzy/go-gitlab"
)

var (
	ciAPIV4URL         string = ""
	gitlabAPIToken     string = ""
	ciProjectID        string = ""
	ciProjectDir       string = ""
	ciJobName          string = ""
	issuesRelativePath string = ".gitlab/recurring_issue_templates/"
)

type metadata struct {
	Title        string   `yaml:"title"`
	Confidential bool     `yaml:"confidential"`
	Assignees    []string `yaml:"assignees,flow"`
	Labels       []string `yaml:"labels,flow"`
	DueIn        string   `yaml:"duein"`
	Crontab      string   `yaml:"crontab"`
	NextTime     time.Time
}

func processIssueFile(lastTime time.Time) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Fatal(err)
		}

		if filepath.Ext(path) != ".md" {
			log.Println(path, "does not end in .md, skipping file")
			return nil
		}

		contents, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}

		var mdata metadata
		data, err := frontmatter.Parse(
			strings.NewReader(string(contents)),
			&mdata,
		)

		if err != nil {
			return err
		}

		cronExpr, err := cron.ParseStandard(mdata.Crontab)
		if err != nil {
			return err
		}

		mdata.NextTime = cronExpr.Next(lastTime)

		if mdata.NextTime.Before(time.Now()) {
			log.Println(path, "was due", mdata.NextTime.Format(time.RFC3339), "- creating new issue")

			err := createIssue(&mdata, data)
			if err != nil {
				return err
			}
		} else {
			log.Println(path, "is due", mdata.NextTime.Format(time.RFC3339))
		}

		return nil
	}
}

func createIssue(mdata *metadata, data []byte) error {
	transCfg := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{
		Transport: transCfg,
	}

	git, err := gitlab.NewClient(gitlabAPIToken, gitlab.WithBaseURL(ciAPIV4URL), gitlab.WithHTTPClient(httpClient))
	if err != nil {
		return err
	}

	project, _, err := git.Projects.GetProject(ciProjectID, nil)
	if err != nil {
		return err
	}

	options := &gitlab.CreateIssueOptions{
		Title:        gitlab.String(mdata.Title),
		Description:  gitlab.String(string(data)),
		Confidential: &mdata.Confidential,
		CreatedAt:    &mdata.NextTime,
	}

	if mdata.DueIn != "" {
		duration, err := time.ParseDuration(mdata.DueIn)
		if err != nil {
			return err
		}

		dueDate := gitlab.ISOTime(mdata.NextTime.Add(duration))

		options.DueDate = &dueDate
	}

	_, _, err = git.Issues.CreateIssue(project.ID, options)
	if err != nil {
		return err
	}

	return nil
}

func getLastRunTime() (time.Time, error) {
	transCfg := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{
		Transport: transCfg,
	}

	git, err := gitlab.NewClient(gitlabAPIToken, gitlab.WithBaseURL(ciAPIV4URL), gitlab.WithHTTPClient(httpClient))
	if err != nil {
		return time.Unix(0, 0), err
	}

	options := &gitlab.ListProjectPipelinesOptions{
		Scope:   gitlab.String("finished"),
		Status:  gitlab.BuildState(gitlab.Success),
		OrderBy: gitlab.String("updated_at"),
	}

	pipelineInfos, _, err := git.Pipelines.ListProjectPipelines(ciProjectID, options)
	if err != nil {
		return time.Unix(0, 0), err
	}

	for _, pipelineInfo := range pipelineInfos {
		jobs, _, err := git.Jobs.ListPipelineJobs(ciProjectID, pipelineInfo.ID, nil)
		if err != nil {
			return time.Unix(0, 0), err
		}

		for _, job := range jobs {
			if job.Name == ciJobName {
				return *job.FinishedAt, nil
			}
		}
	}

	return time.Unix(0, 0), nil
}

func main() {
	gitlabAPIToken = os.Getenv("GITLAB_API_TOKEN")
	if gitlabAPIToken == "" {
		log.Fatal("Environment variable 'GITLAB_API_TOKEN' not found. Ensure this is set under the project CI/CD settings.")
	}

	ciAPIV4URL = os.Getenv("CI_API_V4_URL")
	if ciAPIV4URL == "" {
		log.Fatal("Environment variable 'CI_API_V4_URL' not found. This tool must be ran as part of a GitLab pipeline.")
	}

	ciProjectID = os.Getenv("CI_PROJECT_ID")
	if gitlabAPIToken == "" {
		log.Fatal("Environment variable 'CI_PROJECT_ID' not found. This tool must be ran as part of a GitLab pipeline.")
	}

	ciProjectDir = os.Getenv("CI_PROJECT_DIR")
	if gitlabAPIToken == "" {
		log.Fatal("Environment variable 'CI_PROJECT_DIR' not found. This tool must be ran as part of a GitLab pipeline.")
	}

	ciJobName = os.Getenv("CI_JOB_NAME")
	if gitlabAPIToken == "" {
		log.Fatal("Environment variable 'CI_JOB_NAME' not found. This tool must be ran as part of a GitLab pipeline.")
	}

	issuesRelativePath = path.Join(ciProjectDir, issuesRelativePath)

	lastRunTime, err := getLastRunTime()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Last run:", lastRunTime.Format(time.RFC3339))

	err = filepath.Walk(issuesRelativePath, processIssueFile(lastRunTime))
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Run complete")
}
