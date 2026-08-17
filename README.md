# luci-app-miair

轻量级 OpenWrt / LuCI 小爱音箱 AirPlay 与 DLNA / UPnP 投放桥接插件。

当前版本：见 [`VERSION`](VERSION)

作者：[Shawn Rain](https://github.com/ShawnRn)

项目主页：[github.com/ShawnRn/luci-app-miair](https://github.com/ShawnRn/luci-app-miair)

### 特性
- **轻量原生**：Go 语言自研核心守护进程，体积小（< 7MB），无 Docker / Python 依赖。
- **AirPlay 1 协议支持**：支持 iPhone / iPad / Mac 隔空播放音频串流。
- **Mac 音量联动**：将 macOS / iOS AirPlay 音量控制同步到小爱音箱，并合并快速滑动产生的连续请求。
- **Android DLNA / UPnP**：提供 MediaRenderer，可被支持 DLNA 投放的 Android 播放器发现与控制；不包含 Miracast 或 Chromecast。
- **多设备切换策略**：支持最新设备优先、锁定当前设备、空闲后接管，以及 AirPlay / DLNA 协议优先级。
- **扫码授权**：通过米家 / 小米 App 扫码登录，无需在路由器中保存账号密码。
- **可调音频预缓冲**：可在 LuCI 中设置 0–5000 ms，减少音箱接入时丢失开头音频。
- **固定无损 PCM 输出**：44.1 kHz / 16-bit / 双声道，约 1411 kbps，无需额外转码依赖。
- **原生 LuCI 界面**：适配 OpenWrt / QWRT 路由器 Web 管理界面，并显示当前协议、音源与切换策略。

### 音频参数说明

- AirPlay 接收端解码后固定输出 44.1 kHz / 16-bit / 双声道 PCM，码率约 1411 kbps。
- LuCI 中可自定义 0–5000 ms 的预缓冲，以在延迟、内存占用和开头完整度之间取舍。
- 当前版本不提供码率调节：降低 PCM 码率需要引入重采样或有损转码，会增加路由器 CPU、依赖与延迟。

### 致谢

本项目的创意、小爱音箱桥接工作流与管理界面设计参考了以下项目：

- [KiriChen-Wind/MiAir](https://github.com/KiriChen-Wind/MiAir)
- [deerwan/miair-next](https://github.com/deerwan/miair-next)

感谢原作者与相关开源项目贡献者的探索和分享。

### 目录结构
- `core/`: Go 编写的核心守护进程源码与交叉编译脚本。
- `luasrc/`: LuCI 控制器与 CBI 模型界面。
- `root/`: OpenWrt 服务配置文件及 procd 守护脚本。

### 编译与安装
```bash
# 交叉编译核心
cd core
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o miair-core .
```

发布包由根目录 `VERSION` 统一管理版本号，并通过以下命令构建：

```bash
chmod +x scripts/build-packages.sh
APK=/path/to/apk-tools-v3 scripts/build-packages.sh
```

- OpenWrt 24.10 及更早的 opkg 系统安装 `.ipk`。
- OpenWrt 25.12 及更新的 apk-tools v3 系统使用 `apk --allow-untrusted add <package.apk>` 安装 `.apk`。
