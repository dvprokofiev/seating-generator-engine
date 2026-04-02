// Copyright (C) 2026 Прокофьев Даниил <d@dvprokofiev.ru>
// Лицензировано под GNU Affero General Public License v3.0
// Часть проекта генератора рассадок
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"seating-generator/ga"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

type Response struct {
	Seating []ga.Response
	Fitness float64
	Date    int64
	ID      string
}

func main() {
	log.SetFlags(log.LstdFlags)
	_ = godotenv.Load(".env")
	port := os.Getenv("PORT")
	if port == "" {
		log.Println("No PORT env found, defaulting to 5000")
		port = "5000"
	} else {
		log.Printf("Using PORT from environment: %s", port)
	}

	http.HandleFunc("/generate-seating", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		log.Printf("IN: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		generateSeatingHandler(w, r)

		log.Printf("OUT: Completed in %v", time.Since(start))
	})

	server := &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("Server starting on port %s...", port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func generateSeatingHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", os.Getenv("ALLOWED_ORIGIN"))
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	r.Body = http.MaxBytesReader(w, r.Body, 1*1024*1024)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ga.Request
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		log.Printf("JSON Decode Error: %v", err)
		if typeErr, ok := err.(*json.UnmarshalTypeError); ok {
			msg := fmt.Sprintf("Type error: expected %v in field %v, got %v", typeErr.Type, typeErr.Field, typeErr.Value)
			http.Error(w, msg, http.StatusBadRequest)
		} else {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		}
		return
	}
	if len(req.Students) > 80 {
		http.Error(w, "Too much students!", http.StatusBadRequest)
		log.Printf("Task: %d - canceled - too much students", len(req.Students))
		return
	} else if len(req.Students) > req.ClassConfig.Columns*req.ClassConfig.Rows {
		http.Error(w, "Classroom is too small for this much students", http.StatusBadRequest)
		log.Printf("Task: %d - canceled - classroom is too small for this much students", len(req.Students))
		return
	}
	log.Printf("Task: %d students, config %dx%d",
		len(req.Students), req.ClassConfig.Rows, req.ClassConfig.Columns)
	seating, fitness, totalGens := ga.RunGA(req)
	if seating == nil {
		log.Println("GA returned nil seating")
		http.Error(w, "Invalid input or no solution found", http.StatusBadRequest)
		return
	}
	log.Printf("Solved: Fitness=%f, took %d generations", fitness, totalGens)
	response := Response{
		Seating: seating,
		Fitness: fitness,
		Date:    time.Now().Unix(),
		ID:      uuid.New().String(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Response Encode Error: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
