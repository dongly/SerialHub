# TODO

## web-last-conn — Web 终端记住上次串口与连接参数（用户需求："xterm.js ,若串口可用,串口选择框默认选中上次端口" + "其他参数也是"）

- [x] 1. `pkg/web/static/terminal.html`：
  - 保存：`connectBtn.onclick` 发 connect 前写 `localStorage`——`serialhub-last-port`（端口字符串）+ `serialhub-last-params`（JSON：baudRate/dataBits/parity/stopBits），try/catch 包裹（同 `serialhub-language` 先例）
  - 恢复参数：脚本启动时读 last-params，逐框仅当值是现有 option 时应用（非法/旧值回落 HTML 默认 selected）
  - 恢复端口：`updatePortList` 重建后——当前已选值在新列表中优先保留，否则选 last-port（在列表中才选，不在留占位项，即"若串口可用"）
  - 不动 `selectPortFromStatus`（连接后 status 驱动选中仍生效）
- [x] 2. 验证：提取内嵌 JS 跑 `node --check`（语法）+ 逻辑核对；可行则浏览器手工验证（重启实例后端口/参数默认值、刷新列表后选中行为）——playwright 全套 14 通过（既有 8 + 新增 6：参数恢复/非法值回落/端口可用选中/不可用占位/刷新保持/连接保存，connect 消息被 send 包装吞掉避免真实连接）
- [x] 3. `@code-review` 至无错误（R1 Standards=测试构造重复→`_params_init_script`、Spec=P1 合法 JSON `null` 抛 TypeError 中断页面初始化→对象类型防护+5 参数化损坏存储用例；R2 双轴通过，Spec 建议"四框默认断言+数组/嵌套对象用例"已采纳）
- [x] 4. 提交 `feat(web): 终端记住上次串口与连接参数`；重建二进制 + 重启实例部署

## serial-list-pts — serial_list 列出 /dev/pts 伪终端（用户需求："/dev/pts/13 等加入列表"）

背景：库 `serial.GetPortsList` 枚举 `/dev` 直接子项且跳过目录，`/dev/pts`（目录）整目录被跳过 → pty 永不出现在列表；WSL 前缀过滤只留 ttyUSB/ttyACM。实测结论（保留现状部分）：WSL 下 serial8250 的 ttyS* 有 device 且全可 open，sysfs/试开探测均无法区分，前缀白名单维持不动；本次仅**追加 pts**。

- [ ] 1. `pkg/serial/manager.go`：
  - 注入点 `listPtsFn func() []string`（默认 `listPtsPorts`，同 `listPortsFn` 风格）
  - `listPtsPorts()`：仅 Linux（`runtime.GOOS`），读 `/dev/pts` 取**纯数字**名 → `/dev/pts/N`，按数值升序；ReadDir 失败返回 nil（不报错，pts 缺失是正常情况）
  - `ListPorts()`：库列表（WSL 前缀过滤**之后**）追加 pts——过滤逻辑不动（pts 由我们自己追加，不存在被白名单误杀的路径）
  - 重连影响：`matchReconnectPort` 按精确名/VID·PID 匹配，pts 进列表只增不误配（已核实 manager.go:787）
- [ ] 2. 测试 `pkg/serial/manager_test.go`：注入 `listPtsFn` 断言追加与排序；WSL 过滤分支不吞 pts（追加在过滤后）；`listPtsPorts` 真实 /dev/pts 非 Linux 跳过/失败容忍
- [x] 3. 文档：AGENTS.md 工具表 serial_list 行微调（MCP.md 按用户指示不加专门说明）
- [x] 4. 验证：gofmt、`go vet ./...`、`go test ./...` 全绿；本机实测：`serial_list` 返回 `/dev/pts/0~13`（含新建 pts/13，数值升序、WSL ttyS* 仍被屏蔽），`serial_connect /dev/pts/13` 成功、`serial_status` 正确，测毕恢复 ttyUSB1
- [x] 5. `@code-review` 至无错误（R1 Standards=测试断言英文 → R2 Standards=残留 1 条英文断言+补边界用例、Spec 通过+2 建议（Atoi 放行 `+1`→提取 `parsePtsNum` 逐字符校验、可控边界测试→`ptsPortsFromNames` 纯函数+`TestListPtsFromNames`）→ R3 Standards 终审通过、Spec R2 通过）
- [x] 6. 提交 `feat(serial): serial_list 列出 pts 伪终端`

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

