<!-- How about "GitSumBot"? It's a pun on "get some bot", which implies that the AI is helping you get a summary of the code changes you need. Plus, "Git" is a reference to the popular version control system used by developers. -->

# GitSumBot

GitSumBot is a Go program that generates a summary of changes made to a codebase using commit messages.

## Installation

To install GitSumBot, you need to have Go 1.16 or later installed on your system. You can then install GitSumBot by running:

```shell
go get github.com/zaibon/gitsumbot
```

## Usage

### API

GitSumBot provides an API that you can use to generate summaries of code changes programmatically. To use the API, you need to import the gitsumbot package and create a new instance of the GitSumBot struct:

```go
import "github.com/sashabaranov/gitsumbot"

func main() {
    var(
        githubOwner       ="zaibon"
        githubRepo        ="gitsumbot"
        githubAccessToken = "..." // Github token with scope to read the repository you want to summarize
        openAIAccessToken = "..." // openAI API token
    )

    bot := gitsumbot.New(githubAccessToken, openAIAccessToken)
    summary, err := bot.ChangeDigest(ctx, githubOwner, githubRepo, time.Hour * 24 * 7)
    if err != nil {
        log.Fatalf("error while generating summary: %v", err)
    }

    fmt.Println(summary)
}
```

## Fun fact

This AI assistant has named itself. Here is the reasoning behind the name.

`It's a pun on "get some bot", which implies that the AI is helping you get a summary of the code changes you need. Plus, "Git" is a reference to the popular version control system used by developers.`
