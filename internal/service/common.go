package service

import (
	"fmt"
	"proxy-convert/internal/parser"
)

func buildProxyMap(proxy *parser.Proxy) map[string]interface{} {
	proxyMap := map[string]interface{}{
		"name":   proxy.Name,
		"type":   proxy.Type,
		"server": proxy.Server,
		"port":   proxy.Port,
	}

	switch proxy.Type {
	case "ss":
		if cipher, ok := proxy.Extra["cipher"].(string); ok {
			proxyMap["cipher"] = cipher
		}
		if password, ok := proxy.Extra["password"].(string); ok {
			proxyMap["password"] = password
		}
	case "vmess":
		if uuid, ok := proxy.Extra["uuid"].(string); ok {
			proxyMap["uuid"] = uuid
		}
		if alterId, ok := proxy.Extra["alterId"].(int); ok {
			proxyMap["alterId"] = alterId
		}
		if cipher, ok := proxy.Extra["cipher"].(string); ok {
			proxyMap["cipher"] = cipher
		}
		if network, ok := proxy.Extra["network"].(string); ok {
			proxyMap["network"] = network
		}
		if tls, ok := proxy.Extra["tls"].(bool); ok && tls {
			proxyMap["tls"] = true
			if servername, ok := proxy.Extra["servername"].(string); ok {
				proxyMap["servername"] = servername
			}
		}
		if wsOpts, ok := proxy.Extra["ws-opts"].(map[string]interface{}); ok {
			proxyMap["ws-opts"] = wsOpts
		}
	case "vless":
		if uuid, ok := proxy.Extra["uuid"].(string); ok {
			proxyMap["uuid"] = uuid
		}
		if network, ok := proxy.Extra["network"].(string); ok {
			proxyMap["network"] = network
		}
		if tls, ok := proxy.Extra["tls"].(bool); ok && tls {
			proxyMap["tls"] = true
			if servername, ok := proxy.Extra["servername"].(string); ok {
				proxyMap["servername"] = servername
			}
		}
		if flow, ok := proxy.Extra["flow"].(string); ok {
			proxyMap["flow"] = flow
		}
		if wsOpts, ok := proxy.Extra["ws-opts"].(map[string]interface{}); ok {
			proxyMap["ws-opts"] = wsOpts
		}
	case "trojan":
		if password, ok := proxy.Extra["password"].(string); ok {
			proxyMap["password"] = password
		}
		if sni, ok := proxy.Extra["sni"].(string); ok {
			proxyMap["sni"] = sni
		}
	case "hysteria2", "hy2":
		if authStr, ok := proxy.Extra["auth_str"].(string); ok {
			proxyMap["password"] = authStr
		}
		if sni, ok := proxy.Extra["sni"].(string); ok {
			proxyMap["sni"] = sni
		}
	default:
		return nil
	}

	return proxyMap
}

func escapeUnicode(s string) string {
	result := ""
	for i := 0; i < len(s); {
		// 处理 YAML 序列化后的 Unicode 转义序列（带双反斜杠）
		if i+11 <= len(s) && s[i:i+2] == "\\\\" && s[i+2:i+4] == "U" {
			hexStr := s[i+4 : i+12]
			if codePoint, err := parseHex(hexStr); err == nil {
				result += string(rune(codePoint))
				i += 12
				continue
			}
		}
		// 处理标准 Unicode 转义序列（带单反斜杠）
		if i+10 <= len(s) && s[i:i+2] == "\\U" {
			hexStr := s[i+2 : i+10]
			if codePoint, err := parseHex(hexStr); err == nil {
				result += string(rune(codePoint))
				i += 10
				continue
			}
		}
		if i+6 <= len(s) && s[i:i+2] == "\\u" {
			hexStr := s[i+2 : i+6]
			if codePoint, err := parseHex(hexStr); err == nil {
				result += string(rune(codePoint))
				i += 6
				continue
			}
		}
		result += string(s[i])
		i++
	}
	return result
}

func parseHex(s string) (int64, error) {
	var result int64
	for _, c := range s {
		var v byte
		switch {
		case '0' <= c && c <= '9':
			v = byte(c - '0')
		case 'a' <= c && c <= 'f':
			v = byte(c - 'a' + 10)
		case 'A' <= c && c <= 'F':
			v = byte(c - 'A' + 10)
		default:
			return 0, fmt.Errorf("invalid hex")
		}
		result = result<<4 | int64(v)
	}
	return result, nil
}