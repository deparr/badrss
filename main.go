package main

import (
	"encoding/json"
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
	blogRoll    string
	feedCache   string
	notifyCache string
	noNotify    bool
	command     string
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
	notifyFile := cache + "/badrss/notify"

	res := Options{}

	flag.StringVar(&res.blogRoll, "blogroll", blogRoll, "where to find the blogroll file")
	flag.StringVar(&res.feedCache, "feed-cache", localData, "where to store the local feed record")
	flag.StringVar(&res.notifyCache, "notify-cache", notifyFile, "where to store the notification file")
	flag.BoolVar(&res.noNotify, "no-notify", false, "do not notify after fetching")

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
		err = os.Remove(options.notifyCache)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "error removing notify file(%s): %s", options.notifyCache, err)
		}

		return
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
		if numNewPosts > 0 {
			notifyFile, err := os.Create(options.notifyCache)
			if err != nil {
				fatal("unable to open notifyFile", err)
			}

			notifyFile.WriteString(fmt.Sprintf("%d new posts\n", numNewPosts))
			builder := strings.Builder{}
			for _, feed := range newPosts {
				builder.Reset()
				builder.WriteString(fmt.Sprintf("[%s]", feed.Title))
				builder.WriteByte('\n')
				for _, post := range feed.Entries {
					builder.WriteString(fmt.Sprintf("%s\n", post.Title))
				}
				builder.WriteByte('\n')

				notifyFile.WriteString(builder.String())
			}

			notifyFile.Close()
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

		if options.noNotify {
			return
		}

		fallthrough
	case "notify":
		postBytes, err := os.ReadFile(options.notifyCache)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return
			}
			fatal("reading notify file", err)
		}

		summary, body, _ := strings.Cut(string(postBytes), "\n")
		err = notifySend(summary, body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error sending notif: %s", err)
		}

		err = os.Remove(options.notifyCache)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error removing notify file(%s): %s", options.notifyCache, err)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: '%s'. Try '--help'.\n", options.command)
	}
}
