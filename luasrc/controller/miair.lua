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
	entry({"admin", "services", "miair", "restart"}, call("action_restart")).leaf = true
end

local function read_file(path)
	local f = io.open(path, "r")
	if not f then return nil end
	local content = f:read("*a")
	f:close()
	return content
end

local function resolve_device_name(device_str)
	if not device_str or device_str == "" then return "" end
	local ip = string.match(device_str, "^%[?([^%]]+)%]?")
	if not ip then ip = string.match(device_str, "^([^:]+)") end
	if not ip then return device_str end

	local leases_mac_name = {}
	local leases_ip_name = {}
	local leases_mac_ip = {}
	local f = io.open("/tmp/dhcp.leases", "r")
	if f then
		for line in f:lines() do
			local _, mac, lease_ip, name = string.match(line, "(%d+)%s+([%a%d:]+)%s+([%d%.]+)%s+([^%s]+)")
			if mac and name and name ~= "*" then
				local lmac = string.lower(mac)
				leases_mac_name[lmac] = name
				leases_ip_name[lease_ip] = name
				leases_mac_ip[lmac] = lease_ip
			end
		end
		f:close()
	end

	if leases_ip_name[ip] then
		return string.format("%s (%s)", leases_ip_name[ip], ip)
	end

	local handle = io.popen(string.format("ip neigh show %s 2>/dev/null", ip))
	if handle then
		local out = handle:read("*a")
		handle:close()
		local mac = string.match(out or "", "lladdr%s+([%a%d:]+)")
		if mac then
			local lmac = string.lower(mac)
			if leases_mac_name[lmac] then
				local show_ip = leases_mac_ip[lmac] or ip
				return string.format("%s (%s)", leases_mac_name[lmac], show_ip)
			end
		end
	end

	return device_str
end

function action_status()
	local sys = require "luci.sys"
	local http = require "luci.http"
	local running = (sys.call("pidof miair-core >/dev/null") == 0)
	local has_token = nixio.fs.access("/etc/miair/token.json")
	local version = read_file("/usr/share/miair/version") or "1.1.1"
	local runtime = {}
	local runtime_json = read_file("/var/run/miair-status.json")
	if runtime_json then
		local ok, jsonc = pcall(require, "luci.jsonc")
		if ok and jsonc then
			runtime = jsonc.parse(runtime_json) or {}
		end
	end
	if runtime.source and runtime.source.active and runtime.source.active.device then
		runtime.source.active.device = resolve_device_name(runtime.source.active.device)
	end
	version = string.gsub(version, "^%s*(.-)%s*$", "%1")
	http.prepare_content("application/json")
	http.write_json({
		running = running,
		has_token = has_token,
		version = version,
		source = runtime.source or {},
		token = runtime.token or {}
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

function action_restart()
	local sys = require "luci.sys"
	local http = require "luci.http"
	sys.call("/etc/init.d/miair restart >/dev/null 2>&1")
	http.prepare_content("application/json")
	http.write_json({ code = 0, msg = "服务已重启" })
end
