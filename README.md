# 三相平衡稳态 N-1 停运筛查后端

使用 Go 与 Chi 实现的纯后端服务。它对给定交流网络先求解基态，再按输入顺序逐一停运每条线路，独立完成 N-1 筛查：

- 线路采用正序（三相平衡）π 模型：串联阻抗 `r+jx`，总充电电纳 `b` 两端各分 `b/2`。
- 采用极坐标 Newton-Raphson 交流潮流；不是直流潮流近似。
- PV 母线固定有功净注入和电压幅值，slack 母线固定完整电压相量。
- 不建模发电机无功限值、变压器分接头或相位移动。
- 奇异矩阵或达到迭代上限未收敛时返回 `undetermined`，不会伪造有效电压或潮流。

## 目录结构

- `cmd/server/main.go`：HTTP 服务入口。
- `internal/api/server.go`：Chi 路由、JSON 解码、未知字段拒绝和错误响应。
- `internal/pf/model.go`：输入模型、字段合法性与有限值校验。
- `internal/pf/network.go`：母线索引、停运拓扑、连通性和复导纳矩阵。
- `internal/pf/solver.go`：极坐标 Newton-Raphson、Jacobian 和部分主元高斯消元。
- `internal/pf/flow.go`：双端线路功率、实功/无功损耗、过载与电压越限。
- `internal/pf/screening.go`：基态与逐线独立 N-1 编排和响应 DTO。
- `examples/small_network.json`：4 母线、5 线路小网络示例。

## 单位与输入约定

- `base_mva`：全网统一功率基准，单位 MVA。
- 母线 `p_mw`、`q_mvar`：净注入功率；发电为正、负荷为负，MW/MVAr。
- `voltage_pu`：电压幅值标幺；`angle_deg`：角度，单位度。
- 线路 `r_pu`、`x_pu`、`b_pu`：以请求中的 `base_mva` 为基准的标幺参数。
- `capacity_mva`：线路 MVA 容量，必须为正。
- 约束：2 至 12 个母线、最多 24 条线路；全网仅一个 slack；`r>=0`、`x>0`、`b>=0`；拒绝自环、重复 ID、未知母线、非法枚举、非有限值和未知 JSON 字段。

`max_iterations` 默认为 30，`tolerance_pu` 默认为 `1e-10`，二者可在请求中配置。容差按功率失配的标幺最大绝对值判断。

## 状态含义

- `safe`：潮流收敛，所有母线连通 slack，且无电压或线路 MVA 越限。
- `violation`：潮流收敛但至少有一个母线电压越界，或一条线路某端视在功率超过容量。越限按母线和线路双端逐项列出。
- `deenergized`：停运后有母线与 slack 无电气连接；这些断供情景不会标记为安全，也不输出伪造潮流。
- `undetermined`：Jacobian 奇异、更新无效或迭代上限内未收敛。单个情景失败不影响后续停运情景。

## API

### 健康检查

```bash
GET /healthz
```

### 执行筛查

```bash
POST /api/v1/screen
Content-Type: application/json
```

响应包含：

- `base_case`：完整网络的求解结果。
- `contingencies`：按输入线路顺序排列的单线路停运结果。
- `voltages`：每个母线的电压标幺和角度。
- `line_flows`：每条在运线路两端流入的 P/Q、双端 MVA 和实功损耗。
- `max_power_residual_pu`、`max_power_residual_mva`：最大功率失配。
- `violations`：`bus_voltage` 或 `line_mva` 越限对象、实际值与限值。

## 运行、测试与 curl 示例

```bash
# 测试与静态检查
go test ./...
go vet ./...

# 构建
go build -o bin/n1-screen ./cmd/server

# 启动（默认 :8080，可用 -addr 覆盖）
./bin/n1-screen -addr :18081

# 另一个终端执行示例
curl -s http://127.0.0.1:18081/healthz
curl -s -X POST http://127.0.0.1:18081/api/v1/screen \
  -H 'Content-Type: application/json' \
  --data @examples/small_network.json | python3 -m json.tool
```

示例基态应收敛为 `safe`；停运 `L12` 后会观察到 `L13` 双端视在功率超过 90 MVA，状态为 `violation`。
