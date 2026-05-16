import { mkdir, readdir, readFile, rm, writeFile } from "node:fs/promises";
import path from "node:path";

const root = process.cwd();
const sourceDir = path.join(root, "public");
const outDir = process.env.OUT_DIR ? path.resolve(root, process.env.OUT_DIR) : path.join(root, "dist", "public");
const publicBasePath = normalizeBasePath(process.env.PUBLIC_BASE_PATH || "");

await rm(outDir, { recursive: true, force: true });
await copyOptimized(sourceDir, outDir);

async function copyOptimized(src, dest) {
  const entries = await readdir(src, { withFileTypes: true });
  await mkdir(dest, { recursive: true });

  for (const entry of entries) {
    const srcPath = path.join(src, entry.name);
    const destPath = path.join(dest, entry.name);
    if (entry.isDirectory()) {
      await copyOptimized(srcPath, destPath);
      continue;
    }

    const ext = path.extname(entry.name).toLowerCase();
    const rel = path.relative(sourceDir, srcPath);
    const input = await readFile(srcPath, ext === ".svg" || isText(ext) ? "utf8" : undefined);
    let output = input;

    if (ext === ".html") output = rewritePublicBase(minifyHtml(input));
    if (ext === ".css") output = minifyCss(input);
    if (ext === ".svg") output = minifySvg(input);
    if (ext === ".js" && !rel.startsWith(`assets${path.sep}jasmine${path.sep}`)) {
      output = await minifyJs(srcPath);
    }

    await writeFile(destPath, output);
  }
}

function normalizeBasePath(value) {
  if (!value || value === "/") return "";
  return `/${value.replace(/^\/+|\/+$/g, "")}`;
}

function rewritePublicBase(value) {
  if (!publicBasePath) return value;
  return value
    .replaceAll('href="/', `href="${publicBasePath}/`)
    .replaceAll('src="/', `src="${publicBasePath}/`)
    .replaceAll('content="/', `content="${publicBasePath}/`);
}

function isText(ext) {
  return [".html", ".css", ".js", ".svg", ".json", ".txt"].includes(ext);
}

function minifyHtml(value) {
  return value
    .replace(/<!--(?!\[if).*?-->/gs, "")
    .replace(/>\s+</g, "><")
    .replace(/\s{2,}/g, " ")
    .trim();
}

function minifyCss(value) {
  return value
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/\s+/g, " ")
    .replace(/\s*([{}:;,>+~])\s*/g, "$1")
    .replace(/;}/g, "}")
    .trim();
}

function minifySvg(value) {
  return value
    .replace(/<!--[\s\S]*?-->/g, "")
    .replace(/>\s+</g, "><")
    .replace(/\s{2,}/g, " ")
    .trim();
}

async function minifyJs(srcPath) {
  const result = await Bun.build({
    entrypoints: [srcPath],
    minify: true,
    target: "browser",
    write: false,
  });
  if (!result.success) {
    throw new Error(result.logs.map((log) => log.message).join("\n"));
  }
  return await result.outputs[0].text();
}
