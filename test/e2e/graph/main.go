//go:build real

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

// startHttpServer - private function
func startHttpServer() {
	mux := http.NewServeMux()
	// used for customized cincinnatti graph search
	mux.HandleFunc("/graph", func(w http.ResponseWriter, req *http.Request) {
		renderGraph(w, req)
	})

	http.Handle("/", mux)

	if err := http.ListenAndServeTLS(":3443", "test/e2e/graph/server.crt", "test/e2e/graph/server.key", nil); err != nil {
		log.Println("Error: HttpsServer: ListenAndServeTLS() : ", err)
		os.Exit(1)
	}
}

func renderGraph(w http.ResponseWriter, req *http.Request) {
	data, err := os.ReadFile("test/e2e/graph/cincinnatti.json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", "{\"statusCode\":\"500\", \"message\":\"failed to read graph data\"}")
	}
	fmt.Fprintf(w, "%s", string(data))
}

// main - needs no explanation :)
func main() {
	log.Println("Starting tls server on port 3443")
	startHttpServer()
}
