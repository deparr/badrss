package main

import (
	json "encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func fatal(msg string, err error) {
	fmt.Fprintf(os.Stderr, "error %s: %s", msg, err)
	os.Exit(1)
}

type Options struct {
	blogRoll  string
	feedCache string
	command   string
}

var options = Options{}

func parseArgs() Options {
	config, err := os.UserConfigDir()
	if err != nil {
		// todo these probably shouldn't exit
		fatal("reading config dir", err)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		fatal("reading cache dir", err)
	}

	blogRoll := config + "/badrss/blogroll"
	localData := cache + "/badrss/feeds.json"

	res := Options{}

	flag.StringVar(&res.blogRoll, "blogroll", blogRoll, "where to find the blogroll file")
	flag.StringVar(&res.feedCache, "feed-cache", localData, "where to store the local feed record")

	flag.Parse()

	res.command = flag.Arg(0)

	return res
}

func main() {

	options = parseArgs()

	switch options.command {
	case "clean":
		err := os.Remove(options.feedCache)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "error removing cache file(%s): %s", options.feedCache, err)
		}
	case "":
		fallthrough
	case "fetch":
		feeds, err := readBlogRoll(options.blogRoll)
		if err != nil {
			fatal("reading blogroll", err)
		}

		fetchRemote(feeds)

		for _, feed := range feeds {
			parseFeed(feed)
		}

		rawLocal, err := os.ReadFile(options.feedCache)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			fatal("reading stored", err)
		}
		local := LocalFeeds{}
		json.Unmarshal(rawLocal, &local)

		newPosts, numNewPosts := diffFeeds(local, feeds)

		// var proc *exec.Cmd = nil
		if numNewPosts > 0 {
			summary := fmt.Sprintf("%d new posts", numNewPosts)
			builder := strings.Builder{}
			for _, feed := range newPosts {
				builder.WriteString(fmt.Sprintf("[%s]", feed.Title))
				builder.WriteByte('\n')
				for _, post := range feed.Entries {
					builder.WriteString(fmt.Sprintf("%s\n", post.Title))
				}
				builder.WriteByte('\n')
			}

			body := builder.String()
			_, err = notifySend(summary, body)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error notifying: %s", err)
			}
		}

		local = LocalFeeds{
			Fetched: time.Now().Unix(),
			Feeds:   feeds,
		}
		rawLocal, err = json.Marshal(local)
		if err != nil {
			fatal("marshalling local data", err)
		}
		os.WriteFile(options.feedCache, rawLocal, 0644)

		// if proc != nil {
		// 	err = proc.Wait()
		// 	if err != nil {
		// 		fmt.Fprintf(os.Stderr, "%s:\n\n%s", proc.String(), err)
		// 	}
		// }
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: '%s'. Try '--help'.\n", options.command)
	}
}
