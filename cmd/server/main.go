package main

import (
	"log"
	"net/http"
	"time"

	"github.com/louislw/AIDataAnalysis/internal/config"
	"github.com/louislw/AIDataAnalysis/internal/datasource"
	api "github.com/louislw/AIDataAnalysis/internal/http"
	"github.com/louislw/AIDataAnalysis/internal/llm"
	"github.com/louislw/AIDataAnalysis/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(cfg.DataRoot)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()
	if err := st.Bootstrap(cfg.BootstrapAdminPassword, cfg.BootstrapUserPassword); err != nil {
		log.Fatal(err)
	}
	handler := api.New(cfg, st, datasource.New(cfg.DataRoot), llm.HTTPClient{
		BaseURL: cfg.LLMBaseURL,
		APIKey:  cfg.LLMAPIKey,
		Model:   cfg.LLMModel,
		Timeout: cfg.LLMTimeout,
	})
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("listening on %s", cfg.Addr)
	log.Fatal(server.ListenAndServe())
}
