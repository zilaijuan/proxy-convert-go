package parser

import "testing"

func TestParseSSWithEscapedBase64Credentials(t *testing.T) {
	link := "ss://MjAyMi1ibGFrZTMtY2hhY2hhMjAtcG9seTEzMDU6SG56SU4xQ0xHR0VoK21EL3V6YmJ2VFFjL0FscStoa21yQzdQbzNENGs0RT0%3D@46.38.143.236:60633#%3E%3E%40v2ray1_ng%3A%3AXX"

	proxy, err := ParseSS(link)
	if err != nil {
		t.Fatalf("ParseSS returned error: %v", err)
	}

	if proxy.Type != "ss" {
		t.Fatalf("unexpected type: %s", proxy.Type)
	}
	if proxy.Server != "46.38.143.236" {
		t.Fatalf("unexpected server: %s", proxy.Server)
	}
	if proxy.Port != 60633 {
		t.Fatalf("unexpected port: %d", proxy.Port)
	}
	if proxy.Name != ">>@v2ray1_ng::XX" {
		t.Fatalf("unexpected name: %q", proxy.Name)
	}

	cipher, _ := proxy.Extra["cipher"].(string)
	if cipher != "2022-blake3-chacha20-poly1305" {
		t.Fatalf("unexpected cipher: %q", cipher)
	}

	password, _ := proxy.Extra["password"].(string)
	if password != "HnzIN1CLGGEh+mD/uzbbvTQc/Alq+hkmrC7Po3D4k4E=" {
		t.Fatalf("unexpected password: %q", password)
	}
}

func TestParseSSNormalizesCipherAlias(t *testing.T) {
	link := "ss://Y2hhY2hhMjAtcG9seTEzMDU6VGVzdFBhc3N3b3JkQDEyMw==@series-a2.samanehha.co:443#alias"

	proxy, err := ParseSS(link)
	if err != nil {
		t.Fatalf("ParseSS returned error: %v", err)
	}

	cipher, _ := proxy.Extra["cipher"].(string)
	if cipher != "chacha20-ietf-poly1305" {
		t.Fatalf("unexpected cipher: %q", cipher)
	}
}

func TestParseVMessWithAppendedMetadata(t *testing.T) {
	link := "vmess://eyJhZGQiOiJjZWY5MWQ3YS10NzBmNDAtdG84eG4zLWw0bnYuaGt0LmdvdG9jaGluYXRvd24ubmV0IiwiYWlkIjoyLCJob3N0IjoiY2VmOTFkN2EtdDcwZjQwLXRvOHhuMy1sNG52LmhrdC5nb3RvY2hpbmF0b3duLm5ldCIsImlkIjoiMTQ2OWUwOWUtMDI3Yy0xMWVjLWEwZmMtZjIzYzkxM2M4ZDJiIiwibmV0Ijoid3MiLCJwYXRoIjoiLyIsInBvcnQiOjgwLCJwcyI6IvCfh6jwn4ezX0NOX+S4reWbvSIsInNlY3VyaXR5IjoiYXV0byIsInNuaSI6ImJyb2FkY2FzdGx2LmNoYXQuYmlsaWJpbGkuY29tIn0=@_@v2rayse.com@_@\U0001F1ED\U0001F1F0HK-219.79.209.100-4832"

	proxy, err := ParseVMess(link)
	if err != nil {
		t.Fatalf("ParseVMess returned error: %v", err)
	}

	if proxy.Type != "vmess" {
		t.Fatalf("unexpected type: %s", proxy.Type)
	}
	if proxy.Server != "cef91d7a-t70f40-to8xn3-l4nv.hkt.gotochinatown.net" {
		t.Fatalf("unexpected server: %s", proxy.Server)
	}
	if proxy.Port != 80 {
		t.Fatalf("unexpected port: %d", proxy.Port)
	}
	if proxy.Name != "\U0001F1E8\U0001F1F3_CN_\u4e2d\u56fd" {
		t.Fatalf("unexpected name: %q", proxy.Name)
	}
}

func TestParseTrojanWithRawPercentInPassword(t *testing.T) {
	link := "trojan://M7v%w11Se*@tttyder.wsone.icu:443?allowInsecure=1&sni=tttyder.wsone.icu#%F0%9F%87%AE%F0%9F%87%B3_IN_%E5%8D%B0%E5%BA%A6"

	proxy, err := ParseTrojan(link)
	if err != nil {
		t.Fatalf("ParseTrojan returned error: %v", err)
	}

	if proxy.Type != "trojan" {
		t.Fatalf("unexpected type: %s", proxy.Type)
	}
	if proxy.Server != "tttyder.wsone.icu" {
		t.Fatalf("unexpected server: %s", proxy.Server)
	}
	if proxy.Port != 443 {
		t.Fatalf("unexpected port: %d", proxy.Port)
	}
	if proxy.Name != "\U0001F1EE\U0001F1F3_IN_\u5370\u5ea6" {
		t.Fatalf("unexpected name: %q", proxy.Name)
	}

	password, _ := proxy.Extra["password"].(string)
	if password != "M7v%w11Se*" {
		t.Fatalf("unexpected password: %q", password)
	}

	sni, _ := proxy.Extra["sni"].(string)
	if sni != "tttyder.wsone.icu" {
		t.Fatalf("unexpected sni: %q", sni)
	}

	if skip, _ := proxy.Extra["skip-cert-verify"].(bool); !skip {
		t.Fatalf("expected skip-cert-verify=true")
	}
}

