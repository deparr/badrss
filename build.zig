const std = @import("std");

pub fn build(b: *std.Build) !void {
    const optimize = b.standardOptimizeOption(.{});
    const target = b.standardTargetOptions(.{});

    const main_mod = b.addModule("rss", .{
        .root_source_file = b.path("src/main.zig"),
        .target = target,
        .optimize = optimize,
    });

    const main_exe = b.addExecutable(.{
        .name = "rss",
        .root_module = main_mod,
    });

    const check_only = b.option(bool, "check", "check only") orelse false;

    if (check_only) {
        b.getInstallStep().dependOn(&main_exe.step);
    } else {
        b.installArtifact(main_exe);
    }
}
