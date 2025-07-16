const std = @import("std");
const badrss = @import("badrss.zig");
const builtin = @import("builtin");

const log = std.log;

const usage =
    \\Usage
    \\  badrss [options] [command]
    \\
    \\  --help  print this message and exit
    \\
    \\ OPTIONS
    \\ --blogroll=PATH      Where to find the blogroll file
    \\ --feed-cache=PATH    Where to store the local feed record (defaults to <USER_CACHE_DIR>/badrss/feed.json)
    \\ --notify-cache=PATH  Where to store the notification file (defaults to <USER_CACHE_DIR>/badrss/notify)
    \\ --notify=BOOL        Whether or not to notify on fetch (default true). Ignored when command is not 'fetch'.
    \\
    \\ COMAMNDS
    \\ fetch                Fetch remote feeds listed in <blogroll> and update <feed-cache>. Notifies on new posts
    \\ notify               Check notification file and push a desktop notification if there are new posts
    \\ clean                Remove <feed-cache> and <notify-cache> files
;

const Options = struct {
    blogroll: []const u8 = "",
    @"feed-cache": []const u8 = "",
    @"notify-cache": []const u8 = "",
    notify: bool = true,
    command: Command = .fetch,
    allocator: std.mem.Allocator = undefined,

    const Command = enum {
        fetch,
        clean,
        notify,
    };
};

fn parseArgs(arena: std.mem.Allocator) !Options {
    var args = try std.process.argsWithAllocator(arena);

    var options: Options = .{};
    var found_positional = false;
    _ = args.next();
    const stderr = std.io.getStdErr().writer();
    while (args.next()) |arg| {
        switch (optKind(arg)) {
            .long => {
                var split = std.mem.splitScalar(u8, arg[2..], '=');
                const opt = split.first();
                const val = split.rest();

                if (eql(opt, "blogroll")) {
                    options.blogroll = val;
                } else if (eql(opt, "feed-cache")) {
                    options.@"feed-cache" = val;
                } else if (eql(opt, "notify-cache")) {
                    options.@"notify-cache" = val;
                } else if (eql(opt, "notify")) {
                    options.notify = parseArgBool(val) orelse true;
                } else if (eql(opt, "help")) {
                    try stderr.writeAll(usage);
                    std.process.exit(0);
                } else {
                    try stderr.print("invalid option: {s}", .{arg});
                    std.process.exit(1);
                }
            },
            .positional => {
                if (!found_positional)
                    options.command = std.meta.stringToEnum(Options.Command, arg) orelse .fetch;
                found_positional = true;
            },
            .short => {
                try stderr.print("invalid option: {s}", .{arg});
                std.process.exit(1);
            },
        }
    }

    const config_dir = blk: switch (builtin.target.os.tag) {
        .windows => try std.process.getEnvVarOwned(arena, "AppData"),
        .linux => {
            var dir = std.process.getEnvVarOwned(arena, "XDG_CONFIG_HOME") catch |err| switch (err) {
                .EnvironmentVariableNotFound => {},
                else => return err,
            };

            if (dir.len > 0) break :blk dir;

            dir = try std.process.getEnvVarOwned(arena, "HOME");
            dir = try std.fs.path.join(arena, &[_][]const u8{ dir, "/.config" });
            break :blk dir;
        },
        else => @compileError("unsupported os"),
    };

    const cache_dir = blk: switch (builtin.target.os.tag) {
        .windows => try std.process.getEnvVarOwned(arena, "LocalAppData"),
        .linux => {
            var dir = std.process.getEnvVarOwned(arena, "XDG_CACHE_HOME") catch |err| switch (err) {
                .EnvironmentVariableNotFound => {},
                else => return err,
            };

            if (dir.len > 0) break :blk dir;

            dir = try std.process.getEnvVarOwned(arena, "HOME");
            dir = try std.fs.path.join(arena, &[_][]const u8{ dir, "/.cache" });
            break :blk dir;
        },
        else => @compileError("unsupported os"),
    };

    const default_blogroll = "/badrss/blogroll";
    const default_feed_cache = "/badrss/feed.json";
    const default_notify_cache = "/badrss/notify";

    if (options.blogroll.len == 0)
        options.blogroll = try std.fs.path.join(arena, &[_][]const u8{ config_dir, default_blogroll });

    if (options.@"feed-cache".len == 0)
        options.@"feed-cache" = try std.fs.path.join(arena, &[_][]const u8{ cache_dir, default_feed_cache });

    if (options.@"notify-cache".len == 0)
        options.@"notify-cache" = try std.fs.path.join(arena, &[_][]const u8{ cache_dir, default_notify_cache });

    options.allocator = arena;
    return options;
}

