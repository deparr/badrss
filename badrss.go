package main

import (
	"bytes"
	"container/heap"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

const userAgent string = "badrss/0.0 (https://github.com/deparr/badrss)"

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
	Id      string `json:"id"`
	Title   string `json:"title"`
	Updated int64  `json:"updated"`
}

func (entry BlogEntry) String() string {
	return fmt.Sprintf("{%s (%s) @ %d}", entry.Title, entry.Id, entry.Updated)
}

type LocalFeeds struct {
	Fetched int64       `json:"fetched"`
	Feeds   []*BlogFeed `json:"feeds"`
}

func (lf LocalFeeds) getById(id string) *BlogFeed {
	for _, feed := range lf.Feeds {
		if feed.Id == id {
			return feed
		}
	}
	return nil
}

type BlogFeed struct {
	Url     string       `json:"url"`
	Spec    feedSpec     `json:"-"`
	Raw     []byte       `json:"-"`
	Title   string       `json:"title"`
	Id      string       `json:"id"`
	Entries []*BlogEntry `json:"entries"`
}

func (bf *BlogFeed) hasPost(target *BlogEntry) bool {
	for _, post := range bf.Entries {
		if post.Id == target.Id {
			// todo this is strange
			return target.Updated <= post.Updated
		}
	}
	return false
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

func readBlogRoll(path string) ([]*BlogFeed, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	feeds := make([]*BlogFeed, 0, 10)
	for line := range strings.Lines(string(raw)) {
		trimmed := strings.TrimSpace(line)
		feed := &BlogFeed{
			Url: trimmed,
			Id:  trimmed,
		}

		feeds = append(feeds, feed)
	}

	return feeds, nil
}

func fetchRemote(feeds []*BlogFeed) {
	for _, feed := range feeds {
		req, err := http.NewRequest("GET", feed.Url, nil)
		if err != nil {
			slog.Error("making request", "err", err, "url", feed.Url)
			continue
		}
		req.Header.Set("User-Agent", userAgent)

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			slog.Error("fetching", "err", err, "url", feed.Url)
			continue
		}
		rawXml, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			slog.Error("reading body", "err", err, "url", feed.Url)
		} else {
			feed.Raw = rawXml
		}
	}
}

func diffFeeds(localFeeds LocalFeeds, remoteFeeds []*BlogFeed) ([]*BlogFeed, int) {
	var updatedFeeds []*BlogFeed = nil
	numNewPosts := 0
	for _, remote := range remoteFeeds {
		local := localFeeds.getById(remote.Id)
		if local != nil {
			var newPosts []*BlogEntry = nil

			for _, post := range remote.Entries {
				if !local.hasPost(post) {
					newPosts = append(newPosts, post)
				}
			}

			if newPosts != nil {
				updatedFeeds = append(updatedFeeds, &BlogFeed{
					Entries: newPosts,
					Title:   remote.Title,
					Url:     remote.Url,
				})
				numNewPosts += 1
			}
		} else {
			// whole feed is new
			updatedFeeds = append(updatedFeeds, remote)
			numNewPosts += len(remote.Entries)
		}
	}

	return updatedFeeds, numNewPosts
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
	const MAX_STORED_ENTRIES = 5
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
							post.Updated = max(ts.Unix(), post.Updated)
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
		slog.Error("xml decode", "err", decodeErr)
	}
}

func trimmed(b []byte) string {
	return strings.TrimSpace(string(b))
}
