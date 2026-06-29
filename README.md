# stream-source-tester

一个用于协议互操作与异常行为测试的最小流源工具。

当前仓库已经具备：
- 读取本地 MP4 / PCAP 输入
- 生成最小 `SessionBundle`
- 通过 RTSP 暴露控制面
- 通过 RTP/UDP 发送最小数据面
- 支持一组可组合的 mutation

> 当前实现仍然偏“最小可运行测试平台”，不是完整商用 RTSP server。下面的 get started 以**最简单、最贴近当前能力**的方式说明如何开始。

---

## Get Started

### 1. 准备一个本地输入文件

你可以提供：
- 一个本地 `.mp4` 文件
- 或一个本地 `.pcap` 文件

例如：
- `./fixtures/sample.mp4`
- `./fixtures/sample.pcap`

如果你用自己的文件，假设路径如下：
- `/path/to/input.mp4`
- `/path/to/input.pcap`

---

### 2. 直接使用仓库自带的最小配置

仓库已经提供了两个可直接运行的示例配置：

- `examples/quickstart-rtsp.yaml`（MP4 -> RTSP）
- `examples/quickstart-pcap-rtsp.yaml`（PCAP -> RTSP）

默认输入文件分别是：

- `./fixtures/sample.mp4`
- `./fixtures/sample.pcap`

如果你要换成自己的本地文件，只需要把这个文件里的：

```yaml
location: ./fixtures/sample.mp4
```

改成你的实际路径，例如：

```yaml
location: /path/to/input.mp4
```

下面是它的完整内容，方便你理解：

```yaml
name: quickstart-rtsp
server:
  rtspPort: 8554
inputs:
  - name: local-input
    kind: mp4
    codec: h264
    location: /path/to/input.mp4
outputs:
  - name: rtsp-out
    kind: rtsp
    target: rtsp://127.0.0.1:8554/test
mutations:
  - name: passthrough
    kind: identity
    enabled: true
profiles:
  - name: normal-rtsp
    input: local-input
    output: rtsp-out
    mutations: [passthrough]
```

如果输入是 PCAP，可以把 `kind` 和 `location` 改成：

```yaml
inputs:
  - name: local-input
    kind: pcap
    codec: h264
    location: /path/to/input.pcap
```

---

### 3. 启动程序

在项目根目录执行：

```bash
go run ./cmd/stream-source-tester -config ./examples/quickstart-rtsp.yaml
```

如果你想使用 PCAP 版本：

```bash
go run ./cmd/stream-source-tester -config ./examples/quickstart-pcap-rtsp.yaml
```

或者使用仓库自带的一键启动脚本：

```bash
./scripts/run-quickstart.sh
```

如果要指定别的配置文件：

```bash
./scripts/run-quickstart.sh ./examples/quickstart-pcap-rtsp.yaml
```

或者先编译：

```bash
go build ./cmd/stream-source-tester
./stream-source-tester -config ./examples/quickstart-rtsp.yaml
```

程序启动后会：
- 加载配置
- 构建 profile
- 打开输入文件
- 启动 RTSP listener
- 在收到 RTSP `PLAY` 后触发 RTP 数据发送

---

### 4. 用 VLC 播放

> 当前仓库已经验证过：**本地 MP4 + `examples/quickstart-rtsp.yaml` + VLC** 这条正常播放路径是可用的。

在 VLC 中打开下面这个 URL：

```text
rtsp://127.0.0.1:8554/test
```

如果你使用的是别的播放器，只要它支持标准 RTSP，也可以使用相同 URL。

---

## 快速测试用的最小示例

最简单、当前最推荐、并且已经用 VLC 验证通过的运行方式就是：

```bash
go run ./cmd/stream-source-tester -config ./examples/quickstart-rtsp.yaml
```

然后在 VLC 打开：

```text
rtsp://127.0.0.1:8554/test
```

如果你要测试自己的文件，只需要编辑：

- `examples/quickstart-rtsp.yaml`

把其中的 `location` 改成你自己的本地 MP4 路径。

如果你要做协议回放或异常测试，也可以使用：

- `examples/quickstart-pcap-rtsp.yaml`

但当前**最适合播放器正常播放验证**的路径仍然是 MP4 -> RTSP。

---

## 当前实现边界说明

为了避免误解，下面这些点你需要知道：

1. 当前实现是**最小 RTSP/RTP 测试平台**，不是完整媒体服务器。
2. RTSP 控制面已经支持：
   - `OPTIONS`
   - `DESCRIBE`
   - `SETUP`
   - `PLAY`
   - `TEARDOWN`
   - `GET_PARAMETER`
   - `SET_PARAMETER`
   - Basic 鉴权登录（可选开启）
3. RTP 数据面已经支持：
   - timeline 调度发送
   - RTP/UDP 真实发包
   - RTSP `PLAY` 触发 RTP