func TestParseVLESSWithInvalidUserinfo(t *testing.T) {
	link := "vless://Telegram\U0001F1E8\U0001F1F3 @WangCai2@193.37.70.63:443?host=tn1rr.qzz.io&path=%2Fsg-lnd&security=tls&sni=tn1rr.qzz.io&type=ws#%F0%9F%87%B7%F0%9F%87%BA_RU_%E4%BF%84%E7%BD%97%E6%96%AF-%3E%F0%9F%87%A9%F0%9F%87%AA_DE_%E5%BE%B7%E5%9B%BD"

	proxy, err := ParseVLESS(link)
	if err != nil {
		t.Fatalf("ParseVLESS returned error: %v", err)
	}

	if proxy.Type != "vless" {
		t.Fatalf("unexpected type: %s", proxy.Type)
	}
	if proxy.Server != "193.37.70.63" {
		t.Fatalf("unexpected server: %s", proxy.Server)
	}
	if proxy.Port != 443 {
		t.Fatalf("unexpected port: %d", proxy.Port)
	}
	if proxy.Name != "\U0001F1F7\U0001F1FA_RU_\u4fc4\u7f57\u65af->\U0001F1E9\U0001F1EA_DE_\u5fb7\u56fd" {
		t.Fatalf("unexpected name: %q", proxy.Name)
	}

	uuid, _ := proxy.Extra["uuid"].(string)
	if uuid != "Telegram\U0001F1E8\U0001F1F3 @WangCai2" {
		t.Fatalf("unexpected uuid: %q", uuid)
	}

	network, _ := proxy.Extra["network"].(string)
	if network != "ws" {
		t.Fatalf("unexpected network: %q", network)
	}

	if tls, _ := proxy.Extra["tls"].(bool); !tls {
		t.Fatalf("expected tls=true")
	}

	servername, _ := proxy.Extra["servername"].(string)
	if servername != "tn1rr.qzz.io" {
		t.Fatalf("unexpected servername: %q", servername)
	}

	wsOpts, ok := proxy.Extra["ws-opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing ws-opts")
	}
	if wsOpts["path"] != "/sg-lnd" {
		t.Fatalf("unexpected ws path: %v", wsOpts["path"])
	}
	headers, ok := wsOpts["headers"].(map[string]string)
	if !ok {
		t.Fatalf("missing ws headers")
	}
	if headers["Host"] != "tn1rr.qzz.io" {
		t.Fatalf("unexpected ws host: %q", headers["Host"])
	}
}

func TestParseAnyTLSWithPlainCredentials(t *testing.T) {
	link := "anytls://f0090017-9a4a-4eaf-b88a-9c1554ffd0d1@us-sje.baipiao0721-dns78655.eu:18443?allowInsecure=1&alpn=h2&fp=chrome&sni=gw.alicdn.com&udp=1#%F0%9F%87%BA%F0%9F%87%B8_US_%E7%BE%8E%E5%9B%BD@_@v2rayse.com@_@US120%EF%BD%9C%E6%9C%BA%E6%88%BF%EF%BD%9C%E4%BD%8E%E9%A3%8E%E9%99%A9"

	proxy, err := ParseAnyTLS(link)
	if err != nil {
		t.Fatalf("ParseAnyTLS returned error: %v", err)
	}

	if proxy.Type != "anytls" {
		t.Fatalf("unexpected type: %s", proxy.Type)
	}
	if proxy.Server != "us-sje.baipiao0721-dns78655.eu" {
		t.Fatalf("unexpected server: %s", proxy.Server)
	}
	if proxy.Port != 18443 {
		t.Fatalf("unexpected port: %d", proxy.Port)
	}
	if proxy.Name != "\U0001F1FA\U0001F1F8_US_\u7f8e\u56fd@_@v2rayse.com@_@US120\uff5c\u673a\u623f\uff5c\u4f4e\u98ce\u9669" {
		t.Fatalf("unexpected name: %q", proxy.Name)
	}

	password, _ := proxy.Extra["password"].(string)
	if password != "f0090017-9a4a-4eaf-b88a-9c1554ffd0d1" {
		t.Fatalf("unexpected password: %q", password)
	}

	sni, _ := proxy.Extra["sni"].(string)
	if sni != "gw.alicdn.com" {
		t.Fatalf("unexpected sni: %q", sni)
	}

	fingerprint, _ := proxy.Extra["fingerprint"].(string)
	if fingerprint != "chrome" {
		t.Fatalf("unexpected fingerprint: %q", fingerprint)
	}

	alpn, ok := proxy.Extra["alpn"].([]string)
	if !ok || len(alpn) != 1 || alpn[0] != "h2" {
		t.Fatalf("unexpected alpn: %#v", proxy.Extra["alpn"])
	}

	if skip, _ := proxy.Extra["skip-cert-verify"].(bool); !skip {
		t.Fatalf("expected skip-cert-verify=true")
	}
}
