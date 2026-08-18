# AGENTS.md - luci-app-miair 开发与维护指南

本文档为 AI Agent 及开发者维护 **luci-app-miair**（OpenWrt 小爱音箱 AirPlay 与 DLNA 投播桥接插件）提供完整的设计规范、代码架构、开发流程与排错参考。

---

## 1. 项目概览与架构

**luci-app-miair** 是一个运行在 OpenWrt 路由器上的完整投播桥接服务，包含高性能 Go 核心守护进程与 LuCI 现代化 Web 交互界面：

```
[ 发送端: iPhone / Mac / Android ]
          │  AirPlay 1 (RTSP/RTP) / DLNA (UPnP/MediaProxy)
          ▼
┌─────────────────────────────────────────────────────────────┐
│  路由器端: miair-core (Go 守护进程, 运行在 /usr/bin/miair-core)   │
│  ├── airplay/   : RTSP 握手、RTP 数据接收、AES 解密与 ALAC 解码 │
│  ├── dlna/      : SSDP 广播发现、AVTransport/Rendering 控制与代理 │
│  ├── playback/  : 多音源调度协调器 (Coordinator) 与实时状态输出  │
│  ├── source/    : 抢占策略管理器 (latest / lock / idle / priority)│
│  └── miservice/ : 小米云端 API (扫码登录、Token 6h 主动保活刷新)  │
└──────────────────────────────┬──────────────────────────────┘
                               │ HTTP WAV 实时流 (192.168.10.1:8300)
                               ▼
┌─────────────────────────────────────────────────────────────┐
│  小爱音箱硬件端 (通过小米云端 Mina/UBUS 协议由 miair-core 控制起播) │
└─────────────────────────────────────────────────────────────┘
```

---

## 2. 目录结构说明

```
luci-app-miair/
├── VERSION                        # 语义化版本号 (例如 1.1.2)
├── core/                          # Go 后台核心守护进程源码
│   ├── main.go                    # 程序入口与命令行参数解析
│   ├── airplay/                   # AirPlay 1 (RAOP) 引擎
│   ├── dlna/                      # DLNA MediaRenderer 引擎
│   ├── miservice/                 # 小米云端账号与播放控制 API
│   ├── playback/                  # 播放协调器 (Coordinator)
│   ├── source/                    # 音源抢占策略管理器
│   └── pkg_alac/                  # 纯 Go ALAC 苹果无损音频解码库
├── luasrc/                        # LuCI Web 界面源码
│   ├── controller/miair.lua       # 路由分发与 REST API (status, qr, devices)
│   ├── model/cbi/miair/miair.lua  # CBI 表单配置模型
│   └── view/miair/status.htm      # 实时状态展示与扫码登录前端模板
├── root/                          # OpenWrt 系统集成文件
│   ├── etc/config/miair           # UCI 默认配置文件
│   └── etc/init.d/miair           # Procd 服务启动脚本
└── scripts/
    └── build-packages.sh          # IPK / APK 打包脚本 (Python 3 tarfile 归档)
```

---

## 3. 关键开发规范与避坑指南

### 3.1 LuCI 与 Lua 编程规范
1. **文件读写**：
   - **禁止使用** `nixio.fs.readfile`（OpenWrt 官方 nixio 库中不存在该方法）。
   - **必须使用** 标准 Lua `io.open(path, "r")` 封装安全读取函数。
2. **CBI 表单与条件联动 (`depends`)**：
   - 任何带有 `o:depends(...)` 条件联动的选项（如 `idle_timeout`、`preferred_protocol`、`port`、`buffer_ms`），其 `rmempty` 属性**必须设为 `true`**。
   - 若设为 `false`，当选项因联动规则在前端被动态隐藏时，LuCI 提交时会判定为缺少必填项，导致 `一个或多个必选项值为空！无法保存`。

### 3.2 Go 核心与音频传输优化
1. **超低延迟起播**：
   - 在 `core/airplay/server.go` 的 HTTP 音频流分发中，小爱音箱连接时采用**首包批量合并写入并单次 Flush**，快速填满音箱播放器初始缓冲水位线，大幅缩短起播延迟。
   - 必须在 HTTP 响应头携带 `Accept-Ranges: none`，防止播放器进行无意义的 Range 探测。
2. **Token 主动保活与自愈**：
   - 启动时在 `account.StartAutoRefresh()` 中主动预热换取 `serviceToken`。
   - 后台内置 ticker 每 6 小时自动向小米账号中心刷新，状态实时写入 `/var/run/miair-status.json`。
   - 遇到 401 Unauthorized 时执行自动重试自愈。

---

## 4. 常用维护与构建命令

### 4.1 本地测试与跨平台编译
```bash
# 1. 运行 Go 全套单元测试
cd core && go test -v ./...

# 2. 编译 Linux ARM64 核心 (OpenWrt 路由器)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w -X main.version=$(cat ../VERSION)" -o miair-core .
```

### 4.2 生成 IPK 安装包
```bash
# 在项目根目录下执行打包脚本
./scripts/build-packages.sh
# 产物生成在: dist/luci-app-miair_<VERSION>-1_aarch64_cortex-a53.ipk
```

### 4.3 部署与实机验证 (路由器)
```bash
# 上传并重新安装
sshpass -p '<PASSWORD>' scp -O -o StrictHostKeyChecking=no dist/luci-app-miair_*.ipk root@192.168.10.1:/tmp/
sshpass -p '<PASSWORD>' ssh -o StrictHostKeyChecking=no root@192.168.10.1 "opkg install --force-reinstall /tmp/luci-app-miair_*.ipk; /etc/init.d/miair restart; rm -f /tmp/luci-indexcache*"

# 检查服务运行状态与日志
sshpass -p '<PASSWORD>' ssh root@192.168.10.1 "cat /var/run/miair-status.json; logread | grep miair | tail -n 30"
```
