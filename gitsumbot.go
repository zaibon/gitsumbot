package gitsumbot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-github/v42/github"
	"golang.org/x/sync/errgroup"

	"github.com/sashabaranov/go-openai"
	"golang.org/x/oauth2"
)

// truncate the commit message to this size
const maxMsgSize = 500

var ErrNoNewChanges = fmt.Errorf("no new changes in the repository")

type GitSumBot struct {
	gh *github.Client
	ai *openai.Client

	modelVersion ModelVersion
}

type ChangeDigest struct {
	Summary     string
	Categorized string
}

type ModelVersion string

const (
	ModelVersionGPT3 ModelVersion = openai.GPT3Dot5Turbo
	ModelVersionGPT4 ModelVersion = openai.GPT4
)

var ModelVersions = []ModelVersion{
	ModelVersionGPT3,
	ModelVersionGPT4,
}

func New(githubToken, openAIToken string, mv ModelVersion) *GitSumBot {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)

	return &GitSumBot{
		gh:           github.NewClient(tc),
		ai:           openai.NewClient(openAIToken),
		modelVersion: mv,
	}
}

// ChangeDigest gather the commit message of the repository identified by owner/name over duration period
// and generate a summary of all the commit messages.
func (b *GitSumBot) ChangeDigest(ctx context.Context, owner, name string, duration time.Duration) (ChangeDigest, error) {
	messages, err := b.getCommitMessages(ctx, owner, name, duration)
	if err != nil {
		return ChangeDigest{}, fmt.Errorf("error while fetching Github commit messages: %w", err)
	}

	if len(messages) == 0 {
		return ChangeDigest{}, ErrNoNewChanges
	}

	g, ctx := errgroup.WithContext(ctx)
	var (
		summary     string
		categorized string
	)

	g.Go(func() error {
		s, err := b.summarize(ctx, messages)
		if err != nil {
			return fmt.Errorf("error while generating summary: %w", err)
		}
		summary = s
		return nil
	})

	g.Go(func() error {
		c, err := b.dedupAndGroup(ctx, messages)
		if err != nil {
			return fmt.Errorf("error while categorizing messages: %w", err)
		}
		categorized = c
		return nil
	})

	if err := g.Wait(); err != nil {
		return ChangeDigest{}, err
	}

	return ChangeDigest{
		Summary:     summary,
		Categorized: categorized,
	}, nil
}

