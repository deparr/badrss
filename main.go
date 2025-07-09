package main

import (
	"bytes"
	"container/heap"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

type feedSpec int

const (
	rss feedSpec = iota
	atom
)

func (fs feedSpec) String() string {
	switch fs {
	case rss:
		return "rss"
	case atom:
		return "atom"
	default:
		return "unknown"
	}
}

type BlogEntry struct {
	Id      string
	Title   string
	Updated int64
}

func (entry BlogEntry) String() string {
	return fmt.Sprintf("{%s (%s) @ %d}", entry.Title, entry.Id, entry.Updated)
}

type BlogFeed struct {
	Url     string
	Spec    feedSpec
	Raw     []byte
	Title   string
	Id      string
	Entries []*BlogEntry
}

func (bf BlogFeed) String() string {
	b := strings.Builder{}
	b.WriteString(fmt.Sprintf("%s (%s)\n", bf.Title, bf.Id))
	b.WriteString("{\n")
	for _, entry := range bf.Entries {
		b.WriteString(fmt.Sprintf("\t%s\n", entry))
	}
	b.WriteString("}\n")

	return b.String()
}

func (bf BlogFeed) Len() int { return len(bf.Entries) }
func (bf BlogFeed) Less(i, j int) bool {
	return bf.Entries[i].Updated > bf.Entries[j].Updated
}

func (bf BlogFeed) Swap(i, j int) {
	bf.Entries[i], bf.Entries[j] = bf.Entries[j], bf.Entries[i]
}

func (bf *BlogFeed) Push(x any) {
	entry := x.(*BlogEntry)
	bf.Entries = append(bf.Entries, entry)
}

func (bf *BlogFeed) Pop() any {
	n := len(bf.Entries)

	entry := bf.Entries[n-1]
	bf.Entries[n-1] = nil
	bf.Entries = bf.Entries[0 : n-1]
	return entry
}

func (bf *BlogFeed) pushOrUpdate(entry *BlogEntry) {
	for i, e := range bf.Entries {
		if e.Id == entry.Id {
			if e.Updated < entry.Updated {
				e.Updated = entry.Updated
				heap.Fix(bf, i)
			}
			return
		}
	}

	heap.Push(bf, entry)
}

func readBlogRoll(path string) ([]BlogFeed, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	feeds := make([]BlogFeed, 0, 10)
	for line := range strings.Lines(string(raw)) {
		trimmed := strings.TrimSpace(line)
		feed := BlogFeed{
			Url: trimmed,
			Id:  trimmed,
		}

		feeds = append(feeds, feed)
	}

	return feeds, nil
}

var staticRoll = []string{
	"res/rockorager.xml",
	"res/matklad.xml",
	"res/openmymind.xml",
	"res/andrewrk.xml",
}

func blogRollStatic() ([]BlogFeed, error) {

	res := make([]BlogFeed, len(staticRoll))

	for i, path := range staticRoll {
		raw, err := os.ReadFile(path)
		if err != nil {
			fatal("reading static blogroll", err)
		}
		res[i] = BlogFeed{
			Raw: raw,
			Id:  path,
		}
	}

	return res, nil
}

type Options struct {
	blogRoll  string
	localData string
	command   string
}

func parseArgs() Options {
	config, err := os.UserConfigDir()
	if err != nil {
		fatal("reading config dir", err)
		os.Exit(1)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		fatal("reading cache dir", err)
		os.Exit(1)
	}
	blogRoll := config + "/badrss/blogroll"

	localData := cache + "/badrss_feeds.json"

	res := Options{}

	flag.StringVar(&res.blogRoll, "blogroll", blogRoll, "path to blogroll file")
	flag.StringVar(&res.localData, "localdata", localData, "full path where feed records are stored")

	flag.Parse()

	res.command = flag.Arg(0)

	return res
}

func fatal(msg string, err error) {
	fmt.Fprintf(os.Stderr, "error %s: %s", msg, err)
}

func main() {

	options := parseArgs()
	fmt.Println(options)
	feeds, err := readBlogRoll(options.blogRoll)
	if err != nil {
		fatal("reading blogroll", err)
	}

	// should read local and the blogroll config and then merge them
	for i, feed := range feeds {
		slog.Info("fetching", "url", feed.Url)
		res, err := http.Get(feed.Url)
		if err != nil {
			slog.Error("fetching", "err", err, "url", feed.Url)
			continue
		}
		defer res.Body.Close()
		rawXml, err := io.ReadAll(res.Body)
		if err != nil {
			slog.Error("reading body", "err", err, "url", feed.Url)
		} else {
			feeds[i].Raw = rawXml
		}
	}

	if err != nil {
		fmt.Println("error")
		return
	}

	for i := range feeds {
		parseFeed(&feeds[i])
	}

	for _, feed := range feeds {
		fmt.Println(feed)
	}
}

type feedContext int

const (
	feedp feedContext = iota
	rssp
	channel
	entry
	value
	noContext
)

// TODO decode directly into structs
func parseFeed(feed *BlogFeed) {
	const MAX_STORED_ENTRIES = 10
	dec := xml.NewDecoder(bytes.NewReader(feed.Raw))
	var (
		tagIdx   = -1
		tagStack = make([]string, 10)
		context  = noContext
	)

	var post *BlogEntry = nil
	untypedToken, decodeErr := dec.Token()
feedParse:
	for decodeErr == nil {
		switch token := untypedToken.(type) {
		case xml.StartElement:
			tagIdx += 1
			tagStack[tagIdx] = token.Name.Local
			switch tagStack[tagIdx] {
			case "rss":
				context = rssp
				feed.Spec = rss
			case "channel":
				context = channel
			case "feed":
				context = feedp
				feed.Spec = atom
			case "item":
				fallthrough
			case "entry":
				context = entry
				if post == nil {
					post = new(BlogEntry)
				}
			}
		case xml.EndElement:
			tagIdx = max(tagIdx-1, 0)
			if token.Name.Local == "item" || token.Name.Local == "entry" {
				// handle post
				feed.pushOrUpdate(post)
				post = nil
			}

			if len(feed.Entries) >= MAX_STORED_ENTRIES {
				break feedParse
			}
		case xml.CharData:
			if tagIdx >= 0 {
				switch tagStack[tagIdx] {
				case "title":
					if context == channel || context == feedp {
						feed.Title = trimmed(token)
					} else if context == entry {
						fmt.Println("got post title: ", trimmed(token))
						post.Title = trimmed(token)
					}
				case "link":
					if context == channel {
						feed.Id = trimmed(token)
					}
				case "guid":
					fallthrough
				case "id":
					if context == entry {
						post.Id = trimmed(token)
					} else if context == feedp {
						feed.Id = trimmed(token)
					}
				case "pubDate":
					fallthrough
				case "published":
					fallthrough
				case "updated":
					if context == entry {
						var timeFmt string
						if feed.Spec == rss {
							timeFmt = time.RFC1123
						} else {
							timeFmt = time.RFC3339
						}
						ts, err := time.Parse(timeFmt, trimmed(token))
						if err != nil {
							slog.Error("time parse", "err", err, "feed", feed.Id, "post", post.Title)
						} else {
							post.Updated = ts.Unix()
						}
					}
				}
			}
		case xml.Comment:
		case xml.ProcInst:
		case xml.Directive:
		default:
			slog.Warn("unexpected type in feedParse", "val", token)
		}

		untypedToken, decodeErr = dec.Token()
	}

	if decodeErr != nil && decodeErr != io.EOF {
		fmt.Println("got decode err", decodeErr)
	}
}

func trimmed(b []byte) string {
	return strings.TrimSpace(string(b))
}
