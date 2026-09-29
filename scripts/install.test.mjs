import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  statSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

const script = fileURLToPath(new URL("./install.sh", import.meta.url));
const compose = "services:\n  sublane:\n    image: ${SUBLANE_IMAGE}\n    environment:\n      SUBLANE_DEMO: ${SUBLANE_DEMO:-false}\n";

function fixture(t, mode = "", api = {}, portTool = "ss") {
  const deployment = mode === "legacy-demo"
    ? "services:\n  sublane:\n    image: ${SUBLANE_IMAGE}\n"
    : compose;
  const checksum = createHash("sha256").update(deployment).digest("hex");
  const root = mkdtempSync(join(tmpdir(), "sublane-installer-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const bin = join(root, "bin");
  mkdirSync(bin);
  const archiveRoot = join(root, "archive");
  mkdirSync(archiveRoot);
  writeFileSync(join(archiveRoot, "sublane"), "#!/bin/sh\nexit 0\n", { mode: 0o755 });
  writeFileSync(join(archiveRoot, "LICENSE"), "Synthetic license\n");
  writeFileSync(join(archiveRoot, ".env.example"), mode === "legacy-demo" ? "SUBLANE_ADDR=127.0.0.1:8080\n" : "SUBLANE_DEMO=false\n");
  const archiveName = "sublane_1.2.3_linux_amd64.tar.gz";
  const archivePath = join(root, archiveName);
  const packed = spawnSync("tar", ["-czf", archivePath, "-C", archiveRoot, "sublane", "LICENSE", ".env.example"]);
  assert.equal(packed.status, 0, packed.stderr?.toString());
  const archiveChecksum = createHash("sha256").update(readFileSync(archivePath)).digest("hex");
  const mock = `#!${process.execPath}
const fs = require('node:fs');
const path = require('node:path');
const command = path.basename(process.argv[1]);
const args = process.argv.slice(2);
const mode = process.env.INSTALL_TEST_MODE;
fs.appendFileSync(process.env.INSTALL_TEST_LOG, JSON.stringify({command, args, image: process.env.SUBLANE_IMAGE, port: process.env.SUBLANE_PORT, demo: process.env.SUBLANE_DEMO}) + '\\n');
if (command === 'curl') {
  if (mode === 'download-failure') process.exit(22);
  if (args.some(value => value.endsWith('/readyz'))) {
    if (mode === 'unhealthy-binary') process.exit(22);
    process.stdout.write('{"status":"ready"}');
    process.exit(0);
  }
  const url = args.find(value => value.startsWith('https://'));
  const output = args[args.indexOf('--output') + 1];
  if (url.startsWith('https://api.github.com/')) {
    const api = JSON.parse(process.env.INSTALL_TEST_API);
    const latest = url.endsWith('/releases/latest');
    const status = latest ? (api.latestStatus ?? 404) : (api.listStatus ?? 200);
    const page = new URL(url).searchParams.get('page') ?? '1';
    const body = latest ? (api.latest ?? {message: 'Not Found'}) : (api.pages?.[page] ?? [
      {tag_name:'v0.1.0-rc.1',draft:false,prerelease:true,published_at:'2030-01-01T00:00:00Z'}
    ]);
    fs.writeFileSync(output, JSON.stringify(body));
    process.stdout.write(String(status));
    process.exit(0);
  }
  if (url.endsWith('.tar.gz')) fs.copyFileSync(process.env.INSTALL_TEST_ARCHIVE, output);
  else {
    const body = url.endsWith('/SHA256SUMS')
      ? (mode === 'bad-checksum' ? '0'.repeat(64) : ${JSON.stringify(checksum)}) + '  docker.compose.yaml\\n'
        + (mode === 'bad-checksum' || mode === 'bad-binary-checksum' ? '0'.repeat(64) : process.env.INSTALL_TEST_ARCHIVE_CHECKSUM)
        + '  ' + process.env.INSTALL_TEST_ARCHIVE_NAME + '\\n'
      : ${JSON.stringify(deployment)};
    fs.writeFileSync(output, body);
  }
} else if (command === 'docker') {
  if (args[0] === 'info' && mode === 'no-engine') process.exit(1);
  if (args.includes('pull') && mode === 'pull-failure') process.exit(1);
  if (args.includes('up') && mode === 'unhealthy') process.exit(1);
} else if (command === 'uname') {
  process.stdout.write(args[0] === '-s' ? 'Linux\\n' : 'x86_64\\n');
} else if (command === 'systemctl') {
  if (mode === 'systemd-failure' && args.includes('enable')) process.exit(1);
  if (mode === 'inactive-binary' && args.includes('is-active')) process.exit(1);
} else if (command === 'ss' || command === 'lsof') {
  if (mode === 'port-check-failure') {
    process.stderr.write('Synthetic socket inspection failure\\n');
    process.exit(1);
  }
  process.stdout.write(process.env.INSTALL_TEST_LISTENERS);
  if (command === 'lsof' && !process.env.INSTALL_TEST_LISTENERS) process.exit(1);
}
`;
  for (const name of ["curl", "docker", "uname", "systemctl", "ss", "lsof"])
    writeFileSync(join(bin, name), mock, { mode: 0o755 });
  const shellEnv = join(root, "shell.env");
  writeFileSync(shellEnv, `command() {
  if [[ $1 == -v && ( $2 == ss || $2 == lsof ) && $2 != ${portTool} ]]; then return 1; fi
  builtin command "$@"
}
`);
  const log = join(root, "commands.jsonl");
  return {
    root,
    target: join(root, "sublane"),
    env: {
      ...process.env,
      PATH: `${bin}:${process.env.PATH}`,
      BASH_ENV: shellEnv,
      INSTALL_TEST_LOG: log,
      INSTALL_TEST_MODE: mode,
      INSTALL_TEST_LISTENERS: "",
      INSTALL_TEST_API: JSON.stringify(api),
      INSTALL_TEST_ARCHIVE: archivePath,
      INSTALL_TEST_ARCHIVE_CHECKSUM: archiveChecksum,
      INSTALL_TEST_ARCHIVE_NAME: archiveName,
    },
    calls: () =>
      existsSync(log)
        ? readFileSync(log, "utf8").trim().split("\n").map(JSON.parse)
        : [],
  };
}

function install(f, args = [], piped = false) {
  return spawnSync("bash", piped ? ["-s", "--", ...args] : [script, ...args], {
    cwd: f.root,
    env: f.env,
    input: piped ? readFileSync(script) : undefined,
    encoding: "utf8",
    timeout: 15000,
  });
}

for (const runtime of ["docker", "binary"]) {
  test(`rejects an occupied ${runtime} port before downloading or starting services`, (t) => {
    const f = fixture(t);
    f.env.INSTALL_TEST_LISTENERS = "LISTEN 0 128 127.0.0.1:18080 0.0.0.0:*\n";
    const result = install(f, ["--version", "1.2.3", "--runtime", runtime, "--port", "18080"], true);
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /127\.0\.0\.1:18080.*already in use/i);
    assert.match(result.stderr, /--port/);
    assert.equal(existsSync(f.target), false);
    assert.ok(f.calls().every(call => call.command === "ss"));
  });
}

for (const [tool, addresses] of [
  ["ss", ["0.0.0.0:8080", "*:8080", "[::]:8080", "[::ffff:127.0.0.1]:8080"]],
  ["lsof", ["127.0.0.1:8080", "*:8080", "[::ffff:127.0.0.1]:8080"]],
]) {
  test(`detects loopback and wildcard port conflicts with ${tool}`, (t) => {
    for (const address of addresses) {
      const f = fixture(t, "", {}, tool);
      f.env.INSTALL_TEST_LISTENERS = tool === "ss"
        ? `LISTEN 0 128 ${address} *:*\n`
        : `p123\nf3\nn${address}\n`;
      const result = install(f, ["--version", "1.2.3"]);
      assert.notEqual(result.status, 0, address);
      assert.match(result.stderr, /127\.0\.0\.1:8080.*already in use/i);
      assert.equal(existsSync(f.target), false);
      assert.ok(f.calls().every(call => call.command === tool));
    }
  });

  test(`allows unrelated interfaces and proxy ports with ${tool}`, (t) => {
    const f = fixture(t, "", {}, tool);
    f.env.INSTALL_TEST_LISTENERS = tool === "ss"
      ? "LISTEN 0 128 127.0.0.2:8080 *:*\nLISTEN 0 128 [::1]:8080 *:*\nLISTEN 0 128 *:80 *:*\nLISTEN 0 128 *:443 *:*\nLISTEN 0 128 *:18080 *:*\n"
      : "p123\nf3\nn127.0.0.2:8080\nf4\nn[::1]:8080\nf5\nn*:80\nf6\nn*:443\nf7\nn*:18080\n";
    const result = install(f, ["--version", "1.2.3", "--proxy", "nginx"]);
    assert.equal(result.status, 0, result.stderr);
    assert.ok(f.calls().some(call => call.command === tool));
  });

  test(`stops when ${tool} cannot inspect the port`, (t) => {
    const f = fixture(t, "port-check-failure", {}, tool);
    const result = install(f, ["--version", "1.2.3"]);
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /Could not check.*127\.0\.0\.1:8080/i);
    assert.equal(existsSync(f.target), false);
    assert.ok(f.calls().every(call => call.command === tool));
  });

  test(`accepts an unused port with ${tool}`, (t) => {
    const f = fixture(t, "", {}, tool);
    const result = install(f, ["--version", "1.2.3", "--port", "18851"]);
    assert.equal(result.status, 0, result.stderr);
    const check = f.calls().find(call => call.command === tool);
    assert.ok(check);
    assert.ok(check.args.some(arg => arg.includes("18851")));
  });
}

