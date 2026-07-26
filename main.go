package main

import (
	json "encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"net/url"
)

func fatal(msg string, err error) {
	fmt.Fprintf(os.Stderr, "error %s: %s", msg, err)
	os.Exit(1)
}

type Options struct {
	blogRoll  string
	feedCache string
	command   string
	quiet     bool
}

var options = Options{}

func parseArgs() Options {
	config, err := os.UserConfigDir()
	if err != nil {
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
	flag.BoolVar(&res.quiet, "quiet", false, "silence standard output")

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

		// todo probably shouldn't just craete a go routine for each one
		// oh well, probably wont be a problem, probably
		var wg sync.WaitGroup
		for _, feed := range feeds {
			wg.Go(func() {
				fetchFeed(feed)
				parseFeed(feed)
			})
		}
		wg.Wait()

		local, err := readFeedCache(options.feedCache)
		if err != nil {
			fatal("reading stored", err)
		}

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

			if !options.quiet && strings.Count(body, "\n") > 3 {
				fmt.Print(body)
			}
		}

		local = LocalFeeds{
			Fetched: time.Now().Unix(),
			Feeds:   feeds,
		}
		rawLocal, err := json.Marshal(local)
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
	case "list":
		blogroll, err := os.ReadFile(options.blogRoll)
		if err != nil {
			fatal("reading blogroll", err)
		}
		fmt.Printf("blogroll at '%s':\n", options.blogRoll)
		for line := range strings.Lines(string(blogroll)) {
			fmt.Printf(" %s", line)
		}
	case "cache":
		blogroll, err := readBlogRollForList(options.blogRoll)
		if err != nil {
			fatal("reading blogroll", err)
		}
		local, err := readFeedCache(options.feedCache)
		if err != nil {
			fatal("reading stored", err)
		}

		skipped := &strings.Builder{}
		used := &strings.Builder{}
		used.WriteString("Current feeds:\n")
		skipped.WriteString("Unused feeds:\n")

		targetLen := 0
		for i, blog := range blogroll {
			blogUrl, err := url.Parse(blog.url)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error parsing url for %s", blog.url)
				continue
			}
			shortUrl := blogUrl.Hostname()
			blogroll[i].url = shortUrl

			targetLen = max(targetLen, len(shortUrl))
		}

		targetLen += 2

		for _, blog := range blogroll {
			b := used
			if blog.skipped {
				b = skipped
			}

			fmt.Fprintf(b, "%s%s|", blog.url, strings.Repeat(" ", targetLen - len(blog.url)))
			cached := local.getByUrl(blog.id)
			if cached == nil {
				fmt.Fprintln(b, "")
				continue
			}

			fmt.Fprintf(b, " %d/%d |", len(cached.Entries), blog.limit)
			if (len(cached.Entries) > 0) {
				fmt.Fprintf(b, " %s", cached.Entries[0])
			}

			b.WriteByte('\n')
		}

		fmt.Printf("%s\n\n%s", used.String(), skipped.String())

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: '%s'. Try '--help'.\n", options.command)
	}
}
