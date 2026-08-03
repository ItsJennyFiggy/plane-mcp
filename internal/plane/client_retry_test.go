package plane

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ItsJennyFiggy/plane-mcp/internal/config"
)

func newRetryTestClient() *Client {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	return NewClient(cfg)
}

func TestRetry429ThenSuccessDelaySeconds(t *testing.T) {
	client := newRetryTestClient()
	var sleeps []time.Duration
	client.sleep = func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	}
	calls := 0
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"Retry-After": []string{"2"}},
				Body:       io.NopCloser(strings.NewReader("rate limited")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"id":"me-1","display_name":"Figgy"}`)),
		}, nil
	})

	me, err := client.GetMe(context.Background())
	if err != nil {
		t.Fatalf("GetMe failed: %v", err)
	}
	if me.ID != "me-1" {
		t.Errorf("expected me-1, got %s", me.ID)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
	if len(sleeps) != 1 || sleeps[0] != 2*time.Second {
		t.Errorf("expected single 2s sleep, got %v", sleeps)
	}
}

func TestRetry429ThenSuccessHTTPDate(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	client := newRetryTestClient()
	client.now = func() time.Time { return now }
	var sleeps []time.Duration
	client.sleep = func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	}
	later := now.Add(3 * time.Second)

	calls := 0
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"Retry-After": []string{later.Format(http.TimeFormat)}},
				Body:       io.NopCloser(strings.NewReader("rate limited")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"id":"me-1","display_name":"Figgy"}`)),
		}, nil
	})

	me, err := client.GetMe(context.Background())
	if err != nil {
		t.Fatalf("GetMe failed: %v", err)
	}
	if me.ID != "me-1" {
		t.Errorf("expected me-1, got %s", me.ID)
	}
	if len(sleeps) != 1 || sleeps[0] != 3*time.Second {
		t.Errorf("expected single 3s sleep, got %v", sleeps)
	}
}

func TestRetry429DelaySecondsCappedAtMax(t *testing.T) {
	client := newRetryTestClient()
	var sleeps []time.Duration
	client.sleep = func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	}
	calls := 0
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"Retry-After": []string{"1000"}},
				Body:       io.NopCloser(strings.NewReader("rate limited")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"id":"me-1","display_name":"Figgy"}`)),
		}, nil
	})

	if _, err := client.GetMe(context.Background()); err != nil {
		t.Fatalf("GetMe failed: %v", err)
	}
	if len(sleeps) != 1 || sleeps[0] != defaultRetryMaxDelay {
		t.Errorf("expected sleep capped at %v, got %v", defaultRetryMaxDelay, sleeps)
	}
}

func TestRetry429FallbackBackoff(t *testing.T) {
	for _, tc := range []struct {
		name       string
		header     string
		expected   []time.Duration
	}{
		{"missing header", "", []time.Duration{defaultRetryBaseDelay, defaultRetryBaseDelay * 2}},
		{"malformed header", "not-a-number", []time.Duration{defaultRetryBaseDelay, defaultRetryBaseDelay * 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newRetryTestClient()
			client.jitter = func() time.Duration { return 0 }
			var sleeps []time.Duration
			client.sleep = func(_ context.Context, d time.Duration) error {
				sleeps = append(sleeps, d)
				return nil
			}
			calls := 0
			client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls <= 2 {
					h := http.Header{}
					if tc.header != "" {
						h.Set("Retry-After", tc.header)
					}
					return &http.Response{
						StatusCode: http.StatusTooManyRequests,
						Header:     h,
						Body:       io.NopCloser(strings.NewReader("rate limited")),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"id":"me-1","display_name":"Figgy"}`)),
				}, nil
			})

			if _, err := client.GetMe(context.Background()); err != nil {
				t.Fatalf("GetMe failed: %v", err)
			}
			if calls != 3 {
				t.Errorf("expected 3 calls, got %d", calls)
			}
			if len(sleeps) != 2 {
				t.Fatalf("expected 2 sleeps, got %v", sleeps)
			}
			for i := range tc.expected {
				if sleeps[i] != tc.expected[i] {
					t.Errorf("expected sleep[%d]=%v, got %v", i, tc.expected[i], sleeps)
				}
			}
		})
	}
}

func TestRetry429BudgetExhausted(t *testing.T) {
	client := newRetryTestClient()
	client.sleep = func(_ context.Context, d time.Duration) error { return nil }
	calls := 0
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader("still rate limited")),
		}, nil
	})

	_, err := client.GetMe(context.Background())
	if err == nil {
		t.Fatal("expected error on exhausted retry budget, got nil")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("expected error to mention 429, got: %v", err)
	}
	if !strings.Contains(err.Error(), "still rate limited") {
		t.Errorf("expected error to preserve final response body, got: %v", err)
	}
	if calls != defaultRetryMaxAttempts {
		t.Errorf("expected %d total attempts, got %d", defaultRetryMaxAttempts, calls)
	}
}