test("explains how to enable port inspection when neither utility is installed", (t) => {
  const f = fixture(t, "", {}, "none");
  const result = install(f, ["--version", "1.2.3"]);
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /Install.*ss.*lsof/i);
  assert.equal(existsSync(f.target), false);
  assert.equal(f.calls().length, 0);
});

for (const runtime of ["docker", "binary"]) {
  test(`persists demo mode in a ${runtime} installation`, (t) => {
    const f = fixture(t);
    f.env.SUBLANE_DEMO = "false";
    const result = install(f, ["--version", "1.2.3", "--runtime", runtime, "--demo", "--non-interactive"], true);
    assert.equal(result.status, 0, result.stderr);
    const settings = readFileSync(join(f.target, runtime === "docker" ? ".env" : "sublane.env"), "utf8");
    assert.match(settings, /^SUBLANE_DEMO=true$/m);
    assert.match(result.stdout, /demo.*sublane-demo/i);
    assert.doesNotMatch(result.stdout, /to create the administrator/);
    if (runtime === "docker") {
      const calls = f.calls().filter(call => call.args.includes("--project-name"));
      assert.ok(calls.length >= 3);
      assert.ok(calls.every(call => call.demo === "true"));
    } else {
      writeFileSync(join(f.target, "sublane"), '#!/bin/sh\nprintf "%s" "$SUBLANE_DEMO"\n', { mode: 0o755 });
      const launch = spawnSync("bash", [join(f.target, "start.sh")], { env: f.env, encoding: "utf8" });
      assert.equal(launch.status, 0, launch.stderr);
      assert.equal(launch.stdout, "true");
    }
  });

  test(`normal ${runtime} installation ignores inherited demo interpolation`, (t) => {
    const f = fixture(t);
    f.env.SUBLANE_DEMO = "true";
    const result = install(f, ["--version", "1.2.3", "--runtime", runtime, "--non-interactive"]);
    assert.equal(result.status, 0, result.stderr);
    assert.match(readFileSync(join(f.target, runtime === "docker" ? ".env" : "sublane.env"), "utf8"), /^SUBLANE_DEMO=false$/m);
    if (runtime === "docker")
      assert.ok(f.calls().filter(call => call.args.includes("--project-name")).every(call => call.demo === "false"));
  });

  test(`rejects ${runtime} releases without demo support before deployment`, (t) => {
    const f = fixture(t, "legacy-demo");
    const result = install(f, ["--version", "1.2.3", "--runtime", runtime, "--demo", "--non-interactive"]);
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /does not support demo mode/i);
    assert.equal(existsSync(f.target), false);
    assert.equal(f.calls().some(call => call.args.includes("up") || call.args.includes("pull") || call.args.includes("enable") || call.args.includes("link")), false);
    const normal = install(f, ["--version", "1.2.3", "--runtime", runtime, "--non-interactive"]);
    assert.equal(normal.status, 0, normal.stderr);
  });
}

