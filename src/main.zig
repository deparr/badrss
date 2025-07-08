const std = @import("std");

const Blogs = struct {
    blogs: [][]u8
};

const Options = struct {
    blogroll_path: ?[]u8 = null,
    blogroll: std.ArrayListUnmanaged([]const u8) = .empty,
    command: enum {
        fetch,
        check,
        notify,
    } = .fetch,
};


pub fn main() !void {
    var f = try std.fs.cwd().openFile("input.json", .{});
    const buf = try f.readToEndAlloc(std.heap.smp_allocator, 1 << 24);
    defer std.heap.smp_allocator.free(buf);

    const json = try std.json.parseFromSlice(Blogs, std.heap.smp_allocator, buf, .{ .allocate = .alloc_always });

    for (json.value.blogs) |b| {
        std.debug.print("{s}\n", .{ b });
    }
}
