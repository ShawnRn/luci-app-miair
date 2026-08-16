module("luci.controller.miair", package.seeall)

function index()
	if not nixio.fs.access("/etc/config/miair") then
		return
	end

	entry({"admin", "services", "miair"}, cbi("miair/miair"), _("MiAir 小爱投播"), 70).dependent = true
	entry({"admin", "services", "miair", "get_devices"}, call("action_get_devices")).leaf = true
	entry({"admin", "services", "miair", "status"}, call("action_status")).leaf = true
end

function action_status()
	local running = (luci.sys.call("pidof miair-core >/dev/null") == 0)
	luci.http.prepare_content("application/json")
	luci.http.write_json({
		running = running
	})
end

function action_get_devices()
	local http = require "luci.http"
	local uci = require("luci.model.uci").cursor()
	local user = http.formvalue("user") or uci:get("miair", "config", "mi_user") or ""
	local pass = http.formvalue("pass") or uci:get("miair", "config", "mi_pass") or ""
	local store = uci:get("miair", "config", "mi_token_store") or "/etc/miair/token.json"

	http.prepare_content("application/json")

	if user == "" or pass == "" then
		http.write_json({ code = 1, msg = "请先输入小米账号与密码" })
		return
	end

	local cmd = string.format('/usr/bin/miair-core -list -user "%s" -pass "%s" -store "%s" 2>&1', user, pass, store)
	local handle = io.popen(cmd)
	local result = handle:read("*a")
	handle:close()

	local devices = {}
	for line in string.gmatch(result, '[^%r%n]+') do
		local did, name, hw, ip = string.match(line, 'DID:%s*([^|]+)%s*|%s*Name:%s*([^|]+)%s*|%s*Hardware:%s*([^|]+)%s*|%s*IP:%s*(.+)')
		if did and name then
			table.insert(devices, {
				did = string.gsub(did, '^%s*(.-)%s*$', '%1'),
				name = string.gsub(name, '^%s*(.-)%s*$', '%1'),
				hardware = hw and string.gsub(hw, '^%s*(.-)%s*$', '%1') or '',
				ip = ip and string.gsub(ip, '^%s*(.-)%s*$', '%1') or ''
			})
		end
	end

	if #devices == 0 then
		http.write_json({ code = 1, msg = "获取设备失败。控制台输出: " .. result })
	else
		http.write_json({ code = 0, devices = devices })
	end
end
