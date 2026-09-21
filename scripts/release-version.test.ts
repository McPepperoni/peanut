import { expect, test } from "bun:test";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

const validator = join(import.meta.dir, "release-version.ts");

type CommandResult = {
  code: number;
  stderr: string;
};

async function run(command: string[], cwd: string, env?: Record<string, string>): Promise<CommandResult> {
  const process = Bun.spawn(command, {
    cwd,
    env: env ? { ...globalThis.process.env, ...env } : undefined,
    stderr: "pipe",
    stdout: "pipe",
  });
  const [code, stderr] = await Promise.all([
    process.exited,
    new Response(process.stderr).text(),
  ]);
  return { code, stderr };
}

async function createRepository(parentVersion?: string): Promise<string> {
  const cwd = await mkdtemp(join(tmpdir(), "peanut-release-version-"));
  await run(["git", "init"], cwd);
  await run(["git", "config", "user.email", "test@example.com"], cwd);
  await run(["git", "config", "user.name", "Peanut Test"], cwd);
  await writeFile(
    join(cwd, "package.json"),
    JSON.stringify(parentVersion ? { name: "peanut", version: parentVersion } : { name: "peanut" }),
  );
  await run(["git", "add", "package.json"], cwd);
  await run(["git", "commit", "-m", "parent"], cwd);
  await writeFile(join(cwd, "base"), "base");
  await run(["git", "add", "base"], cwd);
  await run(["git", "commit", "-m", "base"], cwd);
  return cwd;
}

async function commitVersion(cwd: string, version: string): Promise<void> {
  await writeFile(join(cwd, "package.json"), JSON.stringify({ name: "peanut", version }));
  await run(["git", "add", "package.json"], cwd);
  await run(["git", "commit", "-m", version], cwd);
}

async function validate(cwd: string, version: string): Promise<CommandResult & { output: string }> {
  await writeFile(join(cwd, "package.json"), JSON.stringify({ name: "peanut", version }));
  const output = join(cwd, "github-output");
  const result = await run([process.execPath, validator], cwd, { GITHUB_OUTPUT: output });
  return { ...result, output: await readFile(output, "utf8").catch(() => "") };
}

test("accepts the initial package when the parent has no version", async () => {
  const cwd = await createRepository();
  try {
    const result = await validate(cwd, "0.1.0");
    expect(result.code).toBe(0);
    expect(result.output).toContain("version=0.1.0");
    expect(result.output).toContain("tag=v0.1.0");
  } finally {
    await rm(cwd, { recursive: true, force: true });
  }
});

test("rejects invalid versions", async () => {
  for (const version of ["1.2", "01.2.3", "1.2.3-", "1.2.3+"]) {
    const cwd = await createRepository();
    try {
      expect((await validate(cwd, version)).code).not.toBe(0);
    } finally {
      await rm(cwd, { recursive: true, force: true });
    }
  }
});

test("rejects an unchanged version", async () => {
  const cwd = await createRepository("0.1.0");
  try {
    await writeFile(join(cwd, "marker"), "current");
    await run(["git", "add", "marker"], cwd);
    await run(["git", "commit", "-m", "current"], cwd);
    expect((await validate(cwd, "0.1.0")).code).not.toBe(0);
  } finally {
    await rm(cwd, { recursive: true, force: true });
  }
});

test("compares the working package with the parent commit", async () => {
  const cwd = await createRepository("0.1.0");
  try {
    await commitVersion(cwd, "0.2.0");
    expect((await validate(cwd, "0.1.0")).code).not.toBe(0);
  } finally {
    await rm(cwd, { recursive: true, force: true });
  }
});

test("accepts prerelease and build suffixes", async () => {
  const cwd = await createRepository();
  try {
    const result = await validate(cwd, "1.2.3-rc.1+build.7");
    expect(result.code).toBe(0);
    expect(result.output).toContain("tag=v1.2.3-rc.1+build.7");
  } finally {
    await rm(cwd, { recursive: true, force: true });
  }
});

test("rejects an existing local version tag", async () => {
  const cwd = await createRepository();
  try {
    await run(["git", "tag", "v1.2.3"], cwd);
    expect((await validate(cwd, "1.2.3")).code).not.toBe(0);
  } finally {
    await rm(cwd, { recursive: true, force: true });
  }
});