fn eql(a: []const u8, b: []const u8) bool {
    return std.mem.eql(u8, a, b);
}

fn optKind(a: []const u8) enum { short, long, positional } {
    if (std.mem.startsWith(u8, a, "--")) return .long;
    if (std.mem.startsWith(u8, a, "-")) return .short;
    return .positional;
}

fn parseArgBool(arg: []const u8) ?bool {
    if (arg.len == 0) return true;

    if (std.ascii.eqlIgnoreCase(arg, "true")) return true;
    if (std.ascii.eqlIgnoreCase(arg, "1")) return true;
    if (std.ascii.eqlIgnoreCase(arg, "false")) return false;
    if (std.ascii.eqlIgnoreCase(arg, "0")) return false;

    return null;
}

pub fn main() !void {
    var debug_allocator: std.heap.DebugAllocator(.{}) = .init;
    const gpa, const is_debug = gpa: {
        break :gpa switch (builtin.mode) {
            .Debug, .ReleaseSafe => .{ debug_allocator.allocator(), true },
            .ReleaseFast, .ReleaseSmall => .{ std.heap.smp_allocator, false },
        };
    };
    defer if (is_debug) {
        _ = debug_allocator.deinit();
    };

    var arena = std.heap.ArenaAllocator.init(gpa);
    defer arena.deinit();

    const options = try parseArgs(arena.allocator());

    switch (options.command) {
        .fetch => {
            var blogroll_buf: [512]u8 = undefined;
            const blogroll = blk: {
                const blogroll = try std.fs.cwd().openFile(options.blogroll, .{});
                defer blogroll.close();
                const read = try blogroll.readAll(&blogroll_buf);
                break :blk blogroll_buf[0..read];
            };
            const remote_record = try fetchFeeds(options, blogroll);

            const local_record = blk: {
                const feed_cache = std.fs.cwd().openFile(options.@"feed-cache", .{}) catch |err| switch (err) {
                    error.FileNotFound => break :blk badrss.FeedRecord{ .fetched = 0 },
                    else => return err,
                };
                var reader = std.json.reader(options.allocator, feed_cache.reader());
                const parsed_record = try std.json.parseFromTokenSourceLeaky(
                    badrss.FeedRecord,
                    options.allocator,
                    &reader,
                    .{},
                );
                feed_cache.close();
                break :blk parsed_record;
            };

            const record_diff, const num_new_posts = try remote_record.diffAgainst(&local_record, options.allocator);
            if (num_new_posts > 0) {
                const line_delim = switch (builtin.target.os.tag) {
                    .windows => "`n",
                    .linux => "\n",
                    else => @compileError("dont"),
                };
                var notify_cache = try std.fs.cwd().createFile(options.@"notify-cache", .{ .truncate = true });
                const writer = notify_cache.writer();
                try writer.print("{d} new posts\n", .{num_new_posts});
                for (record_diff.feeds.items) |feed| {
                    try writer.print("[{s}]\n", .{record_diff.getTitle(feed.title)});
                    for (feed.entries.items) |post| {
                        try writer.writeAll(record_diff.getTitle(post.title));
                        try writer.writeAll(line_delim);
                    }
                    try writer.writeAll(line_delim);
                }

                notify_cache.close();
            }

            var feed_cache = try std.fs.cwd().createFile(
                options.@"feed-cache",
                .{ .truncate = true },
            );
            try std.json.stringify(remote_record, .{}, feed_cache.writer());
            feed_cache.close();

            if (options.notify)
                try notify(options);
        },
        .notify => try notify(options),
        .clean => try clean(options),
    }
}

