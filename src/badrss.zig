const std = @import("std");
const xml = @import("xml");
const zeit = @import("zeit");

const log = std.log;

/// this this is actually so bad
pub const FeedRecord = struct {
    fetched: i64,
    feeds: std.ArrayListUnmanaged(BlogFeed) = .empty,
    ids: std.ArrayListUnmanaged(u8) = .empty,
    urls: std.ArrayListUnmanaged(u8) = .empty,
    titles: std.ArrayListUnmanaged(u8) = .empty,

    pub fn init(arena: std.mem.Allocator) !FeedRecord {
        const now = zeit.instant(.{ .source = .now }) catch unreachable;

        var feed_record = FeedRecord{ .fetched = now.unixTimestamp() };
        try feed_record.ids.ensureTotalCapacityPrecise(arena, 4096);
        try feed_record.titles.ensureTotalCapacityPrecise(arena, 4096);
        try feed_record.urls.ensureTotalCapacityPrecise(arena, 2048);
        try feed_record.feeds.ensureTotalCapacityPrecise(arena, 10);

        return feed_record;
    }

    pub fn newBlogFeed(self: *FeedRecord, arena: std.mem.Allocator, id: []const u8, title: []const u8, url: []const u8, entries: std.ArrayListUnmanaged(BlogEntry)) !BlogFeed {
        // log.info("creating new feed{{ .id = '{s}', .title = '{s}', .url = '{s}' }}", .{ id, title, url });
        var feed = BlogFeed{};
        feed.id = if (self.ids.items.len == 0) 0 else @as(u32, @intCast(self.ids.items.len));
        self.ids.appendSliceAssumeCapacity(id);
        self.ids.appendAssumeCapacity(0);

        feed.title = if (self.titles.items.len == 0) 0 else @as(u32, @intCast(self.titles.items.len));
        self.titles.appendSliceAssumeCapacity(title);
        self.titles.appendAssumeCapacity(0);

        feed.url = if (self.urls.items.len == 0) 0 else @as(u32, @intCast(self.urls.items.len));
        self.urls.appendSliceAssumeCapacity(url);
        self.urls.appendAssumeCapacity(0);

        feed.entries = entries;
        try self.feeds.append(arena, feed);

        return feed;
    }

    pub fn appendBlogFeed(self: *FeedRecord, arena: std.mem.Allocator, other_record: *const FeedRecord, feed: BlogFeed) !void {
        var new_entries: std.ArrayListUnmanaged(BlogEntry) = .empty;
        const feed_id = other_record.getId(feed.id);
        const feed_title = other_record.getTitle(feed.title);
        const feed_url = other_record.getUrl(feed.url);

        for (feed.entries.items) |post| {
            try new_entries.append(arena, try self.newBlogEntry(
                other_record.getId(post.id),
                other_record.getTitle(post.title),
                post.updated,
            ));
        }

        _ = try self.newBlogFeed(arena, feed_id, feed_title, feed_url, new_entries);
    }

    pub fn newBlogEntry(self: *FeedRecord, id: []const u8, title: []const u8, updated: i64) !BlogEntry {
        // log.info("creating new entry{{ .id = '{s}', .title = '{s}', .updated = {d} }}", .{ id, title, updated });
        var post = BlogEntry{ .id = 0, .title = 0 };
        post.id = if (self.ids.items.len == 0) 0 else @as(u32, @intCast(self.ids.items.len));
        self.ids.appendSliceAssumeCapacity(id);
        self.ids.appendAssumeCapacity(0);

        post.title = if (self.titles.items.len == 0) 0 else @as(u32, @intCast(self.titles.items.len));
        self.titles.appendSliceAssumeCapacity(title);
        self.titles.appendAssumeCapacity(0);

        post.updated = updated;

        return post;
    }

    pub fn getId(self: *const FeedRecord, pos: u32) []const u8 {
        if (pos > self.ids.items.len - 1)
            return "STRING_TABLE_OVERFLOW";
        const null_term = std.mem.indexOfScalarPos(u8, self.ids.items, pos, 0).?;
        return self.ids.items[pos..null_term];
    }

    pub fn getTitle(self: *const FeedRecord, pos: u32) []const u8 {
        if (pos > self.titles.items.len - 1)
            return "STRING_TABLE_OVERFLOW";
        const null_term = std.mem.indexOfScalarPos(u8, self.titles.items, pos, 0).?;
        return self.titles.items[pos..null_term];
    }

    pub fn getUrl(self: *const FeedRecord, pos: u32) []const u8 {
        if (pos > self.urls.items.len - 1)
            return "STRING_TABLE_OVERFLOW";
        const null_term = std.mem.indexOfScalarPos(u8, self.urls.items, pos, 0).?;
        return self.urls.items[pos..null_term];
    }

    pub fn parseFeed(self: *FeedRecord, buf: []const u8, arena: std.mem.Allocator) !BlogFeed {
        const MAX_STORED_ENTRIES = 10;

        // var tag_stack: [10][]const u8 = undefined;
        // var tag_idx: usize = 0;
        var tag_buf: [32]u8 = undefined;
        var tag: []u8 = undefined;
        var fbs = std.io.fixedBufferStream(buf);
        var doc = xml.streamingDocument(arena, fbs.reader());
        var reader = doc.reader(arena, .{});
        var entries: std.ArrayListUnmanaged(BlogEntry) = .empty;
        var feed_id: ?[]const u8 = null;
        var feed_title: ?[]const u8 = null;
        var feed_url: ?[]const u8 = null;
        var post_id: ?[]const u8 = null;
        var post_title: ?[]const u8 = null;
        var post_updated: ?i64 = null;

        var feed_id_buf: [128]u8 = undefined;
        var feed_title_buf: [128]u8 = undefined;
        var feed_url_buf: [128]u8 = undefined;
        var post_id_buf: [128]u8 = undefined;
        var post_title_buf: [128]u8 = undefined;

        var context: enum {
            root,
            feed,
            entry,
        } = .root;
        var spec: enum { rss, atom } = .rss;
        var last_node: xml.Reader.Node = .eof;

        while (true) {
            const node = try reader.read();
            switch (node) {
                .eof => break,
                .element_start => {
                    const names = reader.elementNameNs();
                    @memcpy(tag_buf[0..names.local.len], names.local);
                    tag = tag_buf[0..names.local.len];
                    if (eql(tag, "rss")) {
                        context = .root;
                        spec = .rss;
                    } else if (eql(tag, "channel")) {
                        context = .feed;
                    } else if (eql(tag, "feed")) {
                        context = .feed;
                        spec = .atom;
                    } else if (eql(tag, "link") and context == .feed) {
                        // iterate over attrs find href
                        for (0..reader.reader.attributeCount()) |i| {
                            if (eql(reader.reader.attributeName(i), "href")) {
                                const ref = try reader.reader.attributeValue(i);
                                const trimmed = trim(ref);
                                @memcpy(feed_url_buf[0..trimmed.len], trimmed);
                                feed_url = feed_url_buf[0..trimmed.len];
                                break;
                            }
                        }
                    } else if (eql(tag, "item") or eql(tag, "entry")) {
                        context = .entry;
                    }

                    last_node = .element_start;
                },
                .element_end => {
                    const names = reader.elementNameNs();
                    if (eql(names.local, "item") or eql(names.local, "entry")) {
                        const id = post_id orelse return error.PostMissingId;
                        const title = post_title orelse return error.PostMissingTitle;
                        const updated = post_updated orelse return error.PostMissingUpdated;
                        try entries.append(arena, try self.newBlogEntry(id, title, updated));
                        post_id = null;
                        post_title = null;
                        post_updated = null;
                    }

                    if (entries.items.len >= MAX_STORED_ENTRIES)
                        break;

                    if (eql(tag, names.local))
                        tag = "";

                    last_node = .element_end;
                },
                .text => {
                    if (eql(tag, "title")) {
                        switch (last_node) {
                            .character_reference, .entity_reference => {
                                switch (context) {
                                    .feed => {
                                        const trimmed = try reader.text();
                                        const written = feed_title.?.len;
                                        @memcpy(feed_title_buf[written .. written + trimmed.len], trimmed);
                                        feed_title = feed_title_buf[0 .. written + trimmed.len];
                                    },
                                    .entry => {
                                        const trimmed = try reader.text();
                                        const written = post_title.?.len;
                                        @memcpy(post_title_buf[written .. written + trimmed.len], trimmed);
                                        post_title = post_title_buf[0 .. written + trimmed.len];
                                    },
                                    .root => {},
                                }
                            },
                            else => {
                                switch (context) {
                                    .feed => {
                                        const trimmed = try reader.text();
                                        @memcpy(feed_title_buf[0..trimmed.len], trimmed);
                                        feed_title = feed_title_buf[0..trimmed.len];
                                    },
                                    .entry => {
                                        const trimmed = try reader.text();
                                        @memcpy(post_title_buf[0..trimmed.len], trimmed);
                                        post_title = post_title_buf[0..trimmed.len];
                                    },
                                    .root => {},
                                }
                            },
                        }
                    } else if (eql(tag, "link") and context == .feed) {
                        const trimmed = trim(try reader.text());
                        @memcpy(feed_url_buf[0..trimmed.len], trimmed);
                        feed_url = feed_url_buf[0..trimmed.len];
                    } else if (eql(tag, "guid") or eql(tag, "id")) {
                        switch (context) {
                            .feed => {
                                const trimmed = trim(try reader.text());
                                @memcpy(feed_id_buf[0..trimmed.len], trimmed);
                                feed_id = feed_id_buf[0..trimmed.len];
                            },
                            .entry => {
                                const trimmed = trim(try reader.text());
                                @memcpy(post_id_buf[0..trimmed.len], trimmed);
                                post_id = post_id_buf[0..trimmed.len];
                            },
                            .root => {},
                        }
                    } else if (eql(tag, "pubDate") or eql(tag, "published") or eql(tag, "updated")) {
                        if (context == .entry) {
                            const dt_str = trim(try reader.text());
                            const source: zeit.Instant.Source = switch (spec) {
                                .rss => .{ .rfc2822 = dt_str },
                                .atom => .{ .rfc3339 = dt_str },
                            };
                            const ts = zeit.instant(.{ .source = source }) catch blk: {
                                // try a second time because rss is garbage
                                break :blk try zeit.instant(.{ .source = .{ .rfc1123 = dt_str } });
                            };
                            post_updated = ts.unixTimestamp();
                        }
                    }
                    last_node = .text;
                },
                .entity_reference => {
                    if (eql(tag, "title")) {
                        switch (context) {
                            .feed => {
                                feed_title_buf[feed_title.?.len] = unescapeChar(reader.entityReferenceName());
                                feed_title = feed_title_buf[0 .. feed_title.?.len + 1];
                            },
                            .entry => {
                                post_title_buf[post_title.?.len] = unescapeChar(reader.entityReferenceName());
                                post_title = post_title_buf[0 .. post_title.?.len + 1];
                            },
                            else => {},
                        }
                        last_node = .entity_reference;
                    }
                },
                .character_reference => {
                    if (eql(tag, "title")) {
                        switch (context) {
                            .feed => {
                                // screw unicode
                                feed_title_buf[feed_title.?.len] = @as(u8, @truncate(reader.characterReferenceChar()));
                                feed_title = feed_title_buf[0 .. feed_title.?.len + 1];
                            },
                            .entry => {
                                post_title_buf[post_title.?.len] = @as(u8, @truncate(reader.characterReferenceChar()));
                                post_title = post_title_buf[0 .. post_title.?.len + 1];
                            },
                            else => {},
                        }
                        last_node = .character_reference;
                    }
                },
                .xml_declaration, .comment, .cdata, .pi => {},
            }
        }

        const feed = try self.newBlogFeed(
            arena,
            feed_id orelse feed_url orelse return error.FeedNoIdOrUrl,
            feed_title orelse "[unknown]",
            feed_url orelse "[unknown]",
            entries,
        );

        // log.info("finished feed '{s}' ({s}) with {d} posts", .{ feed_title orelse "none", feed_url.?, entries.items.len});
        return feed;
    }

    /// `other` is assumed to be the 'older' record
    /// TODO this should probably return a RecordDiff type or something
    pub fn diffAgainst(self: *const FeedRecord, other: *const FeedRecord, allocator: std.mem.Allocator) !std.meta.Tuple(&.{ FeedRecord, usize }) {
        var result: FeedRecord = try .init(allocator);
        var num_new_posts: usize = 0;

        for (self.feeds.items) |feed| {
            const id = self.getId(feed.id);
            const other_feed = for (other.feeds.items) |other_feed| {
                if (eql(id, other.getId(other_feed.id)))
                    break other_feed;
            } else null;

            // old record is missing this entire feed
            if (other_feed == null) {
                _ = try result.appendBlogFeed(allocator, self, feed);
                num_new_posts += feed.entries.items.len;
                // check old record for missing posts
            } else {
                var new_entries: std.ArrayListUnmanaged(BlogEntry) = .empty;
                for (feed.entries.items) |post| {
                    const post_id = self.getId(post.id);
                    const other_post = for (other_feed.?.entries.items) |other_post| {
                        if (eql(post_id, other.getId(other_post.id)))
                            break other_post;
                    } else null;

                    if (other_post == null or other_post.?.updated < post.updated) {
                        const new_post = try result.newBlogEntry(
                            self.getId(post.id),
                            self.getTitle(post.title),
                            post.updated,
                        );
                        try new_entries.append(allocator, new_post);
                    }
                }

                if (new_entries.items.len > 0) {
                    _ = try result.newBlogFeed(
                        allocator,
                        self.getId(feed.id),
                        self.getTitle(feed.title),
                        self.getUrl(feed.url),
                        new_entries,
                    );
                    num_new_posts += new_entries.items.len;
                }
            }
        }

        return .{ result, num_new_posts };
    }

    /// dump buffer lens and caps to stderr
    fn dumpStats(self: *const FeedRecord) void {
        std.debug.print(
            \\------------------------
            \\ids len: {d} cap: {d}
            \\titles len: {d} cap: {d}
            \\urls len: {d} cap: {d}
            \\feeds len: {d} cap: {d}
            \\
        , .{
            self.ids.items.len,
            self.ids.capacity,
            self.titles.items.len,
            self.titles.capacity,
            self.urls.items.len,
            self.urls.capacity,
            self.feeds.items.len,
            self.feeds.capacity,
        });
    }

    pub fn jsonStringify(self: *const FeedRecord, jws: anytype) !void {
        try jws.beginObject();
        try jws.objectField("fetched");
        try jws.write(self.fetched);
        try jws.objectField("feeds");

        try jws.beginArray();
        for (self.feeds.items) |feed| {
            try jws.beginObject();

            try jws.objectField("url");
            try jws.write(self.getUrl(feed.url));
            try jws.objectField("title");
            try jws.write(self.getTitle(feed.title));
            try jws.objectField("id");
            try jws.write(self.getId(feed.id));

            try jws.objectField("entries");
            try jws.beginArray();
            for (feed.entries.items) |post| {
                try jws.beginObject();

                try jws.objectField("id");
                try jws.write(self.getId(post.id));
                try jws.objectField("title");
                try jws.write(self.getTitle(post.title));
                try jws.objectField("updated");
                try jws.write(post.updated);

                try jws.endObject();
            }
            try jws.endArray();

            try jws.endObject();
        }

        try jws.endArray();
        try jws.endObject();
    }

    pub fn jsonParse(allocator: std.mem.Allocator, source: anytype, options: std.json.ParseOptions) !FeedRecord {
        if (try source.next() != .object_begin) {
            return error.UnexpectedToken;
        }

        var fr: FeedRecord = try .init(allocator);

        try expectField(allocator, source, "fetched");
        _ = options;
        fr.fetched = try expectNumber(allocator, source, i64);

        try expectField(allocator, source, "feeds");
        try expectArray(source);
        var posts: std.ArrayListUnmanaged(BlogEntry) = .empty;
        feedLoop: while (true) {
            switch (try source.next()) {
                .object_begin => {
                    try expectField(allocator, source, "url");
                    const feed_url = try expectString(allocator, source);
                    try expectField(allocator, source, "title");
                    const feed_title = try expectString(allocator, source);
                    try expectField(allocator, source, "id");
                    const feed_id = try expectString(allocator, source);

                    try expectField(allocator, source, "entries");
                    try expectArray(source);

                    while (true) {
                        switch (try source.next()) {
                            .object_begin => {},
                            .array_end => break,
                            else => return error.UnexpectedToken,
                        }
                        try expectField(allocator, source, "id");
                        const id = try expectString(allocator, source);
                        try expectField(allocator, source, "title");
                        const title = try expectString(allocator, source);
                        try expectField(allocator, source, "updated");
                        const updated = try expectNumber(allocator, source, i64);

                        const post = try fr.newBlogEntry(id, title, updated);
                        try posts.append(allocator, post);

                        try expectObjectEnd(source);
                    }

                    try expectObjectEnd(source);

                    _ = try fr.newBlogFeed(allocator, feed_id, feed_title, feed_url, posts);
                    posts = .empty;
                },
                .array_end => break :feedLoop,
                else => return error.UnexpectedToken,
            }
        }
        try expectObjectEnd(source);

        return fr;
    }
};