func TestRetry429ContextCancelled(t *testing.T) {
	client := newRetryTestClient()
	ctx, cancel := context.WithCancel(context.Background())
	client.sleep = func(c context.Context, d time.Duration) error {
		cancel()
		return c.Err()
	}
	calls := 0
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader("rate limited")),
		}, nil
	})

	_, err := client.GetMe(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected no subsequent request after cancellation, got %d calls", calls)
	}
}

func TestRetry429BodyReplayOnMutatingRequest(t *testing.T) {
	client := newRetryTestClient()
	client.sleep = func(_ context.Context, d time.Duration) error { return nil }
	var bodies [][]byte
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		bodies = append(bodies, b)
		if len(bodies) == 1 {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       io.NopCloser(strings.NewReader("rate limited")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusCreated,
			Body:       io.NopCloser(strings.NewReader(`{"id":"wi-1","name":"Task"}`)),
		}, nil
	})

	item, err := client.CreateWorkItem(context.Background(), "proj-1", map[string]any{"name": "Task"})
	if err != nil {
		t.Fatalf("CreateWorkItem failed: %v", err)
	}
	if item.ID != "wi-1" {
		t.Errorf("expected wi-1, got %s", item.ID)
	}
	if len(bodies) != 2 {
		t.Fatalf("expected 2 request bodies, got %d", len(bodies))
	}
	if string(bodies[0]) != string(bodies[1]) {
		t.Errorf("request body not replayed identically: %q vs %q", bodies[0], bodies[1])
	}
	if !strings.Contains(string(bodies[1]), `"name":"Task"`) {
		t.Errorf("expected replayed body to contain name, got %q", bodies[1])
	}
}

func TestRetryNon429NoRetry(t *testing.T) {
	statusCases := []struct {
		name   string
		status int
		body   string
	}{
		{"400", http.StatusBadRequest, "bad request"},
		{"500", http.StatusInternalServerError, "server error"},
	}
	for _, tc := range statusCases {
		t.Run(tc.name, func(t *testing.T) {
			client := newRetryTestClient()
			client.sleep = func(_ context.Context, d time.Duration) error {
				t.Errorf("should not sleep on non-429")
				return nil
			}
			calls := 0
			client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{
					StatusCode: tc.status,
					Body:       io.NopCloser(strings.NewReader(tc.body)),
				}, nil
			})
			_, err := client.GetMe(context.Background())
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if calls != 1 {
				t.Errorf("expected exactly 1 request, got %d", calls)
			}
		})
	}

	t.Run("transport error", func(t *testing.T) {
		client := newRetryTestClient()
		client.sleep = func(_ context.Context, d time.Duration) error {
			t.Errorf("should not sleep on transport error")
			return nil
		}
		calls := 0
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			calls++
			return nil, errors.New("connection refused")
		})
		_, err := client.GetMe(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if calls != 1 {
			t.Errorf("expected exactly 1 request, got %d", calls)
		}
	})
}

type trackingCloser struct {
	io.ReadCloser
	closed *bool
}

func (c *trackingCloser) Close() error {
	*c.closed = true
	return c.ReadCloser.Close()
}

func TestRetry429ResponseBodyClosedBetweenAttempts(t *testing.T) {
	client := newRetryTestClient()
	client.sleep = func(_ context.Context, d time.Duration) error { return nil }
	var firstClosed bool
	calls := 0
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       &trackingCloser{ReadCloser: io.NopCloser(strings.NewReader("rate limited")), closed: &firstClosed},
			}, nil
		}
		if !firstClosed {
			t.Error("expected the 429 response body to be closed before the next attempt")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"id":"me-1","display_name":"Figgy"}`)),
		}, nil
	})

	if _, err := client.GetMe(context.Background()); err != nil {
		t.Fatalf("GetMe failed: %v", err)
	}
	if !firstClosed {
		t.Error("expected the 429 response body to be closed")
	}
}

func TestRetry429PaginatedListRecovers(t *testing.T) {
	client := newRetryTestClient()
	client.sleep = func(_ context.Context, d time.Duration) error { return nil }
	calls := 0
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       io.NopCloser(strings.NewReader("rate limited")),
			}, nil
		case 2:
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"results": [{"id": "p1", "name": "Project 1", "identifier": "P1"}],
					"next_cursor": "page-2-cursor",
					"next_page_results": true
				}`)),
			}, nil
		default:
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"results": [{"id": "p2", "name": "Project 2", "identifier": "P2"}],
					"next_cursor": "",
					"next_page_results": false
				}`)),
			}, nil
		}
	})

	projects, err := client.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
	if projects[0].ID != "p1" || projects[1].ID != "p2" {
		t.Errorf("unexpected projects: %+v", projects)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls (retry + 2 pages), got %d", calls)
	}
}