for (const [choice, demo] of [["", "false"], ["1", "false"], ["2", "true"]]) {
  test(`interactive installation selects ${demo === "true" ? "demo" : "normal"} mode from choice '${choice}'`, (t) => {
    const f = fixture(t);
    const source = readFileSync(script, "utf8").replace(/\nmain "\$@"\s*$/, `
prompt_available() { return 0; }
prompt_choice() {
  case "$1" in
    'Instance mode'*) printf '${choice}' ;;
    'Proceed with this installation'*) printf 'y' ;;
    *) printf 'Unexpected prompt: %s' "$1" >&2; return 1 ;;
  esac
}
main "$@"
`);
    const result = spawnSync("bash", ["-s", "--", "--version", "1.2.3", "--runtime", "docker", "--proxy", "none"], {
      cwd: f.root, env: f.env, input: source, encoding: "utf8", timeout: 15000,
    });
    assert.equal(result.status, 0, result.stderr);
    assert.ok(readFileSync(join(f.target, ".env"), "utf8").split("\n").includes(`SUBLANE_DEMO=${demo}`));
  });
}

test("installs a pinned release with checksums and waits for healthy Compose startup", (t) => {
  const f = fixture(t);
  const result = install(f);
  assert.equal(result.status, 0, result.stderr);
  assert.match(
    readFileSync(join(f.target, ".env"), "utf8"),
    /SUBLANE_IMAGE=ghcr.io\/murongg\/sublane:0\.1\.0-rc\.1/,
  );
  assert.equal(
    readFileSync(join(f.target, "docker.compose.yaml"), "utf8"),
    compose,
  );
  assert.equal(statSync(join(f.target, ".env")).mode & 0o777, 0o600);
  const calls = f.calls();
  assert.equal(calls.filter((call) => call.command === "curl").length, 4);
  const up = calls.find((call) => call.args.includes("up"));
  assert.ok(up.args.includes("--wait"));
  assert.ok(up.args.includes("--no-build"));
  assert.ok(up.args.includes("--project-name"));
  assert.ok(
    calls.findIndex((call) => call.args.includes("pull")) < calls.indexOf(up),
  );
  assert.match(result.stdout, /http:\/\/127\.0\.0\.1:8080/);
});

