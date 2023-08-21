package main

import (
	"context"
	"net/http"

	"github.com/diwise/api-rec/internal/pkg/infrastructure/database"
	"github.com/diwise/api-rec/internal/pkg/presentation/api"
	"github.com/diwise/service-chassis/pkg/infrastructure/buildinfo"
	"github.com/diwise/service-chassis/pkg/infrastructure/env"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"

	"github.com/go-chi/chi/v5"
)

const serviceName string = "api-rec"

func main() {
	serviceVersion := buildinfo.SourceVersion()
	ctx, logger, cleanup := o11y.Init(context.Background(), serviceName, serviceVersion)
	defer cleanup()

	db, err := database.Connect(ctx, database.LoadConfiguration(ctx))
	if err != nil {
		logger.Fatal().Err(err)
	}

	r := chi.NewRouter()

	api.RegisterEndpoints(ctx, r, db)

	servicePort := env.GetVariableOrDefault(logger, "SERVICE_PORT", "8080")
	err = http.ListenAndServe(":"+servicePort, r)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to start request router")
	}
}
