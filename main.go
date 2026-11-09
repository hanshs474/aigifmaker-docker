// Command aigifmaker turns text into an animated GIF on AI GIF Maker
// (https://aigifmaker.ai) and prints the GIF URL.
//
//	aigifmaker "a cat dancing in the rain, pixel art"
//	aigifmaker -no-wait "a rocket launching, retro cartoon"   # prints the task id
//	aigifmaker -status <id>                                   # JSON status of a task
//	aigifmaker credits                                        # credits left, free
//
// Set AIGIFMAKER_API_KEY (create a key at https://aigifmaker.ai/settings/apikeys).
// Exit codes: 0 ok, 1 failure, 2 usage, 3 key problem, 4 out of credits.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	version = "0.1.0"
	baseURL = "https://aigifmaker.ai"
	client  = &http.Client{Timeout: 60 * time.Second}
	sleep   = time.Sleep
)

const (
	keysURL    = "https://aigifmaker.ai/settings/apikeys"
	pricingURL = "https://aigifmaker.ai/pricing"
)

// apiError carries the reason a call failed, in terms of what to do next.
type apiError struct {
	Kind    string // auth quota rejected failed timeout other
	Message string
}

func (e *apiError) Error() string { return e.Kind + ": aigifmaker: " + e.Message }

// envelope is every answer: HTTP 200 with {"code":0,"message":"ok","data":{…}} or {"code":-1,"message":"…"}.
type envelope struct {
	Code    *int            `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func classify(code int, msg string) *apiError {
	low := strings.ToLower(msg)
	switch {
	case code != -1:
		return &apiError{Kind: "rejected", Message: msg} // content cooldown and similar
	case strings.Contains(low, "invalid api key") || strings.Contains(low, "no auth"):
		return &apiError{Kind: "auth", Message: msg}
	case strings.Contains(low, "insufficient credits"):
		return &apiError{Kind: "quota", Message: msg + " (buy credits at " + pricingURL + ")"}
	}
	if msg == "" {
		msg = "request failed"
	}
	return &apiError{Kind: "other", Message: msg}
}

func post(key, path string, body any, out any) error {
	var rd io.Reader = bytes.NewReader(nil)
	if body != nil {
		buf, _ := json.Marshal(body)
		rd = bytes.NewReader(buf)
	}
	req, _ := http.NewRequest("POST", baseURL+path, rd)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "aigifmaker-cli/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return &apiError{Kind: "other", Message: err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil || env.Code == nil {
		return &apiError{Kind: "other", Message: fmt.Sprintf("HTTP %d: unexpected answer", resp.StatusCode)}
	}
	if *env.Code != 0 {
		return classify(*env.Code, env.Message)
	}
	var wall struct{ Wall bool }
	_ = json.Unmarshal(env.Data, &wall)
	if wall.Wall {
		return &apiError{Kind: "auth", Message: "an API key is required; create one at " + keysURL}
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return &apiError{Kind: "other", Message: "unreadable answer"}
	}
	return nil
}

type task struct {
	ID           string   `json:"id"`
	Status       string   `json:"status"`
	Images       []string `json:"images"`
	Queued       bool     `json:"queued,omitempty"`
	QueueEta     float64  `json:"queueEta,omitempty"`
	ErrorMessage string   `json:"errorMessage,omitempty"`
	FailureKind  string   `json:"failureKind,omitempty"`
}

// requestBody is the exact JSON sent to /api/ai/generate. Only the prompt varies.
func requestBody(prompt string) map[string]any {
	return map[string]any{
		"provider":  "openlux",
		"mediaType": "image",
		"model":     "gif-maker-v1",
		"scene":     "text-to-image",
		"prompt":    prompt,
		"options":   map[string]any{"output_format": "gif"},
	}
}

func submit(key, prompt string) (string, error) {
	var t task
	if err := post(key, "/api/ai/generate", requestBody(prompt), &t); err != nil {
		return "", err
	}
	if t.ID == "" {
		return "", &apiError{Kind: "other", Message: "the service returned no task id"}
	}
	return t.ID, nil
}

func status(key, id string) (task, error) {
	var t task
	err := post(key, "/api/ai/query", map[string]string{"taskId": id}, &t)
	return t, err
}

func credits(key string) (int, error) {
	var d struct{ RemainingCredits int }
	err := post(key, "/api/user/get-user-credits", nil, &d)
	return d.RemainingCredits, err
}

func wait(key, id string, every, limit time.Duration) (string, error) {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		sleep(every)
		t, err := status(key, id)
		if err != nil {
			if e, ok := err.(*apiError); ok && e.Kind == "auth" {
				return "", err
			}
			continue // a dropped poll is not a failed run
		}
		switch t.Status {
		case "success":
			if len(t.Images) > 0 {
				return t.Images[0], nil
			}
			return "", &apiError{Kind: "other", Message: "finished without a GIF URL"}
		case "failed", "canceled":
			msg := t.ErrorMessage
			if msg == "" {
				msg = t.Status
			}
			if t.FailureKind == "moderation" {
				return "", &apiError{Kind: "rejected", Message: msg}
			}
			return "", &apiError{Kind: "failed", Message: msg + " (the credit is refunded)"}
		}
		// pending, processing, or queued: keep polling
	}
	return "", &apiError{Kind: "timeout", Message: "deadline passed; the GIF may still finish in your history on aigifmaker.ai (task " + id + ")"}
}

func main() { os.Exit(run(os.Args[1:], os.Getenv("AIGIFMAKER_API_KEY"), os.Stdout, os.Stderr)) }

func run(args []string, key string, stdout, stderr io.Writer) int {
	key = strings.TrimSpace(key)
	fs := flag.NewFlagSet("aigifmaker", flag.ContinueOnError)
	fs.SetOutput(stderr)
	noWait := fs.Bool("no-wait", false, "print the task id and exit without waiting")
	statusID := fs.String("status", "", "print the status of a task id as JSON")
	showCredits := fs.Bool("credits", false, "print the credits left (free)")
	timeout := fs.Duration("timeout", 15*time.Minute, "how long to wait for the GIF")
	showVersion := fs.Bool("version", false, "print the version")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: aigifmaker [flags] \"prompt\"\n       aigifmaker credits\n\nTurns text into an animated GIF on https://aigifmaker.ai and prints the GIF URL.\nNeeds AIGIFMAKER_API_KEY (%s).\n\n", keysURL)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	fail := func(err error) int {
		fmt.Fprintln(stderr, err)
		if e, ok := err.(*apiError); ok {
			switch e.Kind {
			case "auth":
				return 3
			case "quota":
				return 4
			}
		}
		return 1
	}
	rest := fs.Args()
	switch {
	case *showVersion:
		fmt.Fprintln(stdout, "aigifmaker", version)
		return 0
	case *showCredits || (len(rest) == 1 && rest[0] == "credits"):
		n, err := credits(key)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintln(stdout, n)
		return 0
	case *statusID != "":
		t, err := status(key, *statusID)
		if err != nil {
			return fail(err)
		}
		out, _ := json.Marshal(t)
		fmt.Fprintln(stdout, string(out))
		return 0
	}
	prompt := strings.Join(rest, " ")
	if strings.TrimSpace(prompt) == "" {
		fs.Usage()
		return 2
	}
	id, err := submit(key, prompt)
	if err != nil {
		return fail(err)
	}
	if *noWait {
		fmt.Fprintln(stdout, id)
		return 0
	}
	url, err := wait(key, id, 3*time.Second, *timeout)
	if err != nil {
		return fail(err)
	}
	fmt.Fprintln(stdout, url)
	return 0
}
