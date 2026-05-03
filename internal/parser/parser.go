package parser

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
)

type Proxy struct {
	Name   string                 `json:"name"`
	Type   string                 `json:"type"`
	Server string                 `json:"server"`
	Port   int                    `json:"port"`
	UDP    bool                   `json:"udp"`
	Extra  map[string]interface{} `json:"-"`
}

func ParseSS(link string) (*Proxy, error) {
	if !strings.HasPrefix(link, "ss://") {
		return nil, fmt.Errorf("invalid ss link")
	}

	body := link[5:]
	parts := strings.SplitN(body, "#", 2)
	body = parts[0]
	name := ""
	if len(parts) > 1 {
		name = unescape(parts[1])
	}

	var method, password, host, port string

	if strings.Contains(body, "@") {
		creds, server := splitAt(body, "@")
		decoded, err := decodeBase64String(creds)
		if err != nil {
			return nil, fmt.Errorf("failed to decode credentials: %w", err)
		}
		credsParts := strings.SplitN(string(decoded), ":", 2)
		if len(credsParts) != 2 {
			return nil, fmt.Errorf("invalid credentials format")
		}
		method = credsParts[0]
		password = credsParts[1]
		host, port, err = parseServerPort(server)
		if err != nil {
			return nil, err
		}
	} else {
		decoded, err := decodeBase64String(body)
		if err != nil {
			return nil, fmt.Errorf("failed to decode body: %w", err)
		}
		rest := strings.SplitN(string(decoded), ":", 2)
		if len(rest) != 2 {
			return nil, fmt.Errorf("invalid body format")
		}
		method = rest[0]
		credsParts := strings.SplitN(rest[1], "@", 2)
		if len(credsParts) != 2 {
			return nil, fmt.Errorf("invalid credentials format")
		}
		password = credsParts[0]
		host, port, err = parseServerPort(credsParts[1])
		if err != nil {
			return nil, err
		}
	}

	method = normalizeSSCipher(method)

	portInt := 8388
	if p, err := parsePort(port); err == nil {
		portInt = p
	}

	return &Proxy{
		Name:   name,
		Type:   "ss",
		Server: host,
		Port:   portInt,
		UDP:    true,
		Extra: map[string]interface{}{
			"cipher":   method,
			"password": password,
		},
	}, nil
}

func normalizeSSCipher(cipher string) string {
	switch strings.ToLower(strings.TrimSpace(cipher)) {
	case "chacha20-poly1305":
		return "chacha20-ietf-poly1305"
	case "xchacha20-poly1305":
		return "xchacha20-ietf-poly1305"
	default:
		return cipher
	}
}