fn expectField(allocator: std.mem.Allocator, source: anytype, expected: []const u8) !void {
    switch (try source.nextAlloc(allocator, .alloc_if_needed)) {
        .string, .allocated_string => |field| {
            if (!eql(field, expected)) return error.UnexpectedToken;
        },
        else => return error.UnexpectedToken,
    }
}

fn expectString(allocator: std.mem.Allocator, source: anytype) ![]const u8 {
    switch (try source.nextAlloc(allocator, .alloc_if_needed)) {
        .string, .allocated_string => |field| {
            return field;
        },
        else => return error.UnexpectedToken,
    }
}

fn expectNumber(allocator: std.mem.Allocator, source: anytype, int: type) !int {
    switch (try source.nextAlloc(allocator, .alloc_if_needed)) {
        .number, .allocated_number => |number| {
            return try std.fmt.parseInt(int, number, 0);
        },
        else => return error.UnexpectedToken,
    }
}

inline fn expectArray(source: anytype) !void {
    if (try source.next() != .array_begin) {
        return error.UnexpectedToken;
    }
}

inline fn expectArrayEnd(source: anytype) !void {
    if (try source.next() != .array_end) {
        return error.UnexpectedToken;
    }
}

inline fn expectObject(source: anytype) !void {
    if (try source.next() != .object_begin) {
        return error.UnexpectedToken;
    }
}

inline fn expectObjectEnd(source: anytype) !void {
    if (try source.next() != .object_end) {
        return error.UnexpectedToken;
    }
}

const BlogFeed = struct {
    id: u32 = 0,
    url: u32 = 0,
    title: u32 = 0,
    entries: std.ArrayListUnmanaged(BlogEntry) = .empty,
};

const BlogEntry = struct {
    id: u32 = 0,
    title: u32 = 0,
    updated: i64 = 0,
};

fn eql(a: []const u8, b: []const u8) bool {
    return std.mem.eql(u8, a, b);
}

fn trim(s: []const u8) []const u8 {
    return std.mem.trim(u8, s, &std.ascii.whitespace);
}

fn unescapeChar(char: []const u8) u8 {
    return if (eql(char, "lt"))
        '<'
    else if (eql(char, "gt"))
        '>'
    else if (eql(char, "amp"))
        '&'
    else if (eql(char, "quot"))
        '"'
    else if (eql(char, "apos"))
        '\''
    else
        ' ';
}