test("supports piped execution, explicit versions, paths with spaces and custom ports", (t) => {
  const f = fixture(t);
  const target = join(f.root, "team gateway");
  const result = install(
    f,
    ["--version", "v1.2.3-beta.2", "--dir", target, "--port", "18851"],
    true,
  );
  assert.equal(result.status, 0, result.stderr);
  const env = readFileSync(join(target, ".env"), "utf8");
  assert.match(env, /SUBLANE_IMAGE=ghcr.io\/murongg\/sublane:1\.2\.3-beta\.2/);
  assert.match(env, /SUBLANE_PORT=18851/);
  assert.match(env, /COMPOSE_PROJECT_NAME=sublane-[a-z0-9-]+/);
  assert.match(result.stdout, /127\.0\.0\.1:18851/);
  assert.equal(
    f
      .calls()
      .some((call) =>
        call.args.some((arg) => arg.startsWith("https://api.github.com/")),
      ),
    false,
  );
});

test("generates a Caddy configuration and external origin for a Docker install", (t) => {
  const f = fixture(t);
  const result = install(f, [
    "--version", "1.2.3", "--runtime", "docker", "--proxy", "caddy",
    "--domain", "gateway.example.test", "--port", "18080",
  ]);
  assert.equal(result.status, 0, result.stderr);
  const env = readFileSync(join(f.target, ".env"), "utf8");
  assert.match(env, /^SUBLANE_PUBLIC_URL=https:\/\/gateway\.example\.test$/m);
  assert.match(readFileSync(join(f.target, "Caddyfile"), "utf8"), /reverse_proxy 127\.0\.0\.1:18080/);
  assert.equal(existsSync(join(f.target, "nginx.conf")), false);
});

