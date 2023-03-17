package gitsumbot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-github/v42/github"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/oauth2"
)

// truncate the commit message to this size
const maxMsgSize = 500

type GitSumBot struct {
	gh *github.Client
	ai *openai.Client
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
func (b *GitSumBot) ChangeDigest(ctx context.Context, owner, name string, duration time.Duration) (string, error) {
	messages, err := b.getCommitMessages(ctx, owner, name, duration)
	if err != nil {
		return "", fmt.Errorf("error while fetching Github commit messages: %w", err)
	}

	summary, err := b.summaries(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("error while generating summary: %w", err)
	}

	return summary, nil
}

func (b *GitSumBot) summaries(ctx context.Context, messages []string) (string, error) {
	prompt := `Your role is to create details summary of code changes added to a codebase using the commit messages given by the user.
	Starts your answer with the sentence: 'Based on the provided commit messages, here's the summary of changes:'
	Follow your answer with a detailed summary of all the changes in a few sentences.
	 Group the changes in one of these category: build ci,ci,docs,feat,fix,perf,refactor,revert,style,test.`

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
