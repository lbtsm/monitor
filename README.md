# Monitor

This project is a monitoring program to monitor whether the light client synchronization is normal,

whether the transaction is cross-chain, and the user balance

# Configuration

See `config.example` for an example configuration.

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
export hooks="https://hooks.slack.com/services/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" // Slack alarm hook, Apply See This https://api.slack.com/messaging/webhooks
```