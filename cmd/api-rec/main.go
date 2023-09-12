package main

import (
	"context"
	"flag"
	"net/http"
	"os"

	"github.com/diwise/api-rec/internal/pkg/application"
	"github.com/diwise/api-rec/internal/pkg/infrastructure/database"
	"github.com/diwise/api-rec/internal/pkg/presentation/api"
	"github.com/diwise/service-chassis/pkg/infrastructure/buildinfo"
	"github.com/diwise/service-chassis/pkg/infrastructure/env"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"

	"github.com/go-chi/chi/v5"
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

	r := chi.NewRouter()

	api.RegisterEndpoints(ctx, r, app)

	servicePort := env.GetVariableOrDefault(logger, "SERVICE_PORT", "8080")
	err = http.ListenAndServe(":"+servicePort, r)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to start request router")
	}
}
