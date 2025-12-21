package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type ChatRequest struct {
	Message string `json:"message"`
}

type ChatReply struct {
	Reply string `json:"reply"`
	Error string `json:"error,omitempty"`
}

// global conversation for now (single user demo)
var (
	messages []Message
	mu       sync.Mutex
)

func main() {
	apiKey := os.Getenv("CEREBRAS_API_KEY")
	if apiKey == "" {
		fmt.Println("Missing CEREBRAS_API_KEY environment variable")
		return
	}

	// init conversation with a system message
	messages = []Message{
		{Role: "system", Content: "You are a helpful assistant."},
	}

	http.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		// ✅ CORS FIRST — ALWAYS
		enableCORS(w, r)

		// ✅ Handle preflight
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
			return
		}

		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, "Invalid JSON: "+err.Error())
			return
		}
		if req.Message == "" {
			writeError(w, "Message is required")
			return
		}

		mu.Lock()
		defer mu.Unlock()

		messages = append(messages, Message{
			Role:    "user",
			Content: req.Message,
		})

		payload := map[string]interface{}{
			"model":    "llama3.1-8b",
			"messages": messages,
		}

		jsonData, err := json.Marshal(payload)
		if err != nil {
			writeError(w, "Marshal error: "+err.Error())
			return
		}

		httpReq, err := http.NewRequest(
			"POST",
			"https://api.cerebras.ai/v1/chat/completions",
			bytes.NewBuffer(jsonData),
		)
		if err != nil {
			writeError(w, "Request creation error: "+err.Error())
			return
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+os.Getenv("CEREBRAS_API_KEY"))

		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			writeError(w, "API call error: "+err.Error())
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			writeError(w, "Read response error: "+err.Error())
			return
		}

		if resp.StatusCode != http.StatusOK {
			writeError(w, fmt.Sprintf("API error (%s): %s", resp.Status, body))
			return
		}

		var apiRes ChatResponse
		if err := json.Unmarshal(body, &apiRes); err != nil {
			writeError(w, "Unmarshal error: "+err.Error())
			return
		}

		reply := apiRes.Choices[0].Message.Content

		messages = append(messages, Message{
			Role:    "assistant",
			Content: reply,
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ChatReply{Reply: reply})
	})

	port := os.Getenv("PORT")
	if port == "" {
		// Local dev fallback
		port = "8080"
	}

	log.Printf("Starting server on :%s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func writeError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(ChatReply{Error: msg})
}

func enableCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")

	if origin == "https://dibinxavier.github.io" ||
		origin == "http://localhost:5500" ||
		origin == "https://bodha-zeta.vercel.app" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}

	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}
