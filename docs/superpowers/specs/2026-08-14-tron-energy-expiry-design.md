# TRON Energy 到期监控设计

日期：2026-08-14
需求来源：`tron-energy-expiry-alert-requirements.zh-CN.md`（TRON 地址 Energy 到期监控与报警服务需求文档）

## 目标

监控目标 TRON 地址的入站 Energy 代理，每小时计算"lookahead（默认 72h）后仍受链上锁定保护的 Energy"，低于阈值时经 Slack 报警，支持去重、持续提醒、恢复通知，查询失败时发独立的监控异常报警。

## 已确认决策

| 决策点 | 结论 |
|---|---|
| 代码位置 | `chains/tron` 内新增独立检查器（方案 B），独立 goroutine + 独立 ticker |
| 链上查询 | 复用现有 gotron-sdk gRPC 连接，不新增 TronGrid HTTP client |
| 持久化 | JSON 状态文件（仿 blockstore 风格），不引入数据库 |
| 报警渠道 | 仅 Slack，复用 `util.Alarm` |

## 架构

```
Monitor.Sync()
 ├── 现有 balance/energy/token 轮询 goroutine（60s，不动）
 └── EnergyExpiryChecker goroutine（每个配置了 protectedThreshold 的地址）
       ticker（默认 60min）
         ↓
       查询层 con.go：GetInboundDelegatedEnergy + GetAccountResource
         ↓ （任一失败 → UNKNOWN，不产出部分数据）
       计算层 energy_calc.go：纯函数，代理列表 → 快照
         ↓
       状态机 energy_state.go：纯逻辑，快照+上轮状态 → 动作（首报/重提醒/恢复/异常/静默）
         ↓
       持久化 energy_store.go：JSON 原子写
       报警 util.Alarm（Slack）
```

## 配置

扩展现有 `config.Energy` 结构（`internal/config/config.go`），向后兼容：`protectedThreshold == 0` 时新监控不启用，原 `waterline` 行为不变。

```json
"energy": [{
  "address": "TT6GDYkpHPVk24w9he9pavbagtzqBRS3XP",
  "waterline": 100000,
  "protectedThreshold": 10000000,
  "recoveryThreshold": 10500000,
  "lookaheadHours": 72,
  "checkIntervalMinutes": 60,
  "repeatIntervalHours": 12
}]
```

默认值：`recoveryThreshold` = `protectedThreshold × 1.05`（向上取整）；`lookaheadHours` = 72；`checkIntervalMinutes` = 60；`repeatIntervalHours` = 12。

配置热重载（reloader）沿用现有 `UpdateCfg` 机制，checker 每轮 tick 读最新快照配置。

## 查询层（chains/tron/con.go 新增）

`GetInboundDelegatedEnergy(target string) ([]*core.DelegatedResource, error)`：

1. `GetDelegatedResourceAccountIndexV2(target)` → 读 `FromAccounts`（注意：SDK 现成的 `GetDelegatedResourcesV2` 遍历的是 `ToAccounts`，方向相反，不能用）。
2. 逐个 `GetDelegatedResourceV2(from → target)`，请求间隔 500ms。
3. 单请求失败重试 3 次，指数退避（1s/2s/4s）。
4. 任一步最终失败 → 返回 error，本轮整体 UNKNOWN。禁止以部分结果计算总量。

同轮再调 `GetAccountResource(target)` 取 `EnergyLimit`、`EnergyUsed`、`TotalEnergyLimit`、`TotalEnergyWeight`，失败同样 UNKNOWN。

## 计算层（chains/tron/energy_calc.go，纯函数）

输入：代理明细列表、AccountResource、`now`。输出快照：