// summarize generate a paragraph explaining in a few sentence the changes made on the code based on the commit messages
func (b *GitSumBot) summarize(ctx context.Context, messages []string) (string, error) {
	prompt := `Write summary of the code change using the commit message send by the user.
	The commit message start with the type of change follow by the category on which the changes applied. 
	when generating the summary, do not list each commit one by one instead generate a continuous text that explain the changes. feel free to aggregate the same type of change into a shorter sentence.`

	exampleMessages := []string{
		"fix(data): drop views (#7031)",
		"feat(api): check if event gorm queries contain question mark (#7034)\n\nThis should fail now but later after I merge from main it should be\r\nfixed",
		"fix(webapp): improve access token handling (#7032)\n\nreuse access token from storage only for e2e tests.\r\nthis should fix the account linking problems.\r\nalso make it possible to accept invitation while being authenticated.\r\n\r\nrelated issues: #6790, #6788",
		"fix(data): remove question mark (#7033)",
		"fix(data): migrate to git hooks events v2 (#7030)",
		"fix(infra): fix migrations (#7026)\n\nThis PR fixes the migration script.",
		"fix(webapp): fix content download event (#7028)\n\nthis fixes the issue when download file from google drive wasn't\r\ntriggering content download event",
		"fix(infra): configure exponential backoff on cvs event subscription (#7027)",
		"build(deps): bump github.com/go-playground/validator/v10 from 10.11.2 to 10.12.0 in /go (#7022)\n\nBumps\r\n[github.com/go-playground/validator/v10](https://github.com/go-playground/validator)\r\nfrom 10.11.2 to 10.12.0.\r\n\u003cdetails\u003e\r\n\u003csummary\u003eRelease notes\u003c/summary\u003e\r\n\u003cp\u003e\u003cem\u003eSourced from \u003ca\r\nhref=\"https://github.com/go-playground/validator/releases\"\u003egithub.com/go-playground/validator/v10's\r\nreleases\u003c/a\u003e.\u003c/em\u003e\u003c/p\u003e\r\n\u003cblockquote\u003e\r\n\u003ch2\u003eRelease 10.12.0\u003c/h2\u003e\r\n\u003ch2\u003eWhat is new?\u003c/h2\u003e\r\n\u003cul\u003e\r\n\u003cli\u003eAdded \u003ccode\u003eeth_a",
		"build(deps): bump github.com/goccy/go-json from 0.10.1 to 0.10.2 in /go (#7020)\n\nBumps [github.com/goccy/go-json](https://github.com/goccy/go-json) from\r\n0.10.1 to 0.10.2.\r\n\u003cdetails\u003e\r\n\u003csummary\u003eRelease notes\u003c/summary\u003e\r\n\u003cp\u003e\u003cem\u003eSourced from \u003ca\r\nhref=\"https://github.com/goccy/go-json/releases\"\u003egithub.com/goccy/go-json's\r\nreleases\u003c/a\u003e.\u003c/em\u003e\u003c/p\u003e\r\n\u003cblockquote\u003e\r\n\u003ch2\u003e0.10.2\u003c/h2\u003e\r\n\u003ch2\u003eWhat's Changed\u003c/h2\u003e\r\n\u003cul\u003e\r\n\u003cli\u003eUpdate CI by \u003ca\r\nhref=\"https://github.com/goccy\"\u003e\u003ccode\u003e@​goccy\u003c/code\u003e\u003c/a\u003e in \u003ca\r\nhref=\"ht",
		"build(deps): bump github.com/go-git/go-git/v5 from 5.6.0 to 5.6.1 in /go (#7021)\n\nBumps [github.com/go-git/go-git/v5](https://github.com/go-git/go-git)\r\nfrom 5.6.0 to 5.6.1.\r\n\u003cdetails\u003e\r\n\u003csummary\u003eRelease notes\u003c/summary\u003e\r\n\u003cp\u003e\u003cem\u003eSourced from \u003ca\r\nhref=\"https://github.com/go-git/go-git/releases\"\u003egithub.com/go-git/go-git/v5's\r\nreleases\u003c/a\u003e.\u003c/em\u003e\u003c/p\u003e\r\n\u003cblockquote\u003e\r\n\u003ch2\u003ev5.6.1\u003c/h2\u003e\r\n\u003ch2\u003eWhat's Changed\u003c/h2\u003e\r\n\u003cul\u003e\r\n\u003cli\u003eplumbing/transport: don't use the \u003ccode\u003efirstErrLine\u003c/code\u003e when it\r\nis empty by \u003ca\r\nh",
		"fix(data): ch migrations (#7019)\n\nThe changes were already applied manually to each node.",
		"fix(webapp): webapp improvements (#7009)\n\n- profile page improvements\r\n- new identify event\r\n- few other ui fixes\r\n\r\nrelated issues: #6972, #6973, #6767\r\n\r\nprofile page for digest reviewer:\r\n\r\n![image](https://user-images.githubusercontent.com/4420081/226299423-f736b519-f42d-45fe-9f0c-546d5133d0e1.png)\r\n\r\nprofile dropdown menu:\r\n\r\n![image](https://user-images.githubusercontent.com/4420081/226377388-f20409e1-df59-429e-8c8c-eefa61f3e3d8.png)\r\n\r\nmain menu with no org selected:\r\n\r\n![image](https://u",
	}
	exampleSummary := `In the recent series of code changes, various fixes and improvements have been made to the application. The API now includes a check for event GORM queries containing a question mark, and several fixes have been applied to the data layer, including dropping views, removing question marks, and migrating to git hooks events v2. Additionally, changes have been made to the webapp, such as improving access token handling, fixing the content download event, and making various UI fixes and profile page improvements. The infrastructure has also seen fixes, including migration script and CVS event subscription adjustments.

	Moreover, several dependencies have been updated, including bumping github.com/go-playground/validator/v10, github.com/goccy/go-json, and github.com/go-git/go-git/v5 to newer versions, ensuring the application stays up-to-date and secure.`

	resp, err := b.ai.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Temperature:      0.6,
			TopP:             1,
			FrequencyPenalty: 0,
			PresencePenalty:  0,
			Model:            string(b.modelVersion),
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: prompt},
				{Role: openai.ChatMessageRoleUser, Content: strings.Join(exampleMessages, "\n\n")},
				{Role: openai.ChatMessageRoleAssistant, Content: exampleSummary},
				{Role: openai.ChatMessageRoleUser, Content: fmt.Sprintf("Here are the commit messages: \n %s", strings.Join(messages, "\n\n"))},
			},
		},
	)

	if err != nil {
		return "", err
	}

	return resp.Choices[0].Message.Content, nil
}