## serial_script-match-delay — 匹配写触发后延时再写（用户需求："匹配模式,触发后应可设定一定延时再写"；"要有默认值"；"默认5ms吧"）

状态：开发/测试/验证完成，待 @code-review。

- [x] 1. `MatchRule` 加 `delayMs *int64`（指针区分未填/填 0：**未填=默认 5ms** `defaultMatchDelayMs`，显式 0=立即写；同 AddNewline 指针先例）；matchState 加规范化 `delayMs` 字段；校验非 nil 时负数/≤ scriptMaxAtMsValue 防溢出
- [x] 2. `serial_script.go`：命中时 delayMs==0 走原有立即写；>0 排入延时队列（container/heap 小顶堆，fireAt=命中时刻+delayMs），到点后走与定时写相同的 ctx 检查 + WriteIfSameGen 路径；重复命中各自独立排队（与 repeat/maxCount 语义一致）
- [x] 3. 完成条件加"延时队列清空"；超时/取消/断连丢弃队列中未发出条目；返回数据加 `delayedWritesQueued/Fired/Pending`；完成消息附"延时写 n/n"（queued>0 时）；事件循环 timer 融合定时写与延时写取最早
- [x] 4. match 触发记录改为**命中时**追加（含 `delayMs` 字段，atMs=命中时刻）——与"延时未写出但已命中"的可观测性一致
- [x] 5. server.go InputSchema matches 加 delayMs（Default 5；0=immediate）；MCP.md 参数示例/注释、完成条件（含延时写）、超时丢弃说明、返回字段 JSON、触发记录字段说明（match atMs=命中时刻）
- [x] 6. 测试 6 个新用例全过：MatchDelayDefault（默认 5ms 计数+delayMs 值）、MatchDelayExplicit（300ms）、MatchDelayZero（显式 0 不入队）、MatchDelayDroppedOnTimeout（超时丢弃 pending=1 fired=0 + 命中计数保留）、MatchDelayRepeatIndependent（一 chunk 两命中独立入队写出）、DelayMsValidation（负/溢出，含于 DelayMsValidation t.Run）
- [x] 7. 验证：gofmt 无输出、go vet ./... 通过、SERIALHUB_TEST_PORT=pty go test ./... 全 ok、-race ./pkg/mcp/... ./internal/buffer/ ./pkg/serial/ 全绿
- [x] 8. @code-review 至无错误（R1 Standards 2 条建议+Spec 1 条高危 → R2 双轴通过；复审建议"慢写跨截止批次"回归测试需注入写耗时钩子不可构造，接受为设计推理+deadline 复核测试覆盖）
- [x] 9. 提交 `feat(mcp): serial_script 匹配写支持触发后延时再写`

微决策（按既有设计共识推导 + 用户默认值指示）：
1. 延时写不阻塞匹配扫描：命中即计 fired（ruleFireCounts 反映命中），写出由延时队列负责
2. 延时写到点若已超时/取消/断连 → 丢弃不写（与"截止后不写"一致），Pending 字段如实上报
3. 队列上限 scriptScheduleMax（防极端 repeat+delay 积压 OOM，超出=脚本中止）
4. trigger 记录=命中事实（含 delayMs），实际写出看 delayedWritesFired

### 实现中自行裁量的微决策（已与用户对齐）
1. 周期写首拍在 `atMs`（默认 0）
2. 空剧本 = 参数错误
3. 断连检测：100ms 轮询，同时比对连接代次 `ConnectionGen()`（断连后快速重连也中止）
4. 工具消息硬编码中文（tools 包惯例）
5. 回放数据上限 1MB（超出截断并返回 `receivedTruncated=true`）；匹配滚动窗口 8KB
6. 零宽正则命中（如 `^`）不触发；`^` 锚定当前窗口起点
