const child = Bun.spawn(["git", "rev-parse", "--is-inside-work-tree"], {
  stderr: "ignore",
  stdout: "ignore",
});

if ((await child.exited) === 0) {
  const configure = Bun.spawn(["git", "config", "core.hooksPath", ".githooks"], {
    stderr: "inherit",
    stdout: "inherit",
  });
  process.exit(await configure.exited);
}
