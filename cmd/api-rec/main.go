package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"time"

	"github.com/diwise/api-rec/internal/pkg/application"
	"github.com/diwise/api-rec/internal/pkg/infrastructure/database"
	"github.com/diwise/api-rec/internal/pkg/presentation/api"
	"github.com/diwise/service-chassis/pkg/infrastructure/buildinfo"
	"github.com/diwise/service-chassis/pkg/infrastructure/env"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const serviceName string = "api-rec"

var recInputDataFile string

func main() {
	serviceVersion := buildinfo.SourceVersion()
	ctx, logger, cleanup := o11y.Init(context.Background(), serviceName, serviceVersion)
	defer cleanup()

	flag.StringVar(&recInputDataFile, "input", "/opt/diwise/config/rec.csv", "A file containing a known REC structure (spaces, buildings, sensors...)")
	flag.Parse()

	db, err := database.Connect(ctx, database.LoadConfiguration(ctx))
	if err != nil {
		logger.Fatal().Err(err).Msg("connect failed")
	}

	err = db.Init(ctx)
	if err != nil {
		logger.Fatal().Err(err).Msg("init failed")
	}

	if _, err := os.Stat(recInputDataFile); err == nil {
		func() {
			f, err := os.Open(recInputDataFile)
			if err != nil {
				logger.Fatal().Err(err).Msgf("failed to open input data file %s", recInputDataFile)
			}
			defer f.Close()

			err = db.Seed(ctx, f)
			if err != nil {
				logger.Fatal().Err(err).Msg("failed to seed database")
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

	servicePort := env.GetVariableOrDefault(logger, "SERVICE_PORT", "8080")
	err = http.ListenAndServe(":"+servicePort, router)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to start request router")
	}
}
