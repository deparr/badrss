const std = @import("std");

pub fn build(b: *std.Build) !void {
    const optimize = b.standardOptimizeOption(.{});
    const target = b.standardTargetOptions(.{});

    const xml_dep = b.dependency("xml", .{
        .target = target,
        .optimize = optimize,
    });

    const zeit_dep = b.dependency("zeit", .{
        .target = target,
        .optimize = optimize,
    });

    const main_mod = b.addModule("badrss", .{
        .root_source_file = b.path("src/main.zig"),
        .target = target,
        .optimize = optimize,
    });

    main_mod.addImport("xml", xml_dep.module("xml"));
    main_mod.addImport("zeit", zeit_dep.module("zeit"));


    const main_exe = b.addExecutable(.{
        .name = "badrss",
        .root_module = main_mod,
    });

    const check_only = b.option(bool, "check", "check only") orelse false;

    if (check_only) {
        b.getInstallStep().dependOn(&main_exe.step);
    } else {
        b.installArtifact(main_exe);
        const fail = b.addFail("the zig version sucks. use the go one, it actually handles dates and json 'properly'");
        b.getInstallStep().dependOn(&fail.step);
    }
}
