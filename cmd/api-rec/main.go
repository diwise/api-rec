package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/diwise/api-rec/internal/pkg/application"
	"github.com/diwise/api-rec/internal/pkg/infrastructure/database"
	"github.com/diwise/api-rec/internal/pkg/presentation/api"
	"github.com/diwise/service-chassis/pkg/infrastructure/buildinfo"
	"github.com/diwise/service-chassis/pkg/infrastructure/env"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const serviceName string = "api-rec"

var recInputDataFile string

func main() {
	serviceVersion := buildinfo.SourceVersion()
	ctx, _, cleanup := o11y.Init(context.Background(), serviceName, serviceVersion)
	defer cleanup()

	flag.StringVar(&recInputDataFile, "input", "/opt/diwise/config/rec.csv", "A file containing a known REC structure (spaces, buildings, sensors...)")
	flag.Parse()

	db, err := database.Connect(ctx, database.LoadConfiguration(ctx))
	if err != nil {
		fatal(ctx, "connect failed", err)
	}

	err = db.Init(ctx)
	if err != nil {
		fatal(ctx, "init failed", err)
	}

	if _, err := os.Stat(recInputDataFile); err == nil {
		func() {
			f, err := os.Open(recInputDataFile)
			if err != nil {
				fatal(ctx, fmt.Sprintf("failed to open input data file %s", recInputDataFile), err)
			}
			defer f.Close()

			err = db.Seed(ctx, f)
			if err != nil {
				fatal(ctx, "failed to seed database", err)
			}
		}()
	}

	app := application.New(db)

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(10 * time.Second))

	api.RegisterEndpoints(ctx, router, app)

	servicePort := env.GetVariableOrDefault(ctx, "SERVICE_PORT", "8080")
	err = http.ListenAndServe(":"+servicePort, router)
	if err != nil {
		fatal(ctx, "failed to start request router", err)
	}
}

func fatal(ctx context.Context, msg string, err error) {
	logger := logging.GetFromContext(ctx)
	logger.Error(msg, "err", err.Error())
	os.Exit(1)
}