- `protectedEnergy24h / 72h / 7d`：到期时间严格晚于 `now+窗口` 的入站代理估算 Energy 之和；
- 排除：已到期、窗口内到期、`expire_time_for_energy == 0`（无锁定，随时可撤回）；
- 换算：`estimatedEnergy = frozen_balance_for_energy / 1e6 × TotalEnergyLimit / TotalEnergyWeight`，每轮用最新全网参数；
- 辅助：当前 EnergyLimit/EnergyUsed/剩余、入站代理总估算 Energy、未来 72h 内到期笔数与 Energy、最近一笔到期时间。

`TotalEnergyWeight == 0` 视为数据异常 → UNKNOWN。

## 状态机（chains/tron/energy_state.go，纯逻辑）

三态：`OK` / `ALERT` / `UNKNOWN`。转移与动作：

| 场景 | 动作 |
|---|---|
| OK→ALERT（protectedEnergy72h < protectedThreshold） | 立即首报 |
| ALERT 持续 | 距上次发送 ≥ repeatInterval 时重提醒 |
| ALERT 持续且缺口比最近一次已发送值扩大 >20% | 立即再报 |
| ALERT→OK（protectedEnergy72h ≥ recoveryThreshold） | 发恢复通知一次 |
| ALERT 且值在 [protectedThreshold, recoveryThreshold) | 维持 ALERT，不发（恢复缓冲） |
| 查询失败 | 转 UNKNOWN，保留上一 OK/ALERT 基线；连续失败 ≥3 轮发一次监控异常报警 |
| UNKNOWN→查询恢复 | 按当前值重新判定，与保留基线比较决定是否首报/恢复 |

Energy 阈值报警与监控异常报警文案明确区分，互不混用。

## 报警文案

按需求 6.1 模板，含：地址、预测时间（now+lookahead）、受保护 Energy、阈值、缺口、未来 3 天到期笔数与量、最近到期时间、查询时间。内部计算与存储用 UTC，展示转换为 `Asia/Singapore`（常量，后续需要再做成配置）。发送走 `util.Alarm`。

## 持久化（chains/tron/energy_store.go）

- 每地址一个文件：`<KeystorePath>/energy_state_<address>.json`；
- 内容：报警状态（当前态、首报/最近发送/最近恢复时间、最近已发送指标值、连续失败计数）+ 最近 168 轮快照环形缓冲（约 7 天，用于追溯）；
- 写入：临时文件 + `os.Rename` 原子替换；
- 启动时加载：状态存在则恢复，避免重启后重复首报；文件损坏则告警并按全新状态启动。

## 错误处理

- 429/5xx/超时：指数退避重试（查询层）；
- 部分代理详情缺失、字段异常（如 TotalEnergyWeight=0）：整轮 UNKNOWN；
- 连续 3 轮失败：监控异常报警（独立文案）；
- 绝不把查询失败当作 Energy=0 触发阈值报警。

## 测试

- `energy_calc_test.go`：到期边界（恰好等于 cutoff 不计入）、无锁定代理排除、已到期排除、换算公式、weight=0 异常；
- `energy_state_test.go`：首报、12h 去重、20% 缺口再报、恢复缓冲（阈值与恢复阈值之间不发）、恢复通知、UNKNOWN 不误报、连续 3 次失败才发异常、UNKNOWN 恢复后基线比较；
- `energy_store_test.go`：读写往返、原子性、损坏文件降级；
- 查询层抽小接口（仅覆盖用到的 3 个方法）供上述测试 mock，gRPC 真实调用不在单测范围。

计算层与状态机为纯函数，TDD 先行。

## 边界条件（继承需求第 10 章）

- 同一代理方多笔代理可能被链上聚合，不承诺逐订单展示；
- 链上到期时间是"最早可撤回时间"，非平台承诺撤回时间；
- 换算比例每轮动态更新；
- 时间统一 UTC 存储，展示转 Asia/Singapore。

## 明确不做（YAGNI）

- Telegram / 企业微信渠道；
- SQLite / PostgreSQL；
- Prometheus 指标、HTTP 健康检查、Grafana；
- 出租平台订单 API 对接。
