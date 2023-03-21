package p

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/slack-go/slack"
	"github.com/zaibon/gitsumbot"
)

type cfg struct {
	GithubOwner       string
	GithubRepo        string
	GithubAccessToken string
	OpenAIAccessToken string
	SlackAccessToken  string
	SlackChannel      string
}

func cfgFromEnv() *cfg {
	c := &cfg{}
	c.GithubOwner = os.Getenv("github-owner")
	c.GithubRepo = os.Getenv("github-repo")
	c.GithubAccessToken = os.Getenv("github-token")
	c.OpenAIAccessToken = os.Getenv("openai-token")
	c.SlackAccessToken = os.Getenv("slack-token")
	c.SlackChannel = os.Getenv("slack-channel")
	return c
}

func Run(w http.ResponseWriter, r *http.Request) {
	cfg := cfgFromEnv()
	bot := gitsumbot.New(cfg.GithubAccessToken, cfg.OpenAIAccessToken)
	slackClient := slacker{
		client: slack.New(cfg.SlackAccessToken),
	}

	ctx := r.Context()
	cd, err := bot.ChangeDigest(ctx, cfg.GithubOwner, cfg.GithubRepo, time.Hour*24)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	today := time.Now().Format("02-01-2006")

	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "Summary of the change in the repository %s/%s for the date %v\n\n", cfg.GithubOwner, cfg.GithubRepo, today)
	fmt.Fprintln(buf, cd.Summary)
	fmt.Fprintf(buf, "\n\nDetailed list of of changes\n\n")
	fmt.Fprintln(buf, cd.Categorized)

	if err := slackClient.SendChannel(ctx, cfg.SlackChannel, buf.String()); err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

type slacker struct {
	client *slack.Client
}

func (a *slacker) findChannelIDByName(ctx context.Context, channel string) (string, error) {
	channels, _, err := a.client.GetConversationsContext(ctx, &slack.GetConversationsParameters{
		Limit: 1000,
	})
	if err != nil {
		return "", err
	}
	for _, c := range channels {
		if c.Name == channel {
			return c.ID, nil
		}
	}
	return "", fmt.Errorf("channel not found: %q", channel)
}

func (a *slacker) SendChannel(ctx context.Context, channel string, msg string) error {
	channelID, err := a.findChannelIDByName(ctx, channel)
	if err != nil {
		return err
	}
	_, _, err = a.client.PostMessageContext(ctx, channelID, slack.MsgOptionText(
		msg,
		false,
	))
	return err
}
