module("luci.controller.miair", package.seeall)

function index()
	if not nixio.fs.access("/etc/config/miair") then
		return
	end

	entry({"admin", "services", "miair"}, cbi("miair/miair"), _("MiAir 小爱投播"), 70).dependent = true
	entry({"admin", "services", "miair", "get_devices"}, call("action_get_devices")).leaf = true
	entry({"admin", "services", "miair", "status"}, call("action_status")).leaf = true
	entry({"admin", "services", "miair", "get_qr"}, call("action_get_qr")).leaf = true
	entry({"admin", "services", "miair", "poll_qr"}, call("action_poll_qr")).leaf = true
end

function action_status()
	local running = (luci.sys.call("pidof miair-core >/dev/null") == 0)
	local has_token = nixio.fs.access("/etc/miair/token.json")
	local version = nixio.fs.readfile("/usr/share/miair/version") or "development"
	local runtime = {}
	local runtime_json = nixio.fs.readfile("/var/run/miair-status.json")
	if runtime_json then
		local ok, jsonc = pcall(require, "luci.jsonc")
		if ok and jsonc then
			runtime = jsonc.parse(runtime_json) or {}
		end
	end
	version = string.gsub(version, "^%s*(.-)%s*$", "%1")
	luci.http.prepare_content("application/json")
	luci.http.write_json({
		running = running,
		has_token = has_token,
		version = version,
		source = runtime.source or {}
	})
end

function action_get_qr()
	local http = require "luci.http"
	local util = require "luci.util"
	local uci = require("luci.model.uci").cursor()
	local store = uci:get("miair", "config", "mi_token_store") or "/etc/miair/token.json"

	http.prepare_content("application/json")
	local cmd = string.format('/usr/bin/miair-core -qr -store %s 2>&1', util.shellquote(store))
	local handle = io.popen(cmd)
	local result = handle:read("*a")
	handle:close()

	local qr_url, lp_url, timeout = string.match(result, 'QR_URL:([^|]+)|LP_URL:([^|]+)|TIMEOUT:(%d+)')
	if qr_url and lp_url then
		http.write_json({
			code = 0,
			qr_url = qr_url,
			lp_url = lp_url,
			timeout = tonumber(timeout) or 300
		})
	else
		http.write_json({
			code = 1,
			msg = "获取二维码失败: " .. (result or "")
		})
	end
end

function action_poll_qr()
	local http = require "luci.http"
	local util = require "luci.util"
	local uci = require("luci.model.uci").cursor()
	local lp_url = http.formvalue("lp_url") or ""
	local store = uci:get("miair", "config", "mi_token_store") or "/etc/miair/token.json"

	http.prepare_content("application/json")
	if lp_url == "" then
		http.write_json({ code = 1, msg = "缺少长轮询地址" })
		return
	end

	local cmd = string.format('/usr/bin/miair-core -poll-qr %s -store %s 2>&1', util.shellquote(lp_url), util.shellquote(store))
	local handle = io.popen(cmd)
	local result = handle:read("*a")
	handle:close()

	result = string.gsub(result, "^%s*(.-)%s*$", "%1")
	if string.match(result, "^SUCCESS") then
		local user_id = string.match(result, "SUCCESS:([^|]+)")
		http.write_json({ code = 0, status = "ok", user_id = user_id })
	elseif string.match(result, "^WAIT") then
		http.write_json({ code = 0, status = "wait" })
	elseif string.match(result, "^EXPIRED") then
		http.write_json({ code = 0, status = "expired" })
	else
		http.write_json({ code = 1, msg = result })
	end
end

function action_get_devices()
	local http = require "luci.http"
	local util = require "luci.util"
	local uci = require("luci.model.uci").cursor()
	local store = uci:get("miair", "config", "mi_token_store") or "/etc/miair/token.json"

	http.prepare_content("application/json")

	local cmd = string.format('/usr/bin/miair-core -list -store %s 2>&1', util.shellquote(store))
	local handle = io.popen(cmd)
	local result = handle:read("*a")
	handle:close()

	local devices = {}
	for line in string.gmatch(result, '[^%r%n]+') do
		local did, name, hw = string.match(line, 'DID:%s*([^|]+)%s*|%s*Name:%s*([^|]+)%s*|%s*Hardware:%s*([^|]+)')
		if did and name then
			local ip = string.match(line, '|%s*IP:%s*(.*)$') or ""
			table.insert(devices, {
				did = string.gsub(did, "^%s*(.-)%s*$", "%1"),
				name = string.gsub(name, "^%s*(.-)%s*$", "%1"),
				hardware = string.gsub(hw, "^%s*(.-)%s*$", "%1"),
				ip = string.gsub(ip, "^%s*(.-)%s*$", "%1")
			})
		end
	end

	if #devices == 0 then
		http.write_json({ code = 1, msg = "未发现小爱设备。输出: " .. result })
	else
		http.write_json({ code = 0, devices = devices })
	end
end
