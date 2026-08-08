// Command oauth-mock-server is a lightweight stand-in for Google's OAuth token
// endpoint used by the qa-local docker stack and manual smoke tests.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

var mockToken = tokenResponse{
	AccessToken:  "mock-access",
	RefreshToken: "mock-refresh",
	TokenType:    "Bearer",
	ExpiresIn:    3600,
}

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		tokenHandler(w, r)
	})

	log.Printf("oauth mock token server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, handler))
}

func tokenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(mockToken); err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
	}
}
