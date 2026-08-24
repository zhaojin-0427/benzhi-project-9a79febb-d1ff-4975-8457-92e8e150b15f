# 舞台机械安全启用核验台

本项目为剧院舞台机械提供演出前安全核验工作台。工程师可创建并修订草稿档案、登记设备边界，通过设备检查矩阵逐项或批量提交现场检查；系统在完整性门禁通过后，依据载荷、限位响应和急停结果执行可重复的风险对账。工程师提交带元数据的整改证据清单后，安全员逐项通过或退回问题，全部当前问题通过后才能冻结安全版本。值班经理随后签发带校验哈希的唯一不可变启用许可，并可按许可编码现场核验冻结内容、版本和审计引用。所有变更均写入带哈希链的本地审计账本。

## 构建、运行与测试

安装 Go 1.22 或更高版本后，在项目根目录执行：

```text
go test ./...
go run ./cmd/stageguard -selfcheck -addr=127.0.0.1:19081
```

启动浏览器工作台：

```text
go run ./cmd/stageguard -addr=127.0.0.1:19081 -data stageguard-data.json
```

打开 `http://127.0.0.1:19081/`。监听地址也可以通过 `PORT` 环境变量设置端口；服务只接受回环地址，不会默认绑定外部网卡。

## 主要能力与 API

- `GET /api/dossiers` 支持 `status`、`venue`、`keyword`、`from`、`to`、`unissued` 组合筛选，并返回检查、问题、待复核和许可摘要；`PATCH /api/dossiers/{id}` 使用 `expectedVersion` 修订无检查记录的草稿档案。
- `GET /api/dossiers/{id}/inspections` 返回设备检查矩阵和缺失项；`POST` 支持单项更正或不超过 200 行的原子批次。批次通过 `Idempotency-Key` 安全重试，档案版本只递增一次。
- `POST /api/dossiers/{id}/issues/check` 在矩阵完整后执行稳定问题键对账，可携带结构化复测值，并返回新增、持续、待确认消除和重开计数。
- `POST /api/issues/{id}/remediation` 保存不可覆盖的整改修订和证据元数据；`GET /api/issues/{id}/revisions/{revision}` 返回指定修订及相邻版本证据差异。
- `POST /api/issues/{id}/review` 逐项通过或退回；`POST /api/dossiers/{id}/freeze` 返回结构化阻塞问题，只有所有决定对应最新整改修订时才冻结。
- `POST /api/dossiers/{id}/permit` 签发每档案唯一许可；`GET /api/permits/{code}` 精确查询，`GET /api/permits/{code}/verify` 只读核验内容哈希、冻结版本和审计引用。
- `GET /api/dossiers/{id}/audit` 支持 `type`、`page` 和 `pageSize` 筛选及有界分页，事件按时间倒序稳定返回。

所有变更请求都应携带当前 `expectedVersion`，版本冲突返回 HTTP 409 和最新版本号。主快照以原子替换方式保存，配套的 `.events.jsonl` 文件记录并校验审计哈希链。
