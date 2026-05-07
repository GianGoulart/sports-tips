package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sportstips/internal/api"
	"sportstips/internal/config"
	"sportstips/internal/ingestion"
	"sportstips/internal/predictions"
	"sportstips/internal/results"
	"sportstips/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	if err := store.RunMigrations(cfg.DatabaseURL); err != nil {
		log.Error("migrations", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("db", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	primaryClient := ingestion.NewOddsAPIClient(cfg.OddsAPIKey)
	fallbackClient := ingestion.NewOddsPapiClient(cfg.OddsPapiKey)

	// PredictionService wired in Phase 3 — stub satisfies interface for compilation
	var _ predictions.PredictionService = &predictions.StubPredictionService{}

	primaryResults := results.NewOddsAPIResultsClient(cfg.OddsAPIKey)
	var fallbackResults results.ResultsClient
	if cfg.FootballDataKey != "" {
		fallbackResults = results.NewFootballDataClient(cfg.FootballDataKey)
	}

	ingester := results.NewIngester(db, primaryResults, fallbackResults,
		[]string{
			"soccer_epl",
			"soccer_spain_la_liga",
			"soccer_italy_serie_a",
			"soccer_germany_bundesliga",
			"soccer_uefa_champs_league",
		},
		log)

	scheduler := ingestion.NewScheduler(db, primaryClient, fallbackClient,
		[]string{
			"soccer_epl",
			"soccer_spain_la_liga",
			"soccer_italy_serie_a",
			"soccer_germany_bundesliga",
			"soccer_uefa_champs_league",
		},
		log)

	go scheduler.Run(ctx)

	handler := api.NewHandler(db, cfg.JWTSecret, ingester)
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.ServerPort),
		Handler:      handler.Router(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("server starting", "port", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server", "err", err)
			cancel()
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx) //nolint:errcheck
}
