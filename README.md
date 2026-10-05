# AC N-1 Contingency Screening

三相平衡稳态交流潮流 N-1 停运筛查后端（无前端）。基于线路 π 模型组装复导纳矩阵，
用极坐标 Newton-Raphson 求解交流功率平衡（非直流近似）。先求基态，再按输入顺序
逐条线路停运，每个情景都从平启动独立求解，失败情景不影响后续情景。

## 单位与约定

- 功率：净注入，发电为正、负荷为负，单位 MW / MVAr（请求与响应均为有名值）。
- 电压：标幺值（pu）；角度：度（请求与响应），内部计算用弧度。
- 线路参数 `r_pu` / `x_pu` / `b_pu`：基准容量 `base_mva` 下的标幺值；
  `b_pu` 为总充电电纳，π 模型两端各挂一半。
- 母线类型：`PQ`（给 `p_mw`、`q_mvar`）、`PV`（给 `p_mw`、`v_pu`）、
  `slack`（给 `v_pu`、`theta_deg`），全网必须恰好一个 slack。
- 约束：2–12 个母线、最多 24 条线路；`r>=0`、`x>0`、`b>=0`、`rate_mva>0`；
  允许并联线路。非法字段、非有限值、自环、重复 ID、未知母线一律返回 400。
- 不含机组无功限值与变压器。

## 运行

```sh
go build -o bin/server .
ADDR=:8080 ./bin/server        # ADDR 缺省 :8080
```

测试：

```sh
go test ./...
```

## API

- `GET /healthz` — 健康检查。
- `POST /api/v1/screen` — 请求体为算例 JSON（见 `examples/case5.json`）。
  可选字段 `max_iter`（默认 50）与 `tolerance`（标幺残差容差，默认 1e-8）。

```sh
curl -s -X POST localhost:8080/api/v1/screen --data @examples/case5.json | jq .
```

## 响应与状态

响应包含 `base`（基态）与 `outages`（按输入顺序的逐线停运情景）。每个情景：

- `status`：
  - `safe` — 收敛、无超限、无母线脱离 slack（无断供）。
  - `violated` — 收敛但存在线路过载 / 电压越界，或有母线断供。
  - `undetermined` — 雅可比奇异或迭代不收敛；不伪造有效解，`reason` 说明原因。
- `islanded_buses`：脱离 slack 的母线（断供），此类情景绝不标记为 `safe`。
- `buses`：各母线电压幅值（pu）与相角（度）。
- `lines`：各在运线路两端流入的 P/Q（MW/MVAr）与实功损耗 `loss_mw`。
- `max_mismatch_pu`：收敛时最大标幺功率残差。
- `violations`：逐项超限对象 —— `line_overload`（双端视在功率较大者 vs `rate_mva`）
  与 `voltage`（母线幅值 vs 其上下限）。

## 代码结构

- `types.go` — 请求模型与求解配置。
- `validate.go` — 输入校验。
- `ybus.go` — 拓扑（孤岛检测）与 π 模型复导纳组装。
- `solver.go` — 极坐标 Newton-Raphson 与高斯消元。
- `outage.go` — 情景求解、潮流/损耗计算、超限判定与 N-1 筛查。
- `server.go` / `main.go` — Chi HTTP 路由与入口。
- `internal/cx` — 复数运算。
- `examples/case5.json` — 5 母线示例（基态安全；L13/L23/L34 停运导致 L24 过载；
  L45 停运使 B5 断供且方程奇异，标记 `undetermined`）。
