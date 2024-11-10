// main.go
package main

import (
	"io"
	"log"
	"net/http"
)

func startEchoServer(port string) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(port + "|" + string(body)))
	})

	log.Printf("Echo server starting on port %s", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal(err)
	}
}

func main() {

	go startEchoServer(":8081")
	go startEchoServer(":8082")

	lb := NewLoadBalancer()
	err := lb.AddNode("http://localhost:8081", 1000, 60)
	if err != nil {
		return
	}
	err = lb.AddNode("http://localhost:8082", 2000, 120)
	if err != nil {
		return
	}

	log.Println("Starting load balancer on :8080")

	if err = lb.Start(":8080"); err != nil {
		log.Fatal(err)
	}
}
