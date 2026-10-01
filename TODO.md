# TODO

## serial_script — MCP 定时写/匹配写脚本工具

需求共识见 docs/CONTEXT.md（脚本/定时写/匹配写三术语）。状态：修复完成，待第 2 轮审查。

- [x] 1. `internal/buffer/buffer.go`：数据到达订阅 `Subscribe(fn func([]byte)) (unsubscribe func())`，`Append` 锁外通知（订阅者快照），不改 Read/Peek/Clear 语义
- [x] 2. `pkg/config/config.go`：`ScriptConfig{TimeoutMinMs, TimeoutMaxMs}`，默认 100/1800000，`<=0` 回退默认；`config.example.toml` 加 `[script]` 段
- [x] 3. `pkg/mcp/tools/serial_script.go`（新文件）：
  - 输入 `timeoutMs`（必填）、`writes[]`（atMs/intervalMs/count/data/addNewline）、`matches[]`（pattern/data/addNewline/repeat/maxCount）、`returnData`（默认 true）
  - 定时条目展开为 `atMs + i*intervalMs`（首拍在 atMs，默认 0）
  - 包级 `scriptMu` 单实例；校验：timeout 范围、pattern 编译、条目非空（空剧本=参数错误）、须已连接
  - 订阅 DataBuffer → 单 goroutine 事件循环（timer / 数据 channel / ctx.Done / 100ms tick 查 IsConnected 断连中止）
  - 匹配：8KB 滚动窗口跨 chunk、绝对偏移防重触发、按声明顺序全执行、repeat+maxCount 耗尽失效
  - 完成 = 定时写全发完 且（无匹配规则 或 每条 ≥1 命中）→ 提前成功
  - 返回：timedOut/cancelled、触发记录、各规则触发次数、未触发清单、回放数据（returnData 控制）
  - 超时=成功（同 serial_read 先例）；断连/取消=失败；消息硬编码中文
- [x] 4. `pkg/mcp/server.go`：注册第 8 个工具 `serial_script` + handleSerialScript；超时范围字段 + setter，`cmd/serialhub/serve.go` 用 `cfg.Script` 注入；"7 tools" 日志改 8
- [x] 5. 文档：`MCP.md` 加 serial_script；`AGENTS.md` MCP 工具表加行
- [x] 6. 测试（python3 pty，无 python3 则 Skip）：定时多发、匹配单发、repeat+maxCount、超时成功+未触发清单、断连中止、并发第二脚本拒绝、timeout 范围校验、旧数据不参与匹配；审查后补：零宽正则不卡死、预取消 ctx 不发写、`^` 锚点不重锚、断连后快速重连中止、真实链路回显（python responder 落盘 + DataChan→DataBuffer 接线，断言设备实收 PING/PONG）
- [x] 7. 验证：`go vet ./... && go test ./...` + pty 环境 `-race ./pkg/mcp/...`
- [x] 8. `@code-review` 审查（标准轴 + spec 轴并行）→ 修复直到无错误（共 7 轮，R7 双轴通过）
  - R1：Standards=i18n 硬编码（微决策 4 已批准例外，不阻断）、完成结果构造重复、min/max Data Clumps；Spec=P1 零宽正则死循环、P1 预取消 ctx 仍发写、P1 断连 100ms 内重连漏检、P2 `^` repeat 后缀重锚误触发、P2 回放静默截断、P2 缺真实接收链路测试
    修复：零宽命中跳过（FindAllIndex 全窗口+绝对偏移过滤）、事件循环顶部 ctx.Err 检查、`ConnectionGen()` 代次判定（pkg/serial 新导出）、回放 1MB + `receivedTruncated` 标记、补 5 个用例（含真实链路）、`ScriptLimits` 收拢 min/max、`completeResult`/`abortResult` 消除重复
  - R2：Spec=P1 快速重连绕过（写路径不查代次）、P1 运行中取消不能中止批量发送、P2 atMs×ms 溢出负时长提前发送
    修复：`WriteIfSameGen`（pkg/serial，读锁内校验+写入原子）、process 每次写前 ctx 检查、`scriptMaxAtMsValue` 溢出校验、事件循环每轮核对取消+代次、completeResult 成功前复核
  - R3：Spec=P1 completeResult 遮盖断连、P2 截止后数据仍可触发/误"完成"、P2 累计展开上限不完整
    修复：completeResult 连接复核、inbox 记录到达时刻（超时后到达不匹配不计回放，MCP.md 已注）、`want > max-len` 防溢出预检
  - R4：Spec=P1 展开上限 int 溢出（MaxInt count）、P2 展开期间数据丢失（start/订阅时序）、P2 match 触发缺 occurrence
    修复：先订阅再展开、start 移至展开后、scriptFireItem 存 offsetMs、trigger 补 occurrence（0-based）
  - R5：Spec=P2 大 chunk 先裁剪丢窗口前匹配、P2 到达时刻在锁外回调才记（调度延迟推后边界）
    修复：先扫描后裁剪（裁剪只限跨 chunk 回看）、`Subscribe(fn(data, at))` 签名带到达时刻（Append 临界区捕获）
  - R6：Spec=P1 锁外回调入队被调度延迟，超时排水漏截止前数据
    修复：Append 回调移入 b.mu 临界区同步执行（契约：快速、禁重入、禁阻塞，三处注释）、超时分支 drain 前 `_ = buf.Length()` 屏障
  - R7：Standards 通过（1 条 judgement：Length 屏障命名倾向，已有注释说明，不阻断——为单一调用方新增屏障 API 属过度设计）；Spec 通过（建议：截止前回调暂停的确定性测试无法在不注入生产钩子的前提下构造，接受为设计推理+race 覆盖）
- [x] 9. 提交 `feat(mcp): serial_script 定时写与匹配写脚本工具`

### 实现中自行裁量的微决策（已与用户对齐）
1. 周期写首拍在 `atMs`（默认 0）
2. 空剧本 = 参数错误
3. 断连检测：100ms 轮询，同时比对连接代次 `ConnectionGen()`（断连后快速重连也中止）
4. 工具消息硬编码中文（tools 包惯例）
5. 回放数据上限 1MB（超出截断并返回 `receivedTruncated=true`）；匹配滚动窗口 8KB
6. 零宽正则命中（如 `^`）不触发；`^` 锚定当前窗口起点
