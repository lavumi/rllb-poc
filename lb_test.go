// lb_test.go
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

	sseServer1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("Streaming is not supported")
			return
		}

		for i := 0; i < 5; i++ {
			fmt.Fprintf(w, "data: {\"message\": \"server1 event %d\"}\n\n", i)
			flusher.Flush()
			time.Sleep(100 * time.Millisecond)
		}
	}))
	defer sseServer1.Close()

	sseServer2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("Streaming is not supported")
			return
		}

		for i := 0; i < 5; i++ {
			fmt.Fprintf(w, "data: {\"message\": \"server2 event %d\"}\n\n", i)
			flusher.Flush()
			time.Sleep(100 * time.Millisecond)
		}
	}))
	defer sseServer2.Close()

	t.Run("Test SSE Streaming", func(t *testing.T) {
		lb := NewLoadBalancer()
		lb.AddNode(sseServer1.URL, 1000, 5)
		lb.AddNode(sseServer2.URL, 1000, 5)

		server := httptest.NewServer(lb)
		defer server.Close()

		req, err := http.NewRequest("GET", server.URL, nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Accept", "text/event-stream")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.Header.Get("Content-Type") != "text/event-stream" {
			t.Errorf("Expected Content-Type text/event-stream, got %s",
				resp.Header.Get("Content-Type"))
		}

		scanner := bufio.NewScanner(resp.Body)
		messageCount := 0
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				messageCount++
				if !strings.Contains(line, "server1") &&
					!strings.Contains(line, "server2") {
					t.Errorf("Unexpected message format: %s", line)
				}
			}
		}

		if messageCount != 5 {
			t.Errorf("Expected 5 messages, got %d", messageCount)
		}
	})

	t.Run("Test SSE with Rate Limit", func(t *testing.T) {
		lb := NewLoadBalancer()
		lb.AddNode(sseServer1.URL, 50, 1)
		lb.AddNode(sseServer2.URL, 50, 1)

		server := httptest.NewServer(lb)
		defer server.Close()

		req1, _ := http.NewRequest("GET", server.URL, nil)
		req1.Header.Set("Accept", "text/event-stream")
		resp1, err := http.DefaultClient.Do(req1)
		if err != nil {
			t.Fatalf("First request failed: %v", err)
		}
		if resp1.StatusCode != http.StatusOK {
			t.Errorf("Expected first request to succeed, got %d", resp1.StatusCode)
		}
		resp1.Body.Close()

		req2, _ := http.NewRequest("GET", server.URL, nil)
		req2.Header.Set("Accept", "text/event-stream")
		resp2, err := http.DefaultClient.Do(req2)
		if err != nil {
			t.Fatalf("Second request failed: %v", err)
		}
		if resp2.StatusCode != http.StatusOK {
			t.Errorf("Expected second request to succeed, got %d", resp2.StatusCode)
		}
		resp2.Body.Close()

		req3, _ := http.NewRequest("GET", server.URL, nil)
		req3.Header.Set("Accept", "text/event-stream")
		resp3, err := http.DefaultClient.Do(req3)
		if err != nil {
			t.Fatalf("Third request failed: %v", err)
		}
		if resp3.StatusCode != http.StatusTooManyRequests {
			t.Errorf("Expected third request to fail with 429, got %d",
				resp3.StatusCode)
		}
		resp3.Body.Close()
	})

	t.Run("Test SSE Response Headers", func(t *testing.T) {
		lb := NewLoadBalancer()
		lb.AddNode(sseServer1.URL, 1000, 5)

		server := httptest.NewServer(lb)
		defer server.Close()

		req, _ := http.NewRequest("GET", server.URL, nil)
		req.Header.Set("Accept", "text/event-stream")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		expectedHeaders := map[string]string{
			"Content-Type":  "text/event-stream",
			"Cache-Control": "no-cache",
			"Connection":    "keep-alive",
		}

		for header, expectedValue := range expectedHeaders {
			if value := resp.Header.Get(header); value != expectedValue {
				t.Errorf("Expected %s header to be %s, got %s",
					header, expectedValue, value)
			}
		}
	})
}
