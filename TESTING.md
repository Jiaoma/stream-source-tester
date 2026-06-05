# TESTING

## 常用命令

- 全量测试：`make test`
- RTSP 测试：`make test-rtsp`
- RTP/UDP 测试：`make test-rtp`
- Mutation 测试：`make test-mutation`
- 全量构建：`make build`
- 代码格式化：`make fmt`
- 本地一键检查：`./scripts/ci-local.sh`

也可以直接使用 Go 命令：

- `go test ./...`
- `go build ./...`

## 已实现功能与测试映射

### 1. 配置与计划构建
- 文件：`internal/config/config.go`、`internal/app/app.go`
- 测试：`internal/config/config_test.go`、`internal/app/app_test.go`
- 覆盖内容：默认值、配置加载、profile 解析、mutation 顺序保留、profile 组合应用

### 2. 输入探测（MP4 / PCAP）
- 文件：`internal/input/builtin/mp4/source.go`、`internal/input/builtin/pcap/source.go`
- 测试：`internal/input/builtin/mp4/source_test.go`、`internal/input/builtin/pcap/source_test.go`
- 覆盖内容：文件头校验、MP4 ftyp 信息、PCAP global header 信息

### 3. Mutation 执行
- 文件：`internal/mutation/builtin/*`
- 测试：`internal/mutation/builtin/setmarker/mutator_test.go`、`internal/output/builtin/rtpudp/mutation_integration_test.go`、`internal/app/app_test.go`
- 覆盖内容：marker、sequence、timestamp 改写；全丢包、选择性丢包、乱序；组合 mutation

### 4. RTP/UDP 输出
- 文件：`internal/output/builtin/rtpudp/*`
- 测试：`internal/output/builtin/rtpudp/sink_test.go`、`internal/output/builtin/rtpudp/multistream_test.go`、`internal/output/builtin/rtpudp/mutation_integration_test.go`
- 覆盖内容：最小真实发包、timeline 调度、多 stream 发包、mutation 对网络输出生效

### 5. RTSP 控制面
- 文件：`internal/output/builtin/rtsp/*`
- 测试：`internal/output/builtin/rtsp/sink_test.go`、`internal/output/builtin/rtsp/multitrack_test.go`、`internal/output/builtin/rtsp/sharedlistener_test.go`
- 覆盖内容：OPTIONS / DESCRIBE / SETUP / PLAY / TEARDOWN、SETUP 协商 client_port、PLAY 触发 RTP、shared listener、多 mount、多 track SDP、audio/video SDP

### 6. Session / Manager
- 文件：`internal/output/runtime.go`、`internal/output/manager.go`
- 测试：`internal/output/manager_test.go`
- 覆盖内容：session 注册、查询、关闭、CloseAll、serving 状态可见

## 当前测试执行情况

当前基线：
- `go test ./...`：通过
- `go build ./...`：通过

说明：
- `internal/output/builtin/rtsp/sink_test.go` 中 `TestRequestAfterTeardownReturns454` 当前为 skip，用于记录 shared listener 之后仍需重构的会话关闭语义。

## 回归场景配置

示例文件：`examples/anomaly-scenarios.yaml`

当前包含的 profile：
- `normal-udp`
- `marker-and-seq`
- `timestamp-and-drop`
- `reorder-over-rtsp`

这些 profile 已由回归测试加载并进行基本校验。后续可以扩展为更严格的端到端场景回放。 
