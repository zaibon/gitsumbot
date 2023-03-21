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

type GitSumBot struct {
	gh *github.Client
	ai *openai.Client
}

type ChangeDigest struct {
	Summary     string
	Categorized string
}

func New(githubToken, openAIToken string) *GitSumBot {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)

	return &GitSumBot{
		gh: github.NewClient(tc),
		ai: openai.NewClient(openAIToken),
	}
}

// ChangeDigest gather the commit message of the repository identified by owner/name over duration period
// and generate a summary of all the commit messages.
func (b *GitSumBot) ChangeDigest(ctx context.Context, owner, name string, duration time.Duration) (ChangeDigest, error) {
	messages, err := b.getCommitMessages(ctx, owner, name, duration)
	if err != nil {
		return ChangeDigest{}, fmt.Errorf("error while fetching Github commit messages: %w", err)
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
	prompt := `The commit message start with the type of change follow by the category on which the changes applied.

	For example :
	"feat(api): add new endpoint"
	type: new feature
	category: API`

	resp, err := b.ai.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Temperature:      0.1,
			TopP:             1,
			FrequencyPenalty: 0,
			PresencePenalty:  0,
			Model:            openai.GPT3Dot5Turbo,
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
			Model:            openai.GPT3Dot5Turbo,
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
