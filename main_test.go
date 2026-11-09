package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// serve replays recorded response bodies in order (all HTTP 200, like the real API) and records each request.
func serve(t *testing.T, replies []string) *[]map[string]any {
	t.Helper()
	var seen []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		b := map[string]any{}
		_ = json.Unmarshal(raw, &b)
		b["_path"] = r.URL.Path
		seen = append(seen, b)
		rp := replies[0]
		replies = replies[1:]
		_, _ = w.Write([]byte(rp))
	}))
	t.Cleanup(srv.Close)
	baseURL = srv.URL
	sleep = func(time.Duration) {}
	return &seen
}

func TestSuccess(t *testing.T) {
	seen := serve(t, []string{
		`{"code":0,"message":"ok","data":{"id":"t1","status":"pending","taskId":"up-9"}}`,
		`{"code":0,"message":"ok","data":{"id":"t1","status":"processing","images":[]}}`,
		`{"code":0,"message":"ok","data":{"id":"t1","status":"success","images":["https://cdn.aigifmaker.ai/t1.gif"],"watermarked":[false]}}`,
	})
	var out, errb bytes.Buffer
	if code := run([]string{"a", "cat", "dancing"}, "sk-x", &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if strings.TrimSpace(out.String()) != "https://cdn.aigifmaker.ai/t1.gif" {
		t.Fatal(out.String())
	}
	s := *seen
	got := s[0]
	delete(got, "_path")
	b, _ := json.Marshal(requestBody("a cat dancing"))
	var want map[string]any
	_ = json.Unmarshal(b, &want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("submit body %v", got)
	}
	if s[1]["_path"] != "/api/ai/query" || s[1]["taskId"] != "t1" {
		t.Fatalf("query body %v", s[1])
	}
}

func TestQueued(t *testing.T) {
	serve(t, []string{
		`{"code":0,"message":"ok","data":{"id":"t1","status":"pending"}}`,
		`{"code":0,"message":"ok","data":{"id":"t1","status":"processing","images":[],"queued":true,"queueEta":40}}`,
		`{"code":0,"message":"ok","data":{"id":"t1","status":"processing","images":[],"queued":true,"queueEta":3}}`,
		`{"code":0,"message":"ok","data":{"id":"t1","status":"success","images":["https://cdn.aigifmaker.ai/q.gif"],"watermarked":[true]}}`,
	})
	var out, errb bytes.Buffer
	if run([]string{"x"}, "sk-x", &out, &errb) != 0 || strings.TrimSpace(out.String()) != "https://cdn.aigifmaker.ai/q.gif" {
		t.Fatal(out.String(), errb.String())
	}
}

func TestFailed(t *testing.T) {
	serve(t, []string{
		`{"code":0,"message":"ok","data":{"id":"f1","status":"pending"}}`,
		`{"code":0,"message":"ok","data":{"id":"f1","status":"failed","images":[],"errorMessage":"upstream error","refundNote":"credit refunded"}}`,
	})
	var out, errb bytes.Buffer
	if run([]string{"x"}, "sk-x", &out, &errb) != 1 || !strings.HasPrefix(errb.String(), "failed:") || !strings.Contains(errb.String(), "upstream error") {
		t.Fatal(errb.String())
	}
	serve(t, []string{
		`{"code":0,"message":"ok","data":{"id":"f2","status":"pending"}}`,
		`{"code":0,"message":"ok","data":{"id":"f2","status":"failed","errorMessage":"blocked","failureKind":"moderation"}}`,
	})
	errb.Reset()
	if run([]string{"x"}, "sk-x", &out, &errb) != 1 || !strings.HasPrefix(errb.String(), "rejected:") {
		t.Fatal(errb.String())
	}
}

func TestErrors(t *testing.T) {
	for _, c := range []struct {
		body string
		kind string
		exit int
	}{
		{`{"code":-1,"message":"invalid API key — create one at https://aigifmaker.ai/settings/apikeys"}`, "auth", 3},
		{`{"code":0,"data":{"wall":true,"reason":"anon_credits","cost":1,"remaining":0,"grant":0}}`, "auth", 3},
		{`{"code":-1,"message":"insufficient credits"}`, "quota", 4},
		{`{"code":429,"message":"please wait before trying again"}`, "rejected", 1},
		{`{"code":-1,"message":"something else broke"}`, "other", 1},
	} {
		serve(t, []string{c.body})
		var out, errb bytes.Buffer
		code := run([]string{"x"}, "sk-x", &out, &errb)
		if code != c.exit || !strings.HasPrefix(errb.String(), c.kind+":") {
			t.Fatalf("%s: exit %d %q", c.body, code, errb.String())
		}
	}
}

func TestCredits(t *testing.T) {
	serve(t, []string{`{"code":0,"message":"ok","data":{"remainingCredits":42,"made":3}}`})
	var out, errb bytes.Buffer
	if run([]string{"credits"}, "sk-x", &out, &errb) != 0 || strings.TrimSpace(out.String()) != "42" {
		t.Fatal(out.String(), errb.String())
	}
	serve(t, []string{`{"code":-1,"message":"no auth, please sign in"}`})
	if run([]string{"-credits"}, "", &out, &errb) != 3 {
		t.Fatal("no auth should be exit 3")
	}
}

// Live, free (no generation): AIGIFMAKER_LIVE=1 go test -run Live -v
func TestLiveSmoke(t *testing.T) {
	if os.Getenv("AIGIFMAKER_LIVE") == "" {
		t.Skip("set AIGIFMAKER_LIVE=1")
	}
	baseURL, sleep = "https://aigifmaker.ai", time.Sleep
	var out, errb bytes.Buffer
	if code := run([]string{"credits"}, "sk-invalidtest000", &out, &errb); code != 3 || !strings.HasPrefix(errb.String(), "auth:") {
		t.Fatalf("wrong key: exit %d %q", code, errb.String())
	}
	errb.Reset()
	if code := run([]string{"-no-wait", "a cat waving"}, "", &out, &errb); code != 3 || !strings.HasPrefix(errb.String(), "auth:") {
		t.Fatalf("no key: exit %d %q", code, errb.String())
	}
}