fn fetchFeeds(options: Options, blogroll: []const u8) !badrss.FeedRecord {
    var record: badrss.FeedRecord = try .init(options.allocator);

    var client: std.http.Client = .{
        .allocator = options.allocator,
    };
    defer client.deinit();

    var res_buf: std.ArrayList(u8) = .init(options.allocator);
    var iter = std.mem.tokenizeScalar(u8, blogroll, '\n');
    while (iter.next()) |url| {
        log.info("fetching {s}", .{url});
        const res = try client.fetch(.{
            .method = .GET,
            .location = .{ .url = url },
            .response_storage = .{ .dynamic = &res_buf },
            .redirect_behavior = @enumFromInt(10),
        });

        if (res.status != .ok) {
            log.err("non OK fetching '{s}': {s}", .{ url, @tagName(res.status) });
            continue;
        }

        _ = try record.parseFeed(res_buf.items, options.allocator);
        // log.info("{s} has {d} posts", .{ record.getTitle(feed.title), feed.entries.items.len });

        res_buf.clearRetainingCapacity();
    }

    return record;
}

fn notify(options: Options) !void {
    var notify_cache_file = std.fs.cwd().openFile(options.@"notify-cache", .{}) catch |err| switch (err) {
        std.fs.File.OpenError.FileNotFound => return,
        else => return err,
    };

    var notify_cache_buf: [4096]u8 = undefined;
    const read = try notify_cache_file.readAll(&notify_cache_buf);
    const notify_cache = notify_cache_buf[0..read];

    const first_linebreak = std.mem.indexOfScalar(u8, notify_cache, '\n') orelse unreachable;
    const summary = notify_cache[0..first_linebreak];
    const body = notify_cache[first_linebreak + 1 ..];

    try notifySend(summary, body, options);

    try std.fs.cwd().deleteFile(options.@"notify-cache");
}

const ps1_script_format =
    \\Add-Type -AssemblyName System.Windows.Forms;
    \\Add-Type -AssemblyName System.Drawing;
    \\
    \\$ErrorActionPreference= 'silentlycontinue';
    \\$notifyIcon = New-Object System.Windows.Forms.NotifyIcon;
    \\$notifyIcon.Icon = New-Object System.Drawing.Icon("{s}") || [System.Drawing.SystemIcons]::Information;
    \\$notifyIcon.BalloonTipTitle = "{s}";
    \\$notifyIcon.BalloonTipText = "{s}";
    \\$notifyIcon.Visible = $true;
    \\
    \\$notifyIcon.ShowBalloonTip(5000);
    \\Start-Sleep -Seconds 6;
    \\$notifyIcon.Dispose();
;

fn notifySend(summary: []const u8, body: []const u8, options: Options) !void {
    switch (builtin.target.os.tag) {
        .windows => {
            const cache_dir = std.fs.path.dirname(options.@"notify-cache") orelse "";
            const script_path = try std.fs.path.join(options.allocator, &[_][]const u8{ cache_dir, "notify-send.ps1" });

            const config_dir = std.fs.path.dirname(options.blogroll) orelse "";
            const icon_path = try std.fs.path.join(options.allocator, &[_][]const u8{ config_dir, "badrss.ico" });

            const script_file = try std.fs.cwd().createFile(script_path, .{ .truncate = true });
            try script_file.writer().print(ps1_script_format, .{ icon_path, summary, body });
            script_file.close();

            _ = try std.process.Child.run(.{
                .allocator = options.allocator,
                .argv = &[_][]const u8{ "pwsh", script_path },
            });

            try std.fs.cwd().deleteFile(script_path);
        },
        .linux => {
            _ = try std.process.Child.run(.{
                .allocator = options.allocator,
                .argv = &[_][]const u8{
                    "notify-send",
                    "--app-name=badrss",
                    "--icon=rss",
                    "--expire-time=6000",
                    "--hint=string:desktop-entry:badrss",
                    summary,
                    body,
                },
            });
        },
        else => @compileError("dont do this"),
    }
}

fn clean(options: Options) !void {
    std.fs.cwd().deleteFile(options.@"feed-cache") catch |err| switch (err) {
        std.fs.Dir.DeleteFileError.FileNotFound => {},
        else => return err,
    };

    std.fs.cwd().deleteFile(options.@"notify-cache") catch |err| switch (err) {
        std.fs.Dir.DeleteFileError.FileNotFound => {},
        else => return err,
    };
}