func ParseVMess(link string) (*Proxy, error) {
	if !strings.HasPrefix(link, "vmess://") {
		return nil, fmt.Errorf("invalid vmess link")
	}

	base64Str := trimEncodedPayload(link[8:])
	decoded, err := decodeBase64String(base64Str)
	if err != nil {
		return nil, fmt.Errorf("failed to decode vmess link: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(decoded, &data); err != nil {
		return nil, fmt.Errorf("failed to parse vmess json: %w", err)
	}

	port := 443
	if p, ok := data["port"].(float64); ok {
		port = int(p)
	}

	proxy := &Proxy{
		Name:   getString(data, "ps", ""),
		Type:   "vmess",
		Server: getString(data, "add", ""),
		Port:   port,
		UDP:    true,
		Extra: map[string]interface{}{
			"uuid":    getString(data, "id", ""),
			"alterId": getInt(data, "aid", 0),
			"cipher":  getString(data, "scy", "auto"),
			"network": getString(data, "net", "tcp"),
		},
	}

	if tls, ok := data["tls"].(string); ok && tls == "tls" {
		proxy.Extra["tls"] = true
		proxy.Extra["servername"] = getString(data, "sni", "")
	}

	if network, ok := proxy.Extra["network"].(string); ok && network == "ws" {
		proxy.Extra["ws-opts"] = map[string]interface{}{
			"path":    getString(data, "path", "/"),
			"headers": map[string]string{"Host": getString(data, "host", "")},
		}
	}

	return proxy, nil
}

func ParseVLESS(link string) (*Proxy, error) {
	if !strings.HasPrefix(link, "vless://") {
		return nil, fmt.Errorf("invalid vless link")
	}

	body := link[len("vless://"):]
	parts := strings.SplitN(body, "#", 2)
	body = parts[0]
	name := ""
	if len(parts) > 1 {
		name = unescape(parts[1])
	}

	serverPart := body
	query := url.Values{}
	if idx := strings.Index(body, "?"); idx >= 0 {
		serverPart = body[:idx]
		if parsedQuery, err := url.ParseQuery(body[idx+1:]); err == nil {
			query = parsedQuery
		}
	}

	user, server := splitAtLast(serverPart, "@")
	if server == "" {
		return nil, fmt.Errorf("invalid vless server format")
	}

	host, portStr, err := parseServerPort(server)
	if err != nil {
		return nil, err
	}

	port := 443
	if p, err := parsePort(portStr); err == nil {
		port = p
	}

	proxy := &Proxy{
		Name:   name,
		Type:   "vless",
		Server: host,
		Port:   port,
		UDP:    true,
		Extra: map[string]interface{}{
			"uuid":    unescape(user),
			"network": getStringFromQuery(query, "type", "tcp"),
		},
	}

	if security := query.Get("security"); security == "tls" {
		proxy.Extra["tls"] = true
		proxy.Extra["servername"] = query.Get("sni")
	} else if security == "reality" {
		proxy.Extra["flow"] = query.Get("flow")
		proxy.Extra["reality-opts"] = map[string]string{
			"public-key": query.Get("pbk"),
			"short-id":   query.Get("sid"),
		}
		proxy.Extra["servername"] = query.Get("sni")
		proxy.Extra["fingerprint"] = getStringFromQuery(query, "fp", "chrome")
	}

	if network, ok := proxy.Extra["network"].(string); ok && network == "ws" {
		proxy.Extra["ws-opts"] = map[string]interface{}{
			"path":    unescape(getStringFromQuery(query, "path", "/")),
			"headers": map[string]string{"Host": getStringFromQuery(query, "host", host)},
		}
	}

	return proxy, nil
}

func ParseTrojan(link string) (*Proxy, error) {
	if !strings.HasPrefix(link, "trojan://") {
		return nil, fmt.Errorf("invalid trojan link")
	}

	body := link[len("trojan://"):]
	parts := strings.SplitN(body, "#", 2)
	body = parts[0]
	name := ""
	if len(parts) > 1 {
		name = unescape(parts[1])
	}

	serverPart := body
	query := url.Values{}
	if idx := strings.Index(body, "?"); idx >= 0 {
		serverPart = body[:idx]
		if parsedQuery, err := url.ParseQuery(body[idx+1:]); err == nil {
			query = parsedQuery
		}
	}

	password, server := splitAtLast(serverPart, "@")
	if server == "" {
		return nil, fmt.Errorf("invalid trojan server format")
	}

	host, portStr, err := parseServerPort(server)
	if err != nil {
		return nil, err
	}

	port := 443
	if p, err := parsePort(portStr); err == nil {
		port = p
	}

	proxy := &Proxy{
		Name:   name,
		Type:   "trojan",
		Server: host,
		Port:   port,
		UDP:    true,
		Extra: map[string]interface{}{
			"password": unescape(password),
			"sni":      query.Get("sni"),
		},
	}

	if typ := query.Get("type"); typ == "ws" {
		proxy.Extra["network"] = "ws"
		proxy.Extra["ws-opts"] = map[string]interface{}{
			"path":    unescape(getStringFromQuery(query, "path", "/")),
			"headers": map[string]string{"Host": getStringFromQuery(query, "host", query.Get("sni"))},
		}
	}

	if query.Get("allowInsecure") == "1" {
		proxy.Extra["skip-cert-verify"] = true
	}

	return proxy, nil
}

func ParseHysteria2(link string) (*Proxy, error) {
	if !strings.HasPrefix(link, "hysteria2://") && !strings.HasPrefix(link, "hy2://") {
		return nil, fmt.Errorf("invalid hysteria2 link")
	}

	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("failed to parse hysteria2 url: %w", err)
	}

	port := 443
	if u.Port() != "" {
		if p, err := parsePort(u.Port()); err == nil {
			port = p
		}
	}

	query := u.Query()
	auth := query.Get("auth")
	if auth == "" {
		auth = query.Get("auth_str")
	}
	if auth == "" && u.User != nil {
		auth = u.User.Username()
	}

	proxy := &Proxy{
		Name:   u.Fragment,
		Type:   "hysteria2",
		Server: u.Hostname(),
		Port:   port,
		UDP:    true,
		Extra: map[string]interface{}{
			"auth_str": auth,
			"alpn":     strings.Split(query.Get("alpn"), ","),
			"up":       query.Get("upmbps"),
			"down":     query.Get("downmbps"),
		},
	}

	if query.Get("insecure") == "1" {
		proxy.Extra["skip-cert-verify"] = true
	}

	if obfs := query.Get("obfs"); obfs != "" {
		proxy.Extra["obfs"] = obfs
		proxy.Extra["obfs-password"] = query.Get("obfs-password")
	}

	return proxy, nil
}

