import { mkdir } from "node:fs/promises";
import { join, resolve } from "node:path";

const root = resolve(import.meta.dir, "..");
await mkdir(join(root, "build"), { recursive: true });

const child = Bun.spawn(
  ["go", "-C", "app", "build", "-o", "../build/peanut", "./cmd/peanut"],
  { cwd: root, stderr: "inherit", stdout: "inherit" },
);

const exitCode = await child.exited;
if (exitCode !== 0) process.exit(exitCode);