test("generates an Nginx configuration with forwarding and streaming headers", (t) => {
  const f = fixture(t);
  const result = install(f, [
    "--version", "1.2.3", "--runtime", "docker", "--proxy", "nginx",
    "--domain", "gateway.example.test",
  ]);
  assert.equal(result.status, 0, result.stderr);
  const config = readFileSync(join(f.target, "nginx.conf"), "utf8");
  assert.match(config, /server_name gateway\.example\.test;/);
  assert.match(config, /proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;/);
  assert.match(config, /proxy_set_header Upgrade \$http_upgrade;/);
  assert.match(config, /proxy_buffering off;/);
  assert.match(config, /proxy_pass http:\/\/127\.0\.0\.1:8080;/);
});

test("allows Caddy and Nginx without a domain using local HTTP only", (t) => {
  for (const proxy of ["caddy", "nginx"]) {
    const f = fixture(t);
    const result = install(f, ["--version", "1.2.3", "--proxy", proxy]);
    assert.equal(result.status, 0, result.stderr);
    const env = readFileSync(join(f.target, ".env"), "utf8");
    assert.match(env, /^SUBLANE_PUBLIC_URL=$/m);
    const config = readFileSync(join(f.target, proxy === "caddy" ? "Caddyfile" : "nginx.conf"), "utf8");
    assert.match(config, /127\.0\.0\.1:80/);
    assert.doesNotMatch(config, /listen 443|https:\/\/gateway/);
  }
});

test("installs a verified binary and enables its systemd user service without Docker", (t) => {
  const f = fixture(t);
  const result = install(f, [
    "--version", "1.2.3", "--runtime", "binary", "--proxy", "caddy",
    "--domain", "gateway.example.test",
  ]);
  assert.equal(result.status, 0, result.stderr);
  assert.equal(existsSync(join(f.target, "sublane")), true);
  assert.equal(existsSync(join(f.target, "start.sh")), true);
  assert.match(readFileSync(join(f.target, "sublane.env"), "utf8"), /SUBLANE_TRUSTED_PROXIES=127\.0\.0\.1\/32/);
  assert.match(readFileSync(join(f.target, "Caddyfile"), "utf8"), /reverse_proxy 127\.0\.0\.1:8080/);
  assert.ok(f.calls().some(call => call.command === "systemctl" && call.args.includes("enable") && call.args.includes("--now")));
  assert.equal(f.calls().some(call => call.command === "docker"), false);
});

test("binary service unit quotes an installation path with spaces", (t) => {
  const f = fixture(t);
  const target = join(f.root, "team gateway");
  const result = install(f, ["--version", "1.2.3", "--runtime", "binary", "--dir", target]);
  assert.equal(result.status, 0, result.stderr);
  const unitName = f.calls().find(call => call.command === "systemctl" && call.args.includes("link"))?.args.at(-1);
  assert.ok(unitName);
  const unit = readFileSync(unitName, "utf8");
  assert.match(unit, /ExecStart=\/bin\/bash "[^"]*team gateway\/start\.sh"/);
});

test("a systemd activation failure retains verified binary files for inspection", (t) => {
  const f = fixture(t, "systemd-failure");
  const result = install(f, ["--version", "1.2.3", "--runtime", "binary"]);
  assert.notEqual(result.status, 0);
  assert.equal(existsSync(join(f.target, "sublane")), true);
  assert.match(result.stderr, /retained|preserved/i);
});