func ParseAnyTLS(link string) (*Proxy, error) {
	if !strings.HasPrefix(link, "anytls://") {
		return nil, fmt.Errorf("invalid anytls link")
	}

	body := link[9:]
	parts := strings.SplitN(body, "#", 2)
	body = parts[0]
	name := ""
	if len(parts) > 1 {
		name = unescape(parts[1])
	}

	serverPart := body
	query := url.Values{}
	if idx := strings.Index(body, "?"); idx >= 0 {
		serverPart = body[:idx]
		if parsedQuery, err := url.ParseQuery(body[idx+1:]); err == nil {
			query = parsedQuery
		}
	}

	creds, server := splitAtLast(serverPart, "@")
	if server == "" {
		return nil, fmt.Errorf("invalid anytls server format")
	}

	password := unescape(creds)
	extra := map[string]interface{}{
		"password": password,
	}

	if decoded, err := decodeBase64String(creds); err == nil {
		credsParts := strings.SplitN(string(decoded), ":", 2)
		if len(credsParts) == 2 {
			extra["cipher"] = credsParts[0]
			extra["password"] = credsParts[1]
		}
	}

	if sni := query.Get("sni"); sni != "" {
		extra["sni"] = sni
	}
	if fp := query.Get("fp"); fp != "" {
		extra["fingerprint"] = fp
	}
	if alpn := query.Get("alpn"); alpn != "" {
		extra["alpn"] = strings.Split(alpn, ",")
	}
	if query.Get("allowInsecure") == "1" || query.Get("insecure") == "1" {
		extra["skip-cert-verify"] = true
	}

	host, portStr, err := parseServerPort(server)
	if err != nil {
		return nil, err
	}

	portInt := 443
	if p, err := parsePort(portStr); err == nil {
		portInt = p
	}

	return &Proxy{
		Name:   name,
		Type:   "anytls",
		Server: host,
		Port:   portInt,
		UDP:    true,
		Extra:  extra,
	}, nil
}

func ParseLink(link string) (*Proxy, error) {
	if strings.HasPrefix(link, "ss://") {
		return ParseSS(link)
	} else if strings.HasPrefix(link, "vmess://") {
		return ParseVMess(link)
	} else if strings.HasPrefix(link, "vless://") {
		return ParseVLESS(link)
	} else if strings.HasPrefix(link, "trojan://") {
		return ParseTrojan(link)
	} else if strings.HasPrefix(link, "hysteria2://") || strings.HasPrefix(link, "hy2://") {
		return ParseHysteria2(link)
	} else if strings.HasPrefix(link, "hysteria://") {
		return ParseHysteria2(link)
	} else if strings.HasPrefix(link, "anytls://") {
		return ParseAnyTLS(link)
	}

	return nil, fmt.Errorf("unsupported link type")
}

