# for local storage

- sqlite
- json, probably wont be that big

10 - 20 blogs keeping the 10 most recent items => 100 - 200 items, json is fine

# for atom feeds

- look for `feed`
- match `id` with blogroll entry
- for each `entry`
  - match `updated` and `id` against local
  - grab `title`

# for rss feeds

- look for rss tag
- look for channel tag
- match `link` with blogroll entry as `id`
- `lastBuildDate` ??
- for each `item`
  - match `pubDate` and `guid` with local
  - grab `title`

above is tedious, shoulddecode directly to structs instead