test("a corrupt binary archive is rejected before systemd is changed", (t) => {
  const f = fixture(t, "bad-binary-checksum");
  const result = install(f, ["--version", "1.2.3", "--runtime", "binary"]);
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /checksum mismatch/);
  assert.equal(existsSync(f.target), false);
  assert.equal(f.calls().some(call => call.command === "systemctl" && call.args.includes("link")), false);
});

test("a binary service that never becomes ready retains its files and reports failure", (t) => {
  const f = fixture(t, "unhealthy-binary");
  const result = install(f, [
    "--version", "1.2.3", "--runtime", "binary", "--wait-timeout", "1",
  ]);
  assert.notEqual(result.status, 0);
  assert.equal(existsSync(join(f.target, "sublane")), true);
  assert.match(result.stderr, /ready|healthy/i);
  assert.ok(f.calls().some(call => call.command === "systemctl" && call.args.includes("disable") && call.args.includes("--now")));
});

test("binary readiness cannot be satisfied by another process on the port", (t) => {
  const f = fixture(t, "inactive-binary");
  const result = install(f, ["--version", "1.2.3", "--runtime", "binary", "--wait-timeout", "1"]);
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /service|ready/i);
});

test("rejects invalid arguments before downloading or starting containers", (t) => {
  for (const args of [
    ["--version", "1.2.3;touch unexpected"],
    ["--version", "1.2"],
    ["--version", "v"],
    ["--version", "01.2.3"],
    ["--version", "1.2.3-rc.01"],
    ["--port", "0"],
    ["--port", "65536"],
    ["--port", "8080\nOTHER=value"],
    ["--dir", "line\nbreak"],
    ["--proxy", "caddy", "--domain", "gateway..example.test"],
    ["--proxy", "nginx", "--domain", "gateway.example.test."],
    ["--proxy", "caddy", "--trusted-proxies", "127.0.0.1"],
    ["--version"],
    ["--force"],
  ]) {
    const f = fixture(t);
    const result = install(f, args);
    assert.notEqual(result.status, 0, args.join(" "));
    assert.match(result.stderr, /Error:/);
    assert.equal(f.calls().length, 0);
    assert.equal(existsSync(f.target), false);
  }
});

test("preserves existing installations and refuses symlinks", (t) => {
  for (const symlink of [false, true]) {
    const f = fixture(t);
    const original = symlink ? join(f.root, "original") : f.target;
    mkdirSync(original);
    writeFileSync(join(original, ".env"), "SYNTHETIC_SETTING=keep\n");
    if (symlink) symlinkSync(original, f.target);
    const result = install(f);
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /exists/i);
    assert.equal(
      readFileSync(join(original, ".env"), "utf8"),
      "SYNTHETIC_SETTING=keep\n",
    );
    assert.equal(f.calls().length, 0);
  }
});

test("download, checksum, engine and pull failures leave no installation", (t) => {
  for (const [mode, message] of [
    ["download-failure", /Could not (download|fetch)/],
    ["bad-checksum", /checksum mismatch/],
    ["no-engine", /Docker is not running/],
    ["pull-failure", /Image pull failed/],
  ]) {
    const f = fixture(t, mode);
    const result = install(f);
    assert.notEqual(result.status, 0, mode);
    assert.match(result.stderr, message);
    assert.equal(existsSync(f.target), false, mode);
    assert.equal(
      f.calls().some((call) => call.args.includes("up")),
      false,
    );
  }
});

test("a startup failure retains configuration and never removes container data", (t) => {
  const f = fixture(t, "unhealthy");
  const result = install(f);
  assert.notEqual(result.status, 0);
  assert.equal(existsSync(join(f.target, ".env")), true);
  assert.match(result.stderr, /retained|preserved/i);
  assert.equal(
    f
      .calls()
      .some((call) => call.args.includes("down") || call.args.includes("rm")),
    false,
  );
  assert.doesNotMatch(result.stdout, /Installation complete/);
});

