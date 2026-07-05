#!/usr/bin/env node
const { execSync } = require("child_process");
const { existsSync, mkdirSync, createWriteStream } = require("fs");
const { join } = require("path");
const { homedir, platform } = require("os");
const https = require("https");

const BIN = "invgate-cli" + (platform() === "win32" ? ".exe" : "");
const RELEASES = "https://github.com/wdelcant/invgate-cli/releases";
const { writeFileSync, readFileSync } = require("fs");
const DIR = join(homedir(), ".invgate", "bin");
const BIN_PATH = join(DIR, BIN);
const VER_PATH = join(DIR, ".version");

async function getLatestVersion() {
  return new Promise((resolve, reject) => {
    https.get(`${RELEASES}/latest`, (res) => {
      const location = res.headers.location || "";
      const tag = location.split("/").pop();
      resolve(tag.startsWith("v") ? tag.slice(1) : tag);
    }).on("error", () => resolve(require("./package.json").version));
  });
}

function getAssetName(version) {
  const osMap = { darwin: "macOS", linux: "Linux", win32: "Windows" };
  const archMap = { x64: "amd64", arm64: "arm64" };
  const osName = osMap[platform()] || "Linux";
  const archName = archMap[process.arch] || "amd64";
  const ext = platform() === "win32" ? "zip" : "tar.gz";
  return `invgate-cli_v${VERSION}_${osName}_${archName}.${ext}`;
}

async function download(url, dest) {
  return new Promise((resolve, reject) => {
    const file = createWriteStream(dest);
    https
      .get(url, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          return download(res.headers.location, dest).then(resolve).catch(reject);
        }
        if (res.statusCode !== 200) {
          return reject(new Error(`Download failed: HTTP ${res.statusCode}`));
        }
        res.pipe(file);
        file.on("finish", () => { file.close(); resolve(); });
      })
      .on("error", reject);
  });
}

async function main() {
  const latestVer = await getLatestVersion();
  let currentVer = "";
  try { currentVer = readFileSync(VER_PATH, "utf8").trim(); } catch {}

  if (existsSync(BIN_PATH) && currentVer === latestVer) {
    try {
      execSync(`"${BIN_PATH}"`, { stdio: "inherit" });
      return;
    } catch {}
  }

  const asset = getAssetName(latestVer);
  const url = `${RELEASES}/download/v${latestVer}/${asset}`;
  const tmp = join(DIR, asset);

  mkdirSync(DIR, { recursive: true });
  console.error(`Downloading invgate-cli v${latestVer} for ${platform()}/${process.arch}...`);
  await download(url, tmp);

  if (asset.endsWith(".zip")) {
    execSync(`powershell -Command "Expand-Archive -Path '${tmp}' -DestinationPath '${DIR}' -Force"`, { stdio: "ignore" });
  } else {
    execSync(`tar -xzf "${tmp}" -C "${DIR}"`, { stdio: "ignore" });
  }

  try { require("fs").unlinkSync(tmp); } catch {}
  writeFileSync(VER_PATH, latestVer);

  execSync(`"${BIN_PATH}"`, { stdio: "inherit" });
}

main().catch((err) => {
  console.error("invgate-cli:", err.message);
  console.error("Install manually: brew install wdelcant/tap/invgate-cli");
  process.exit(1);
});
