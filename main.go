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
}

type Options struct {
	blogRoll   string
	localData  string
	notifyFile string
	command    string
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
	flag.StringVar(&res.localData, "localdata", localData, "where to store the local feed record")
	flag.StringVar(&res.notifyFile, "notify", notifyFile, "where to store the notification file")

	flag.Parse()

	res.command = flag.Arg(0)

	return res
}

func main() {

	options = parseArgs()

	switch options.command {
	case "clean":
		err := os.Remove(options.localData)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "error removing cache file(%s): %s", options.localData, err)
		}
		err = os.Remove(options.notifyFile)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "error removing notify file(%s): %s", options.notifyFile, err)
		}

		return
	case "notify":
		postBytes, err := os.ReadFile(options.notifyFile)
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

		err = os.Remove(options.notifyFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error removing notify file(%s): %s", options.notifyFile, err)
		}
	case "fetch":
		fallthrough
	default:
		feeds, err := readBlogRoll(options.blogRoll)
		if err != nil {
			fatal("reading blogroll", err)
		}

		fetchRemote(feeds)

		for _, feed := range feeds {
			parseFeed(feed)
		}

		rawLocal, err := os.ReadFile(options.localData)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			fatal("reading stored", err)
		}
		local := LocalFeeds{}
		json.Unmarshal(rawLocal, &local)

		newPosts, numNewPosts := diffFeeds(local, feeds)
		if numNewPosts > 0 {
			notifyFile, err := os.Create(options.notifyFile)
			if err != nil {
				fatal("unable to open notifyFile", err)
			}
			defer notifyFile.Close()

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
		}

		local = LocalFeeds{
			Fetched: time.Now().Unix(),
			Feeds:   feeds,
		}
		rawLocal, err = json.Marshal(local)
		if err != nil {
			fatal("marshalling local data", err)
		}
		os.WriteFile(options.localData, rawLocal, 0644)
	}
}