test("explicit installer settings take precedence over inherited Compose interpolation", (t) => {
  const f = fixture(t);
  Object.assign(f.env, {
    SUBLANE_IMAGE: "example.test/unrelated:image",
    SUBLANE_PORT: "9099",
    COMPOSE_PROJECT_NAME: "existing-synthetic-project",
    COMPOSE_FILE: "/synthetic/unrelated.yaml",
  });
  const result = install(f);
  assert.equal(result.status, 0, result.stderr);
  const calls = f
    .calls()
    .filter((call) => call.args.includes("--project-name"));
  assert.ok(calls.length >= 3);
  for (const call of calls) {
    assert.equal(call.image, "ghcr.io/murongg/sublane:0.1.0-rc.1");
    assert.equal(call.port, "8080");
    assert.notEqual(
      call.args[call.args.indexOf("--project-name") + 1],
      "existing-synthetic-project",
    );
    assert.ok(call.args.includes("--env-file"));
    assert.ok(call.args.includes("-f"));
  }
});

test("help performs no deployment actions", (t) => {
  const f = fixture(t);
  const result = install(f, ["--help"]);
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /--version/);
  assert.equal(f.calls().length, 0);
  assert.equal(existsSync(f.target), false);
});

test("a truncated piped download does not begin installation", (t) => {
  const f = fixture(t);
  const source = readFileSync(script, "utf8");
  const result = spawnSync("bash", ["-s"], {
    cwd: f.root,
    env: f.env,
    input: source.slice(0, source.indexOf('  compose "$install_stage" pull')),
    encoding: "utf8",
  });
  assert.notEqual(result.status, 0);
  assert.equal(f.calls().length, 0);
  assert.equal(existsSync(f.target), false);
});

test("prefers the latest stable release without consulting prereleases", (t) => {
  const f = fixture(t, "", {
    latestStatus: 200,
    latest: { tag_name: "v1.2.3", draft: false, prerelease: false },
  });
  const result = install(f);
  assert.equal(result.status, 0, result.stderr);
  assert.match(
    readFileSync(join(f.target, ".env"), "utf8"),
    /SUBLANE_IMAGE=ghcr.io\/murongg\/sublane:1\.2\.3\n/,
  );
  assert.equal(
    f
      .calls()
      .filter((call) =>
        call.args.some((arg) => arg.startsWith("https://api.github.com/")),
      ).length,
    1,
  );
});

test("without stable releases, selects the most recently published prerelease across pages", (t) => {
  const older = {
    tag_name: "v2.0.0-rc.1",
    draft: false,
    prerelease: true,
    published_at: "2030-01-01T00:00:00Z",
  };
  const f = fixture(t, "", {
    pages: {
      1: Array.from({ length: 100 }, () => older),
      2: [
        {
          ...older,
          tag_name: "v2.0.0-rc.3",
          draft: true,
          published_at: "2030-04-01T00:00:00Z",
        },
        {
          ...older,
          tag_name: "v2.0.0",
          prerelease: false,
          published_at: "2030-03-01T00:00:00Z",
        },
        {
          ...older,
          tag_name: "v2.0.0-rc.2",
          published_at: "2030-02-01T00:00:00Z",
        },
      ],
    },
  });
  const result = install(f);
  assert.equal(result.status, 0, result.stderr);
  assert.match(
    readFileSync(join(f.target, ".env"), "utf8"),
    /SUBLANE_IMAGE=ghcr.io\/murongg\/sublane:2\.0\.0-rc\.2\n/,
  );
});

test("API errors and missing releases stop without silently choosing another channel", (t) => {
  for (const api of [
    { latestStatus: 403 },
    { latestStatus: 500 },
    { latestStatus: 200, latest: {} },
    { listStatus: 503 },
    { pages: { 1: [] } },
  ]) {
    const f = fixture(t, "", api);
    const result = install(f);
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /Error:/);
    assert.equal(existsSync(f.target), false);
    assert.equal(
      f
        .calls()
        .some((call) =>
          call.args.some((arg) => arg.includes("/releases/download/")),
        ),
      false,
    );
  }
});
