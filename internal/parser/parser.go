package parser

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type Proxy struct {
	Name     string                 `json:"name"`
	Type     string                 `json:"type"`
	Server   string                 `json:"server"`
	Port     int                    `json:"port"`
	UDP      bool                   `json:"udp"`
	Extra    map[string]interface{} `json:"-"`
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
		name = parts[1]
	}

	var method, password, host, port string

	if strings.Contains(body, "@") {
		creds, server := splitAt(body, "@")
		decoded, err := base64.StdEncoding.DecodeString(creds)
		if err != nil {
			return nil, fmt.Errorf("failed to decode credentials: %w", err)
		}
		credsParts := strings.SplitN(string(decoded), ":", 2)
		if len(credsParts) != 2 {
			return nil, fmt.Errorf("invalid credentials format")
		}
		method = credsParts[0]
		password = credsParts[1]
		serverParts := strings.SplitN(server, ":", 2)
		if len(serverParts) != 2 {
			return nil, fmt.Errorf("invalid server format")
		}
		host = serverParts[0]
		port = strings.TrimSuffix(serverParts[1], "?")
	} else {
		decoded, err := base64.StdEncoding.DecodeString(body)
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
		serverParts := strings.SplitN(credsParts[1], ":", 2)
		if len(serverParts) != 2 {
			return nil, fmt.Errorf("invalid server format")
		}
		host = serverParts[0]
		port = serverParts[1]
	}

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

func ParseVMess(link string) (*Proxy, error) {
	if !strings.HasPrefix(link, "vmess://") {
		return nil, fmt.Errorf("invalid vmess link")
	}

	base64Str := link[8:]
	missingPadding := len(base64Str) % 4
	if missingPadding != 0 {
		base64Str += strings.Repeat("=", 4-missingPadding)
	}

	decoded, err := base64.StdEncoding.DecodeString(base64Str)
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
			"uuid":     getString(data, "id", ""),
			"alterId":  getInt(data, "aid", 0),
			"cipher":   getString(data, "scy", "auto"),
			"network":  getString(data, "net", "tcp"),
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

	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("failed to parse vless url: %w", err)
	}

	port := 443
	if u.Port() != "" {
		if p, err := parsePort(u.Port()); err == nil {
			port = p
		}
	}

	query := u.Query()
	proxy := &Proxy{
		Name:   u.Fragment,
		Type:   "vless",
		Server: u.Hostname(),
		Port:   port,
		UDP:    true,
		Extra: map[string]interface{}{
			"uuid":    u.User.Username(),
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
			"headers": map[string]string{"Host": getStringFromQuery(query, "host", u.Hostname())},
		}
	}

	return proxy, nil
}

func ParseTrojan(link string) (*Proxy, error) {
	if !strings.HasPrefix(link, "trojan://") {
		return nil, fmt.Errorf("invalid trojan link")
	}

	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("failed to parse trojan url: %w", err)
	}

	port := 443
	if u.Port() != "" {
		if p, err := parsePort(u.Port()); err == nil {
			port = p
		}
	}

	query := u.Query()
	proxy := &Proxy{
		Name:   u.Fragment,
		Type:   "trojan",
		Server: u.Hostname(),
		Port:   port,
		UDP:    true,
		Extra: map[string]interface{}{
			"password": u.User.Username(),
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

	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("failed to parse anytls url: %w", err)
	}

	port := 443
	if u.Port() != "" {
		if p, err := parsePort(u.Port()); err == nil {
			port = p
		}
	}

	query := u.Query()
	proxy := &Proxy{
		Name:   u.Fragment,
		Type:   "anytls",
		Server: u.Hostname(),
		Port:   port,
		UDP:    true,
		Extra: map[string]interface{}{
			"password": u.User.Username(),
			"sni":      query.Get("sni"),
			"alpn":     strings.Split(query.Get("alpn"), ","),
		},
	}

	proxy.Extra["skip-cert-verify"] = true

	return proxy, nil
}

func ParseLink(link string) (*Proxy, error) {
	link = strings.TrimSpace(link)
	if link == "" {
		return nil, fmt.Errorf("empty link")
	}

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

	auth := ""
	switch p.Type {
	case "ss", "trojan", "anytls":
		if pw, ok := p.Extra["password"].(string); ok {
			auth = pw
		}
	case "vmess", "vless":
		if uuid, ok := p.Extra["uuid"].(string); ok {
			auth = uuid
		}
	case "hysteria", "hysteria2":
		if authStr, ok := p.Extra["auth_str"].(string); ok {
			auth = authStr
		}
	}

	return fmt.Sprintf("%s,%s,%d,%s", p.Type, p.Server, p.Port, auth)
}

func splitAt(s, sep string) (string, string) {
	idx := strings.Index(s, sep)
	if idx == -1 {
		return s, ""
	}
	return s[:idx], s[idx+len(sep):]
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
	if v := query.Get(key); v != "" {
		return v
	}
	return defaultValue
}

func unescape(s string) string {
	decoded, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return decoded
}
