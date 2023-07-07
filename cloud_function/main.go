package p

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/slack-go/slack"
	"github.com/zaibon/gitsumbot"
	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"
)

type cfg struct {
	GithubOwner       string
	GithubRepo        string
	GithubAccessToken string
	OpenAIAccessToken string
	GPTModelVersion   string
	SlackAccessToken  string
	SlackChannel      string
}

func cfgFromEnv() (*cfg, error) {
	c := &cfg{}
	c.GithubOwner = os.Getenv("github-owner")
	c.GithubRepo = os.Getenv("github-repo")
	c.GithubAccessToken = os.Getenv("github-token")
	c.OpenAIAccessToken = os.Getenv("openai-token")
	c.SlackAccessToken = os.Getenv("slack-token")
	c.SlackChannel = os.Getenv("slack-channel")

	c.GPTModelVersion = os.Getenv("model-version")
	if !slices.ContainsFunc(gitsumbot.ModelVersions, func(version gitsumbot.ModelVersion) bool {
		return version == gitsumbot.ModelVersion(c.GPTModelVersion)
	}) {
		return nil, fmt.Errorf("model %s not supported", c.GPTModelVersion)
	}

	return c, nil
}

func Run(w http.ResponseWriter, r *http.Request) {
	cfg, err := cfgFromEnv()
	if err != nil {
		slog.ErrorCtx(r.Context(), "error while loading config: %v", err)
		os.Exit(1)
	}

	bot := gitsumbot.New(cfg.GithubAccessToken, cfg.OpenAIAccessToken, gitsumbot.ModelVersion(cfg.GPTModelVersion))
	slackClient := slacker{
		client: slack.New(cfg.SlackAccessToken),
	}

	ctx := r.Context()
	cd, err := bot.ChangeDigest(ctx, cfg.GithubOwner, cfg.GithubRepo, time.Hour*24)
	if err != nil && err != gitsumbot.ErrNoNewChanges {
		slog.ErrorCtx(r.Context(), err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	today := time.Now().Format("02-01-2006")

	if err == gitsumbot.ErrNoNewChanges {
		msg := fmt.Sprintf("No new changes in the repository %s/%s for the date %v", cfg.GithubOwner, cfg.GithubRepo, today)
		if err := slackClient.SendChannel(ctx, cfg.SlackChannel, msg); err != nil {
			slog.ErrorCtx(r.Context(), err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "Summary of the change in the repository %s/%s for the date %v\n\n", cfg.GithubOwner, cfg.GithubRepo, today)
	fmt.Fprintln(buf, cd.Summary)
	fmt.Fprintf(buf, "\n\nDetailed list of of changes\n\n")
	fmt.Fprintln(buf, cd.Categorized)

	if err := slackClient.SendChannel(ctx, cfg.SlackChannel, buf.String()); err != nil {
		slog.ErrorCtx(r.Context(), err.Error())
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