func GetNodeFingerprint(p *Proxy) string {
	if p == nil {
		return ""
	}

	// 标准化服务器地址（转小写）
	server := strings.ToLower(p.Server)

	auth := ""
	cipher := ""
	network := ""
	tls := ""
	servername := ""

	switch p.Type {
	case "ss", "anytls":
		if pw, ok := p.Extra["password"].(string); ok {
			auth = pw
		}
		if c, ok := p.Extra["cipher"].(string); ok {
			cipher = c
		}
	case "trojan":
		if pw, ok := p.Extra["password"].(string); ok {
			auth = pw
		}
		if sni, ok := p.Extra["sni"].(string); ok {
			servername = sni
		}
		if net, ok := p.Extra["network"].(string); ok {
			network = net
		}
	case "vmess":
		if uuid, ok := p.Extra["uuid"].(string); ok {
			auth = uuid
		}
		if c, ok := p.Extra["cipher"].(string); ok {
			cipher = c
		}
		if net, ok := p.Extra["network"].(string); ok {
			network = net
		}
		if t, ok := p.Extra["tls"].(bool); ok && t {
			tls = "tls"
		}
		if sni, ok := p.Extra["servername"].(string); ok {
			servername = sni
		}
	case "vless":
		if uuid, ok := p.Extra["uuid"].(string); ok {
			auth = uuid
		}
		if net, ok := p.Extra["network"].(string); ok {
			network = net
		}
		if t, ok := p.Extra["tls"].(bool); ok && t {
			tls = "tls"
		}
		if sni, ok := p.Extra["servername"].(string); ok {
			servername = sni
		}
	case "hysteria", "hysteria2":
		if authStr, ok := p.Extra["auth_str"].(string); ok {
			auth = authStr
		}
		if alpn, ok := p.Extra["alpn"].([]string); ok && len(alpn) > 0 {
			cipher = strings.Join(alpn, ",")
		}
	}

	// 构建指纹，包含关键参数
	return fmt.Sprintf("%s,%s,%d,%s,%s,%s,%s,%s",
		p.Type, server, p.Port, auth, cipher, network, tls, servername)
}

func splitAt(s, sep string) (string, string) {
	idx := strings.Index(s, sep)
	if idx == -1 {
		return s, ""
	}
	return s[:idx], s[idx+len(sep):]
}

func splitAtLast(s, sep string) (string, string) {
	idx := strings.LastIndex(s, sep)
	if idx == -1 {
		return s, ""
	}
	return s[:idx], s[idx+len(sep):]
}

func trimEncodedPayload(s string) string {
	s = strings.TrimSpace(s)
	for _, sep := range []string{"#", "@_@", "?"} {
		if idx := strings.Index(s, sep); idx >= 0 {
			s = s[:idx]
		}
	}
	return strings.TrimSpace(s)
}

func decodeBase64String(s string) ([]byte, error) {
	s = trimEncodedPayload(s)
	decoded, err := url.PathUnescape(s)
	if err == nil {
		s = decoded
	}

	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")

	if missingPadding := len(s) % 4; missingPadding != 0 {
		s += strings.Repeat("=", 4-missingPadding)
	}

	return base64.StdEncoding.DecodeString(s)
}

func parseServerPort(server string) (string, string, error) {
	server = strings.TrimSpace(server)
	server = strings.TrimSuffix(server, "/")
	if idx := strings.Index(server, "?"); idx >= 0 {
		server = server[:idx]
	}

	if host, port, err := net.SplitHostPort(server); err == nil {
		return host, port, nil
	}

	serverParts := strings.SplitN(server, ":", 2)
	if len(serverParts) != 2 {
		return "", "", fmt.Errorf("invalid server format")
	}

	return serverParts[0], serverParts[1], nil
}

func parsePort(s string) (int, error) {
	var port int
	_, err := fmt.Sscanf(s, "%d", &port)
	return port, err
}

func getString(m map[string]interface{}, key, defaultValue string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return defaultValue
}

func getInt(m map[string]interface{}, key string, defaultValue int) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return defaultValue
}

func getStringFromQuery(query url.Values, key, defaultValue string) string {
	if val := query.Get(key); val != "" {
		return val
	}
	return defaultValue
}

func unescape(s string) string {
	if s == "" {
		return ""
	}
	decoded, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return decoded
}
