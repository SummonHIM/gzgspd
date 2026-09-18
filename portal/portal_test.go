package portal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T) *Client {
	t.Helper()
	c, err := NewClient("127.0.0.1", 3*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestQuickAuthBuildsQuery(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","message":"ok","userId":"u1","groupId":19}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := newTestClient(t)
	resp, err := c.QuickAuth(context.Background(), QuickAuthRequest{
		Scheme: u.Scheme, Host: u.Host, UserAgent: "UA",
		UserID: "user", Password: "pass",
		Wlanuserip: "10.0.0.1", Wlanacname: "ac", WlanacIP: "10.0.0.2",
		VLAN: "1", MAC: "aa:bb:cc:dd:ee:ff",
		Version: 4, PortalPageID: 1, Timestamp: 123, UUID: "uuid",
		PortalType: "0", Hostname: "h", Rand: "r",
	})
	if err != nil {
		t.Fatalf("quickauth: %v", err)
	}
	if resp.Code != "0" || resp.UserID != "u1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got.Get("userid") != "user" || got.Get("passwd") != "pass" || got.Get("mac") != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("query mismatch: %v", got)
	}
}

func TestQuickAuthDisconnForm(t *testing.T) {
	var ctype, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctype = r.Header.Get("Content-Type")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		body = string(buf)
		_, _ = w.Write([]byte(`{"code":"0"}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := newTestClient(t)
	if _, err := c.QuickAuthDisconn(context.Background(), DisconnRequest{
		Scheme: u.Scheme, Host: u.Host, UserAgent: "UA",
		WlanacIP: "10.0.0.2", Wlanuserip: "10.0.0.1", Wlanacname: "ac",
		Version: 4, PortalType: "0", UserID: "u@SSGSXY",
		MAC: "aa:bb:cc:dd:ee:ff", GroupID: 19, ClearOperator: "0",
	}); err != nil {
		t.Fatalf("disconn: %v", err)
	}
	if !strings.HasPrefix(ctype, "application/x-www-form-urlencoded") {
		t.Fatalf("unexpected content type: %q", ctype)
	}
	if !strings.Contains(body, "wlanacip=10.0.0.2") || !strings.Contains(body, "groupId=19") {
		t.Fatalf("form mismatch: %q", body)
	}
}

func TestPortalCheckerRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://x/portal.do?wlanuserip=1", http.StatusFound)
	}))
	defer srv.Close()

	c := newTestClient(t)
	need, link, err := c.PortalChecker(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("checker: %v", err)
	}
	if !need || !strings.Contains(link, "portal.do") {
		t.Fatalf("expected login required, got need=%v link=%q", need, link)
	}
}

func TestPortalCheckerScriptReplace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><script>location.replace("http://x/portalScript.do?a=1");</script></html>`))
	}))
	defer srv.Close()

	c := newTestClient(t)
	need, link, err := c.PortalChecker(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("checker: %v", err)
	}
	if !need || !strings.Contains(link, "portalScript.do") {
		t.Fatalf("expected login required, got need=%v link=%q", need, link)
	}
}

func TestPortalCheckerOnline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>ok</html>`))
	}))
	defer srv.Close()

	c := newTestClient(t)
	need, link, err := c.PortalChecker(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("checker: %v", err)
	}
	if need || link != "" {
		t.Fatalf("expected online, got need=%v link=%q", need, link)
	}
}

func TestPortalCheckerNetworkError(t *testing.T) {
	c := newTestClient(t)
	if _, _, err := c.PortalChecker(context.Background(), "http://127.0.0.1:1/none"); err == nil {
		t.Fatal("expected error on connection failure, got nil")
	}
}

func TestQuickAuthHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := newTestClient(t)
	if _, err := c.QuickAuth(context.Background(), QuickAuthRequest{Scheme: u.Scheme, Host: u.Host, UserAgent: "UA"}); err == nil {
		t.Fatal("expected error on non-2xx response")
	}
}

func TestQuickAuthContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.QuickAuth(ctx, QuickAuthRequest{Scheme: u.Scheme, Host: u.Host, UserAgent: "UA"}); err == nil {
		t.Fatal("expected context deadline error")
	}
}
