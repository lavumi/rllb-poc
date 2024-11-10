// lb_test.go
package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadBalancer(t *testing.T) {
	echo1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Write([]byte("server1|" + string(body)))
	}))
	defer echo1.Close()

	echo2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Write([]byte("server2|" + string(body)))
	}))
	defer echo2.Close()

	t.Run("Test RPM Limit Failover", func(t *testing.T) {
		lb := NewLoadBalancer()
		lb.AddNode(echo1.URL, 1000, 3)
		lb.AddNode(echo2.URL, 1000, 5)

		server := httptest.NewServer(lb)
		defer server.Close()

		checkResponse := func(expectedPrefix string) error {
			resp, err := http.Post(server.URL, "text/plain", bytes.NewReader([]byte("test")))
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			if !bytes.HasPrefix(body, []byte(expectedPrefix)) {
				t.Errorf("Expected response prefix %s, got: %s", expectedPrefix, body)
			}
			return nil
		}

		for i := 0; i < 3; i++ {
			if err := checkResponse("server1"); err != nil {
				t.Fatalf("Request failed: %v", err)
			}
		}

		if err := checkResponse("server2"); err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("Test BPM Limit Failover", func(t *testing.T) {
		lb := NewLoadBalancer()
		lb.AddNode(echo1.URL, 100, 100)
		lb.AddNode(echo2.URL, 200, 100)

		server := httptest.NewServer(lb)
		defer server.Close()

		payload := bytes.Repeat([]byte("a"), 99)

		resp, err := http.Post(server.URL, "text/plain", bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("First request failed: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !bytes.HasPrefix(body, []byte("server1")) {
			t.Error("First request should go to server1")
		}

		resp, err = http.Post(server.URL, "text/plain", bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("Second request failed: %v", err)
		}
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if !bytes.HasPrefix(body, []byte("server2")) {
			t.Error("Second request should go to server2")
		}
	})

	t.Run("Test All Nodes Limited", func(t *testing.T) {
		lb := NewLoadBalancer()
		lb.AddNode(echo1.URL, 10, 1)
		lb.AddNode(echo2.URL, 10, 1)

		server := httptest.NewServer(lb)
		defer server.Close()

		resp, err := http.Post(server.URL, "text/plain", bytes.NewReader([]byte("test")))
		if err != nil {
			t.Fatalf("First request failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected first request to succeed, got status: %d", resp.StatusCode)
		}

		resp, err = http.Post(server.URL, "text/plain", bytes.NewReader([]byte("test")))
		if err != nil {
			t.Fatalf("Second request failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected second request to succeed, got status: %d", resp.StatusCode)
		}

		resp, err = http.Post(server.URL, "text/plain", bytes.NewReader([]byte("test")))
		if err != nil {
			t.Fatalf("Third request failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusTooManyRequests {
			t.Errorf("Expected third request to fail with 429, got: %d", resp.StatusCode)
		}
	})

	//t.Run("Test Rate Limit Reset", func(t *testing.T) {
	//	lb := NewLoadBalancer()
	//	lb.AddNode(echo1.URL, 100, 1) // 1 request per minute
	//
	//	server := httptest.NewServer(lb)
	//	defer server.Close()
	//
	//	resp, err := http.Post(server.URL, "text/plain", bytes.NewReader([]byte("test")))
	//	if err != nil {
	//		t.Fatal(err)
	//	}
	//	resp.Body.Close()
	//	if resp.StatusCode != http.StatusOK {
	//		t.Errorf("Expected first request to succeed, got: %d", resp.StatusCode)
	//	}
	//
	//	time.Sleep(time.Minute)
	//
	//	resp, err = http.Post(server.URL, "text/plain", bytes.NewReader([]byte("test")))
	//	if err != nil {
	//		t.Fatal(err)
	//	}
	//	resp.Body.Close()
	//	if resp.StatusCode != http.StatusOK {
	//		t.Errorf("Expected request after reset to succeed, got: %d", resp.StatusCode)
	//	}
	//})
}
