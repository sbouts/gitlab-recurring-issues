package main

import (
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ericaro/frontmatter"
	"github.com/gorhill/cronexpr"
	"github.com/xanzy/go-gitlab"
)

var (
	ciAPIV4URL             string = ""
	gitlabAPIToken         string = ""
	ciProjectID            string = ""
	ciProjectDir           string = ""
	ciJobName              string = ""
	ciProjectRootNamespace string = ""
	issuesRelativePath     string = ".gitlab/recurring_issue_templates/"
)

type metadata struct {
	Title        string   `yaml:"title"`
	Description  string   `fm:"content" yaml:"-"`
	Confidential bool     `yaml:"confidential"`
	Assignees    []string `yaml:"assignees,flow"`
	Labels       []string `yaml:"labels,flow"`
	DueIn        string   `yaml:"duein"`
	Crontab      string   `yaml:"crontab"`
	Epic         string   `yaml:"epic"`
	ProjectId    int      `yaml:"projectid"`
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

		data, err := parseMetadata(contents)
		if err != nil {
			return err
		}

		cronExpression, err := cronexpr.Parse(data.Crontab)
		if err != nil {
			return err
		}

		data.NextTime = cronExpression.Next(lastTime)

		if data.NextTime.Before(time.Now()) {
			log.Println(path, "was due", data.NextTime.Format(time.RFC3339), "- creating new issue")

			err := createIssue(data)
			if err != nil {
				return err
			}
		} else {
			log.Println(path, "is due", data.NextTime.Format(time.RFC3339))
		}

		return nil
	}
}

func parseMetadata(contents []byte) (*metadata, error) {
	data := new(metadata)
	err := frontmatter.Unmarshal(contents, data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func createIssue(data *metadata) error {
	git, err := createGitlabClient()
	if err != nil {
		return err
	}

	options := &gitlab.CreateIssueOptions{
		Title:        gitlab.String(data.Title),
		Description:  gitlab.String(data.Description),
		Confidential: &data.Confidential,
		CreatedAt:    &data.NextTime,
	}

	if data.DueIn != "" {
		duration, err := time.ParseDuration(data.DueIn)
		if err != nil {
			return err
		}

		dueDate := gitlab.ISOTime(data.NextTime.Add(duration))

		options.DueDate = &dueDate
	}

	issueProjectId, err := strconv.Atoi(ciProjectID)
	if err != nil {
		return err
	}

	if data.ProjectId != 0 {
		issueProjectId = data.ProjectId
	}

	newIssue, _, err := git.Issues.CreateIssue(issueProjectId, options)
	if err != nil {
		return err
	}

	if data.Epic != "" {
		groupId, err := getGroupIdFromNamespace()
		if err != nil {
			return err
		}

		epicId, err := getEpicId(groupId, data.Epic)
		if err != nil {
			return err
		}

		_, _, err = git.EpicIssues.AssignEpicIssue(groupId, epicId, newIssue.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func getGroupIdFromNamespace() (int, error) {
	git, err := createGitlabClient()
	if err != nil {
		return 0, err
	}

	options := &gitlab.ListGroupsOptions{
		Search:       &ciProjectRootNamespace,
		TopLevelOnly: gitlab.Bool(true),
		OrderBy:      gitlab.String("id"),
	}

	groups, _, err := git.Groups.ListGroups(options)
	if err != nil {
		return 0, err
	}

	if len(groups) != 1 {
		log.Fatal("Expected one group for namespace", ciProjectRootNamespace, "but found multiple")
	}

	return groups[0].ID, nil
}

func getEpicId(groupId int, epicName string) (int, error) {
	git, err := createGitlabClient()
	if err != nil {
		return 0, err
	}

	options := &gitlab.ListGroupEpicsOptions{
		Search:                  &epicName,
		IncludeDescendantGroups: gitlab.Bool(false),
	}

	epics, _, err := git.Epics.ListGroupEpics(groupId, options)
	if err != nil {
		return 0, err
	}

	if len(epics) != 1 {
		log.Fatal("Expected one epic for epicName", epicName, "but found multiple")
	}

	return epics[0].ID, nil
}

func getLastRunTime() (time.Time, error) {
	git, err := createGitlabClient()

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
	if ciProjectID == "" {
		log.Fatal("Environment variable 'CI_PROJECT_ID' not found. This tool must be ran as part of a GitLab pipeline.")
	}

	ciProjectDir = os.Getenv("CI_PROJECT_DIR")
	if ciProjectDir == "" {
		log.Fatal("Environment variable 'CI_PROJECT_DIR' not found. This tool must be ran as part of a GitLab pipeline.")
	}

	ciJobName = os.Getenv("CI_JOB_NAME")
	if ciJobName == "" {
		log.Fatal("Environment variable 'CI_JOB_NAME' not found. This tool must be ran as part of a GitLab pipeline.")
	}

	ciProjectRootNamespace = os.Getenv("CI_PROJECT_ROOT_NAMESPACE")
	if ciProjectRootNamespace == "" {
		log.Fatal("Environment variable 'CI_PROJECT_ROOT_NAMESPACE' not found. This tool must be ran as part of a GitLab pipeline.")
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
