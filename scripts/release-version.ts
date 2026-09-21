import { readFile } from "node:fs/promises";
import { join } from "node:path";

type PackageJson = {
  version?: unknown;
};

type GitResult = {
  code: number;
  stdout: string;
  stderr: string;
};

const semver = /^(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)(?:-(?:0|[1-9]\d*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;
const root = process.cwd();

async function git(args: string[]): Promise<GitResult> {
  const child = Bun.spawn(["git", ...args], {
    cwd: root,
    stderr: "pipe",
    stdout: "pipe",
  });
  const [code, stdout, stderr] = await Promise.all([
    child.exited,
    new Response(child.stdout).text(),
    new Response(child.stderr).text(),
  ]);
  return { code, stderr, stdout };
}

function fail(message: string): never {
  console.error(message);
  process.exit(1);
}

async function parentPackage(): Promise<PackageJson> {
  const result = await git(["show", "HEAD^:package.json"]);
  if (result.code !== 0) {
    if (/does not have any commits yet|ambiguous argument 'HEAD\^'|invalid object name 'HEAD\^'/i.test(result.stderr)) {
      return {};
    }
    fail(result.stderr.trim() || "could not read parent package.json");
  }
  try {
    return JSON.parse(result.stdout) as PackageJson;
  } catch {
    fail("parent package.json is not valid JSON");
  }
}

async function assertNoTag(tag: string): Promise<void> {
  const local = await git(["rev-parse", "--verify", "--quiet", `refs/tags/${tag}`]);
  if (local.code === 0) fail(`tag already exists locally: ${tag}`);
  if (local.code !== 1) fail(local.stderr.trim() || "could not inspect local tags");

  const remotes = await git(["remote"]);
  if (remotes.code !== 0) fail(remotes.stderr.trim() || "could not inspect Git remotes");
  if (!remotes.stdout.split(/\r?\n/).includes("origin")) return;

  const remote = await git(["ls-remote", "--exit-code", "--tags", "origin", `refs/tags/${tag}`]);
  if (remote.code === 0) fail(`tag already exists on origin: ${tag}`);
  if (remote.code !== 2) fail(remote.stderr.trim() || "could not inspect origin tags");
}

const current = JSON.parse(await readFile(join(root, "package.json"), "utf8")) as PackageJson;
const version = current.version;
if (typeof version !== "string" || !semver.test(version)) {
  fail(`invalid package version: ${String(version)}`);
}

const parent = await parentPackage();
if (typeof parent.version === "string" && parent.version === version) {
  fail(`package version is unchanged: ${version}`);
}

const tag = `v${version}`;
await assertNoTag(tag);

const output = process.env.GITHUB_OUTPUT;
if (output) {
  const file = Bun.file(output);
  const existing = (await file.exists()) ? await file.text() : "";
  await Bun.write(output, `${existing}version=${version}\ntag=${tag}\n`);
}

console.log(`release version ${version} validated`);
