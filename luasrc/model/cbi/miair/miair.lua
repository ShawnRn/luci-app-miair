local m, s, o

m = Map("miair", translate("MiAir 小爱音箱投放桥接"), translate("将小爱音箱接入苹果 AirPlay 与 Android 常用的 DLNA / UPnP 投放系统，并管理多个发送设备之间的音源切换。"))

-- 顶部插入运行状态与扫码区域
local status_sec = m:section(NamedSection, "config", "miair")
status_sec.anonymous = true
status_sec.addremove = false
status_sec.template = "miair/status"

s = m:section(NamedSection, "config", "miair", translate("基本配置"))
s.anonymous = true
s.addremove = false

o = s:option(Flag, "enabled", translate("启用服务"))
o.default = "1"
o.rmempty = false

o = s:option(Value, "name", translate("投放显示名称"))
o.description = translate("iPhone、Mac 与 Android DLNA 应用搜索投放设备时看到的名称")
o.default = "小爱音箱投放"
o.rmempty = false

o = s:option(Flag, "airplay_enabled", translate("启用 AirPlay"))
o.default = "1"
o.rmempty = false

o = s:option(Flag, "dlna_enabled", translate("启用 DLNA / UPnP"))
o.description = translate("让支持 DLNA MediaRenderer 的 Android 应用发现并投送音频")
o.default = "1"
o.rmempty = false

o = s:option(Value, "did", translate("小爱音箱 Device ID (DID)"))
o.description = translate("指定绑定的小爱音箱 DID。点击上方【扫码授权登录】或【刷新/自动识别】可自动填充；若留空，插件启动时将自动绑定名下第一个音箱。")
o.rmempty = true

s_adv = m:section(NamedSection, "config", "miair", translate("高级参数"))
s_adv.anonymous = true
s_adv.addremove = false

o = s_adv:option(Value, "port", translate("AirPlay RTSP 端口"))
o.default = "5000"
o.datatype = "port"
o.rmempty = true
o:depends("airplay_enabled", "1")

o = s_adv:option(Value, "buffer_ms", translate("音频预缓冲（毫秒）"))
o.default = "500"
o.datatype = "range(0,5000)"
o.description = translate("保留最近一段 PCM 音频，供小爱音箱连接后快速填满播放缓冲。局域网环境建议设为 300~500ms（起播更灵敏）；若 Wi-Fi 偶发卡顿可适当调大（如 1000~1500ms）。0 表示关闭缓冲。")
o.rmempty = true
o:depends("airplay_enabled", "1")

o = s_adv:option(Value, "http_port", translate("HTTP 音频流中转端口"))
o.default = "8300"
o.datatype = "port"
o.rmempty = true
o:depends("airplay_enabled", "1")

o = s_adv:option(Value, "dlna_port", translate("DLNA 控制与媒体代理端口"))
o.default = "8301"
o.datatype = "port"
o.rmempty = true
o:depends("dlna_enabled", "1")

s_switch = m:section(NamedSection, "config", "miair", translate("多设备音源切换"))
s_switch.anonymous = true
s_switch.addremove = false

o = s_switch:option(ListValue, "source_policy", translate("切换策略"))
o:value("latest", translate("最新设备优先（推荐）"))
o:value("lock", translate("当前设备锁定"))
o:value("idle", translate("空闲后允许接管"))
o:value("priority", translate("按协议优先级"))
o.default = "latest"
o.description = translate("最新设备优先会终止旧投送会话并立即切换；当前设备锁定会向新设备返回忙碌状态。")
o.rmempty = false

o = s_switch:option(Value, "idle_timeout", translate("音源空闲超时（秒）"))
o.default = "10"
o.datatype = "range(1,3600)"
o.rmempty = true
o:depends("source_policy", "idle")

o = s_switch:option(ListValue, "preferred_protocol", translate("优先协议"))
o:value("airplay", translate("AirPlay 优先"))
o:value("dlna", translate("DLNA 优先"))
o.default = "airplay"
o.rmempty = true
o:depends("source_policy", "priority")

return m