4. 当前更适合：
   - 协议联调
   - 最小播放器连通性验证
   - mutation / 异常场景测试
5. 当前还不是“完整解码器级视频服务器”，所以不同播放器对媒体内容的容忍度可能不同。

---

## 虚拟相机（鉴权 + 参数协商）

除了普通流源，本项目也可以把一个 MP4/PCAP 包装成“虚拟相机”：对外提供需要登录的 RTSP 服务，并支持参数获取/设置。

示例配置：

- `examples/virtual-camera-rtsp.yaml`

它在 RTSP output 的 `options` 中开启了 Basic 鉴权：

```yaml
outputs:
  - name: camera-rtsp
    kind: rtsp
    target: rtsp://127.0.0.1:8554/camera
    options:
      auth.mode: basic
      auth.username: admin
      auth.password: secret
      auth.realm: virtual-camera
```

行为说明：

- `OPTIONS` 允许匿名，方便客户端发现能力。
- `DESCRIBE` / `SETUP` / `PLAY` / `SET_PARAMETER` / `GET_PARAMETER` 需要鉴权。
- 未鉴权或鉴权失败会返回 `401 Unauthorized`，并带 `WWW-Authenticate: Basic realm="..."`。
- `SET_PARAMETER`：
  - 空 body 当作 keepalive，返回 `200 OK`。
  - 形如 `framerate: 30` 的赋值会被记录到会话参数。
  - 不在允许列表内的参数返回 `451 Parameter Not Understood`。
  - 当前允许的参数：`framerate` / `bitrate` / `resolution` / `gop` / `brightness` / `contrast`。
- `GET_PARAMETER`：
  - 空 body 当作 keepalive。
  - 查询已知参数会在响应 body 中回显当前值。

启动虚拟相机：

```bash
go run ./cmd/stream-source-tester -config ./examples/virtual-camera-rtsp.yaml
```

用 VLC 打开（带凭据）：

```text
rtsp://admin:secret@127.0.0.1:8554/camera
```

### 基于 pcap 的虚拟相机

如果你有真实相机的 tcpdump 抓包文件，也可以用它创建虚拟相机：

示例配置：

- `examples/virtual-camera-pcap.yaml`

它的输入是 pcap 文件：

```yaml
inputs:
  - name: camera-pcap-input
    kind: pcap
    codec: h264
    location: ./fixtures/your-camera-dump.pcap
```

使用方式：

1. 用 `tcpdump` 或 Wireshark 抓取真实相机的 RTP/RTSP 流量：
   ```bash
   tcpdump -i en0 -w camera-dump.pcap host <camera-ip>
   ```

2. 把 pcap 文件放到 `fixtures/` 或任意路径。

3. 修改 `examples/virtual-camera-pcap.yaml` 的 `location` 指向你的 pcap 文件。

4. 启动虚拟相机：
   ```bash
   go run ./cmd/stream-source-tester -config ./examples/virtual-camera-pcap.yaml
   ```

5. 用 VLC 带凭据连接：
   ```text
   rtsp://camera:secret@127.0.0.1:8555/camera
   ```

这样你就可以把真实相机的抓包"重放"成一个带鉴权的虚拟 RTSP 相机，用于测试客户端的鉴权、重连、参数协商等行为。


---

## Troubleshooting

### 1. VLC 无法打开 `rtsp://127.0.0.1:8554/test`

请先确认：
- 程序是否已经启动
- 终端里没有报配置或输入文件错误
- 你打开的 URL 和配置中的 `target` 一致

### 2. 端口被占用

如果 `8554` 已被别的程序占用，可以修改配置中的：

```yaml
target: rtsp://127.0.0.1:8554/test
```

例如改成：

```yaml
target: rtsp://127.0.0.1:9554/test
```

然后在 VLC 里也使用对应的新 URL。

### 3. 输入文件路径错误

如果启动时报找不到文件，请检查配置里的：

```yaml
location: ...
```

建议先用绝对路径验证，再决定是否改回相对路径。

### 4. MP4 / PCAP 类型填错

- `.mp4` 文件要用：`kind: mp4`
- `.pcap` 文件要用：`kind: pcap`

如果类型填错，输入探测阶段会直接失败。

---

## 常用开发命令

### 运行全部测试

```bash
make test
```

### 本地一键检查

```bash
./scripts/ci-local.sh
```

它会依次执行：
- `gofmt`
- `go test ./...`
- `go build ./...`

### 只跑 RTSP 测试

```bash
make test-rtsp
```

### 只跑 RTP/UDP 测试

```bash
make test-rtp
```

### 只跑 mutation 测试

```bash
make test-mutation
```

---

## 下一步建议

如果你希望把这个工具更稳定地用于播放器验证，下一步建议优先做：
- 更真实的媒体数据生成
- 更完整的 SDP / fmtp 参数
- 更强的 RTSP 会话状态控制
- 更多正常流与异常流场景模板
