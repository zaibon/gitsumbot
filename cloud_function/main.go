package p

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/zaibon/gitsumbot"
)

type cfg struct {
	GithubOwner       string
	GithubRepo        string
	GithubAccessToken string
	OpenAIAccessToken string
}

func cfgFromEnv() *cfg {
	c := &cfg{}
	c.GithubOwner = os.Getenv("github-owner")
	c.GithubRepo = os.Getenv("github-repo")
	c.GithubAccessToken = os.Getenv("github-token")
	c.OpenAIAccessToken = os.Getenv("openai-token")
	return c
}

func Run(w http.ResponseWriter, r *http.Request) {
	cfg := cfgFromEnv()
	bot := gitsumbot.New(cfg.GithubAccessToken, cfg.OpenAIAccessToken)

	ctx := r.Context()
	summary, err := bot.ChangeDigest(ctx, cfg.GithubOwner, cfg.GithubRepo, time.Hour*24)
	if err != nil {
		log.Fatalln(err)
	}

	w.WriteHeader(http.StatusOK)
	today := time.Now().Format("02-01-2006")
	fmt.Fprintf(w, "Summary of the change in the repository %s/%s for the date %v\n", cfg.GithubOwner, cfg.GithubRepo, today)
	fmt.Fprintln(w, summary)
}
