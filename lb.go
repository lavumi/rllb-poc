// lb.go
package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

type RateLimit struct {
	bpm int64
	rpm int64
}

type Node struct {
	URL       *url.URL
	RateLimit RateLimit
	proxy     *httputil.ReverseProxy
	byteCount int64
	reqCount  int64
	lastReset time.Time
	mu        sync.Mutex
}

func (n *Node) checkRateLimit(bodySize int64) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	now := time.Now()
	if now.Sub(n.lastReset) >= time.Minute {
		n.byteCount = 0
		n.reqCount = 0
		n.lastReset = now
	}

	if n.reqCount >= n.RateLimit.rpm {
		return false
	}

	if n.byteCount+bodySize > n.RateLimit.bpm {
		return false
	}

	n.reqCount++
	n.byteCount += bodySize

	return true
}

type LoadBalancer struct {
	nodes []*Node
	index int
	mu    sync.Mutex
}

func NewLoadBalancer() *LoadBalancer {
	return &LoadBalancer{
		nodes: make([]*Node, 0),
	}
}

func (lb *LoadBalancer) AddNode(rawURL string, bpm, rpm int64) error {
	nodeURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(nodeURL)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		if req.Header.Get("Accept") == "text/event-stream" {
			req.Header.Set("Connection", "keep-alive")
			req.Header.Set("Cache-Control", "no-cache")
		}
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.Header.Get("Content-Type") == "text/event-stream" {
			resp.Header.Set("Connection", "keep-alive")
			resp.Header.Set("Cache-Control", "no-cache")
			resp.Header.Set("Transfer-Encoding", "chunked")
		}
		return nil
	}

	node := &Node{
		URL: nodeURL,
		RateLimit: RateLimit{
			bpm: bpm,
			rpm: rpm,
		},
		proxy:     proxy,
		lastReset: time.Now(),
	}

	lb.nodes = append(lb.nodes, node)
	return nil
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	var selectedNode *Node
	for _, node := range lb.nodes {
		if node.checkRateLimit(int64(len(body))) {
			selectedNode = node
			break
		}
	}

	if selectedNode == nil {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	r.Body = io.NopCloser(bytes.NewReader(body))

	selectedNode.proxy.ServeHTTP(w, r)
}

func (lb *LoadBalancer) Start(port string) error {
	if len(lb.nodes) == 0 {
		return fmt.Errorf("no nodes available")
	}

	server := &http.Server{
		Addr:    port,
		Handler: lb,
	}

	return server.ListenAndServe()
}
