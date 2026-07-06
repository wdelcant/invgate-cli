#!/usr/bin/env node
const { execSync } = require("child_process");
const { existsSync, mkdirSync, createWriteStream, writeFileSync, readFileSync } = require("fs");
const { join } = require("path");
const { homedir, platform } = require("os");
const https = require("https");

const BIN = "invgate-cli" + (platform() === "win32" ? ".exe" : "");
const DIR = join(homedir(), ".invgate", "bin");
const BIN_PATH = join(DIR, BIN);
const VER_PATH = join(DIR, ".version");
const UA = "invgate-cli-npm";

async function getLatestVersion() {
  const opts = { headers: { "User-Agent": UA, "Accept": "application/vnd.github+json" }};
  return new Promise((resolve) => {
    https.get("https://api.github.com/repos/wdelcant/invgate-cli/releases/latest", opts, (res) => {
      let body = "";
      res.on("data", (c) => { body += c; });
      res.on("end", () => {
        try { resolve(JSON.parse(body).tag_name.replace(/^v/, "") || "0.0.0"); }
        catch { resolve("0.0.0"); }
      });
    }).on("error", () => resolve("0.0.0"));
  });
}

function getAsset(ver) {
  const os = { darwin: "macOS", linux: "Linux", win32: "Windows" }[platform()] || "Linux";
  const arch = { x64: "amd64", arm64: "arm64" }[process.arch] || "amd64";
  const ext = platform() === "win32" ? "zip" : "tar.gz";
  return { name: `invgate-cli_${ver}_${os}_${arch}.${ext}`, ext };
}

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const file = createWriteStream(dest);
    https.get(url, { headers: { "User-Agent": UA }}, (res) => {
      // Handle redirects (Node auto-follows, but some proxies don't)
      if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        return download(res.headers.location, dest).then(resolve).catch(reject);
      }
      if (res.statusCode !== 200) {
        file.close();
        try { require("fs").unlinkSync(dest); } catch {}
        return reject(new Error(`HTTP ${res.statusCode} from ${url}`));
      }
      res.pipe(file);
      file.on("finish", () => { file.close(); resolve(); });
    }).on("error", (e) => reject(new Error(`Network error: ${e.message}`)));
  });
}

async function main() {
  const ver = await getLatestVersion();
  const cached = existsSync(VER_PATH) ? readFileSync(VER_PATH, "utf8").trim() : "";

  if (existsSync(BIN_PATH) && cached === ver) {
    try { execSync(`"${BIN_PATH}"`, { stdio: "inherit" }); return; } catch {}
  }

  const { name: asset, ext } = getAsset(ver);
  const url = `https://github.com/wdelcant/invgate-cli/releases/download/v${ver}/${asset}`;
  const tmp = join(DIR, asset);

  mkdirSync(DIR, { recursive: true });
  console.error(`Downloading invgate-cli v${ver} for ${platform()}/${process.arch}...`);
  console.error(`  URL: ${url}`);
  await download(url, tmp);

  if (ext === "zip") {
    execSync(`powershell -Command "Expand-Archive -Path '${tmp}' -DestinationPath '${DIR}' -Force"`, { stdio: "ignore" });
  } else {
    execSync(`tar -xzf "${tmp}" -C "${DIR}"`, { stdio: "ignore" });
  }
  try { require("fs").unlinkSync(tmp); } catch {}
  writeFileSync(VER_PATH, ver);
  execSync(`"${BIN_PATH}"`, { stdio: "inherit" });
}

main().catch((e) => {
  console.error("invgate-cli:", e.message);
  console.error("Download the binary from: https://github.com/wdelcant/invgate-cli/releases/latest");
  process.exit(1);
});
