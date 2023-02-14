# Monitor

This project is a monitoring program to monitor whether the light client synchronization is normal,

whether the transaction is cross-chain, and the user balance

# Configuration

See `config.example` for an example configuration.

## Options

```shell
{
  "lightnode": "0x12345...",                              // the lightnode to sync header
  "waterLine": "5000000000000000000",                     // If the user balance is lower than, an alarm will be triggered, unit ：wei
  "alarmSecond": "3000",                                  // How long does the user balance remain unchanged, triggering the alarm, unit ：seconds
}
```

## Env

```shell
export bridgeConn="user:123456@tcp(127.0.0.1:3306)/dateBaseName?charset=utf8&parseTime=true" // Address of the database where the transaction is stored
 
export hooks="https://hooks.slack.com/services/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" // Slack alarm hook, Apply See This https://api.slack.com/messaging/webhooks
 
```