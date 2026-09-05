# Monitor

This project is a monitoring program to monitor whether the light client synchronization is normal,

whether the transaction is cross-chain, and the user balance

# Configuration

Use a valid `config.json` for runtime configuration. Keep production config
files and keystores out of git.

## Options

```shell
{
  "lightnode": "0x12345...",                              // the lightnode to sync header
  "waterLine": "5000000000000000000",                     // If the user balance is lower than, an alarm will be triggered, unit : wei
  "changeInterval": "3000",                               // How long does the lightnode height remain unchanged, triggering the alarm, use for near unit : seconds
  "checkHeightCount": "20",                               // How long does the lightnode height not change remain unchanged, triggering the alarm, default 15
  "syncHeightAlarm": "false",                              // Optional: disable other-chain-to-map sync height alarm, default true
}
```

## TRON Energy Expiry Monitoring

Add `protectedThreshold` to a tron chain's `energy` entry to enable
Stake 2.0 delegation-expiry alerting (protected energy = inbound delegations
whose lock expires strictly after now+lookahead):

```shell
"energy": [{
  "address": "TT6GDYkpHPVk24w9he9pavbagtzqBRS3XP",
  "waterline": 100000,             // existing: current remaining-energy alarm
  "protectedThreshold": 10000000,  // alert when protected energy drops below
  "recoveryThreshold": 10500000,   // optional, default = protected × 1.05
  "lookaheadHours": 72,            // optional, default 72
  "checkIntervalMinutes": 60,      // optional, default 60
  "repeatIntervalHours": 12        // optional, default 12
}]
```

State files are written to `<keystorePath>/energy_state_<address>.json`.
Scan failures alarm separately as "监控异常" and never count as zero energy.

## Env

```shell
export hooks="https://hooks.slack.com/services/xxx/yyy/zzz"
```

# Docker Deployment

The container uses `/app/runtime` as its runtime directory. Map one host
directory to it and keep `config.json`, `keys/`, and generated state files in
that host directory.

```shell
sudo mkdir -p /opt/bridge-monitor/keys
sudo cp /path/to/your/config.json /opt/bridge-monitor/config.json

cp .env.example .env
vim .env

docker compose up -d --build
docker compose logs -f bridge-monitor
```

If the `github.com/lbtsm/*` Go modules are private, set a GitHub token with
read access before building locally:

```shell
export LBTSM_REPO_TOKEN="github_pat_xxx"
docker compose up -d --build
```

After pulling updates on the server:

```shell
git pull
docker compose up -d --build
```

GitHub Actions builds and tests the project on pull requests, and builds the
Docker image on pushes to `main`, `master`, or version tags. Non-PR builds are
published to GitHub Container Registry as `ghcr.io/<owner>/<repo>:<branch-or-tag>`.
The default branch also publishes `ghcr.io/<owner>/<repo>:latest`.

For private `github.com/lbtsm/*` dependencies, add a repository secret named
`LBTSM_REPO_TOKEN` with read access to the private dependency repositories.
