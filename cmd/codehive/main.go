package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/codehive/codehive/internal/config"
	"github.com/codehive/codehive/internal/database"
	"github.com/codehive/codehive/internal/gitbackend"
	"github.com/codehive/codehive/internal/ssh"
	"github.com/codehive/codehive/internal/store"
	"github.com/codehive/codehive/internal/web"
	"github.com/codehive/codehive/internal/webhook"
)

var Version = "dev"

func main() {
	configPath := flag.String("config", "codehive.yaml", "path to configuration file")
	migrateOnly := flag.Bool("migrate", false, "run migrations and exit")
	showVersion := flag.Bool("version", false, "show version")
	flag.Parse()

	if *showVersion {
		fmt.Println("CodeHive", Version)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	db, err := database.Connect(cfg.Database.DSN)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	if *migrateOnly {
		log.Println("Migrations complete")
		return
	}

	if err := os.MkdirAll(cfg.Git.DataDir, 0755); err != nil {
		log.Fatalf("Failed to create git data dir: %v", err)
	}
	if err := os.MkdirAll(cfg.Packages.DataDir, 0755); err != nil {
		log.Fatalf("Failed to create packages data dir: %v", err)
	}

	userStore := store.NewUserStore(db)
	repoStore := store.NewRepoStore(db)
	issueStore := store.NewIssueStore(db)
	sessionStore := store.NewSessionStore(db)
	tokenStore := store.NewTokenStore(db)
	prStore := store.NewPRStore(db)
	orgStore := store.NewOrgStore(db)
	auditStore := store.NewAuditStore(db)
	webhookStore := store.NewWebhookStore(db)
	packageStore := store.NewPackageStore(db)

	gitSvc := gitbackend.NewService(cfg.Git.DataDir)
	webhookDispatcher := webhook.NewDispatcher(webhookStore)

	sshServer := ssh.NewServer(cfg, userStore, repoStore, gitSvc)
	go func() {
		log.Printf("SSH server listening on :%d", cfg.SSH.Port)
		if err := sshServer.ListenAndServe(); err != nil {
			log.Fatalf("SSH server error: %v", err)
		}
	}()

	httpServer := web.NewServer(cfg, userStore, repoStore, issueStore, sessionStore, tokenStore,
		prStore, orgStore, auditStore, webhookStore, packageStore, gitSvc, webhookDispatcher)
	srv := &http.Server{
		Addr:           fmt.Sprintf(":%d", cfg.HTTP.Port),
		Handler:        httpServer.Router(),
		ReadTimeout:    30 * time.Minute,
		WriteTimeout:   30 * time.Minute,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		log.Printf("HTTP server listening on :%d", cfg.HTTP.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	sshServer.Close()
}