// releaseNote generate a release note based on the commit messages
func (b *GitSumBot) releaseNote(ctx context.Context, messages []string) (string, error) {
	prompt := `I will provide you with a list of git commit message. 
	your role is to create a user friendly release notes based on the git commit messages.
	Organize the release notes in sections: new features, fixes, chores, contributors.
	if possible add a section about new contributors.
	When mentioning users, use their github username and prefix it with @.
	dependabot is not a contributor`

	resp, err := b.ai.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Temperature:      0.6,
			TopP:             1,
			FrequencyPenalty: 0,
			PresencePenalty:  0,
			Model:            string(b.modelVersion),
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: prompt},
				{Role: openai.ChatMessageRoleUser, Content: fmt.Sprintf("Here are the commit messages: \n %s", strings.Join(messages, "\n\n"))},
			},
		},
	)

	if err != nil {
		return "", err
	}

	return resp.Choices[0].Message.Content, nil
}

// dedupAndGroup will generate a bullet point list of commit messages group by category
func (b *GitSumBot) dedupAndGroup(ctx context.Context, messages []string) (string, error) {
	prompt := `Group the relates commit messages into categories and print a list of the messages inside each category`

	resp, err := b.ai.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Temperature:      0.1,
			TopP:             1,
			FrequencyPenalty: 0,
			PresencePenalty:  0,
			Model:            string(b.modelVersion),
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: prompt},
				{Role: openai.ChatMessageRoleUser, Content: strings.Join(messages, "\n")},
			},
		},
	)

	if err != nil {
		return "", err
	}

	return resp.Choices[0].Message.Content, nil
}

func (b *GitSumBot) getCommitMessages(ctx context.Context, owner, repo string, duration time.Duration) ([]string, error) {
	r, _, err := b.gh.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	yesterday := now.Add(-duration)
	// todo: pagination

	commits, _, err := b.gh.Repositories.ListCommits(ctx, owner, repo, &github.CommitsListOptions{
		SHA:   *r.DefaultBranch,
		Since: yesterday,
		Until: now,
	})
	if err != nil {
		return nil, err
	}

	messages := make([]string, 0, len(commits))
	for _, c := range commits {
		msg := c.GetCommit().GetMessage()
		if len(msg) > maxMsgSize {
			msg = msg[:maxMsgSize]
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

func (b *GitSumBot) getCommitsBetween(ctx context.Context, owner, repo, base, head string) ([]string, error) {
	// todo: pagination
	compare, _, err := b.gh.Repositories.CompareCommits(ctx, owner, repo, base, head, &github.ListOptions{})
	if err != nil {
		return nil, err
	}

	messages := make([]string, 0, len(compare.Commits))
	for _, c := range compare.Commits {
		msg := c.GetCommit().GetMessage()
		if len(msg) > maxMsgSize {
			msg = msg[:maxMsgSize]
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

func (b *GitSumBot) tagBefore(ctx context.Context, owner, repo, tag string) (string, error) {
	// todo: pagination
	tags, _, err := b.gh.Repositories.ListTags(ctx, owner, repo, &github.ListOptions{})
	if err != nil {
		return "", err
	}
	for i, t := range tags {
		if t.GetName() == tag {
			if i == len(tags)-1 {
				return "", fmt.Errorf("tag %s is the first tag of the repository", tag)
			}
			return tags[i+1].GetName(), nil
		}
	}
	return "", fmt.Errorf("tag %s not found", tag)
}
