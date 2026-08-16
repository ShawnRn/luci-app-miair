local m, s, o
local fs = require "nixio.fs"
local sys = require "luci.sys"

m = Map("miair", translate("MiAir 小爱音箱 AirPlay 桥接"), translate("通过本插件可将您的小爱音箱（包括未开放 DLNA 的型号）无缝接入苹果 AirPlay 隔空播放。"))

s = m:section(NamedSection, "config", "miair", translate("基本配置"))
s.anonymous = true
s.addremove = false

o = s:option(Flag, "enabled", translate("启用"))
o.rmempty = false

o = s:option(Value, "name", translate("AirPlay 显示名称"))
o.description = translate("iPhone / Mac 搜寻隔空播放时看到的设备名称")
o.default = "小爱音箱 AirPlay"
o.rmempty = false

o = s:option(Value, "account", translate("小米账号 (手机号/小米ID)"))
o.rmempty = true

o = s:option(Value, "password", translate("小米密码"))
o.password = true
o.rmempty = true

o = s:option(Value, "did", translate("小爱音箱 Device ID (DID)"))
o.description = translate("小爱音箱的 DID 设备号（选填，未填写时插件启动时将自动识别并绑定名下的第一个小爱音箱）")
o.rmempty = true

s = m:section(NamedSection, "config", "miair", translate("高级参数"))
s.anonymous = true
s.addremove = false

o = s:option(Value, "port", translate("AirPlay RTSP 端口"))
o.default = "5000"
o.datatype = "port"
o.rmempty = false

o = s:option(Value, "http_port", translate("HTTP 音频流中转端口"))
o.default = "8300"
o.datatype = "port"
o.rmempty = false

return m
