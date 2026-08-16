local m, s, o
local fs = require "nixio.fs"
local sys = require "luci.sys"

m = Map("miair", translate("MiAir 小爱音箱 AirPlay 桥接"), translate("将小爱音箱（包括无 DLNA 功能的型号）无缝接入苹果 AirPlay 隔空播放系统。"))

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

o = s:option(Value, "name", translate("AirPlay 显示名称"))
o.description = translate("iPhone / iPad / Mac 等设备搜寻隔空播放时看到的音箱名称")
o.default = "小爱音箱 AirPlay"
o.rmempty = false

o = s:option(Value, "did", translate("小爱音箱 Device ID (DID)"))
o.description = translate("指定绑定的小爱音箱 DID。点击上方【扫码授权登录】或【刷新/自动识别】可自动填充；若留空，插件启动时将自动绑定名下第一个音箱。")
o.rmempty = true

s_acc = m:section(NamedSection, "config", "miair", translate("账号密码备选（推荐直接使用上方扫码登录）"))
s_acc.anonymous = true
s_acc.addremove = false

o = s_acc:option(Value, "account", translate("小米账号 (手机号/小米ID)"))
o.rmempty = true

o = s_acc:option(Value, "password", translate("小米密码"))
o.password = true
o.rmempty = true

s_adv = m:section(NamedSection, "config", "miair", translate("高级参数"))
s_adv.anonymous = true
s_adv.addremove = false

o = s_adv:option(Value, "port", translate("AirPlay RTSP 端口"))
o.default = "5000"
o.datatype = "port"
o.rmempty = false

o = s_adv:option(Value, "http_port", translate("HTTP 音频流中转端口"))
o.default = "8300"
o.datatype = "port"
o.rmempty = false

return m

