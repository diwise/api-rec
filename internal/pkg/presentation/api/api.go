package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/diwise/api-rec/internal/pkg/infrastructure/database"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/tracing"
	"github.com/go-chi/chi/v5"
	"github.com/rs/cors"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("api-rec/api")

type hydraCollectionResult struct {
	Context    string `json:"@context"`
	Id         string `json:"@id"`
	Type       string `json:"@type"`
	TotalItems int    `json:"hydra:totalItems"`
	Member     any    `json:"hydra:member"`
}

func newHydraCollectionResult(id string, member any, totalItems int) hydraCollectionResult {
	return hydraCollectionResult{
		Context:    "http://www.w3.org/ns/hydra/context.jsonld",
		Id:         id,
		Type:       "hydra:Collection",
		TotalItems: totalItems,
		Member:     member,
	}
}

func RegisterEndpoints(ctx context.Context, r *chi.Mux, db database.Database) {
	r.Use(cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowCredentials: true,
		Debug:            false,
	}).Handler)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Route("/api", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Route("/spaces", func(r chi.Router) {
				r.Get("/", getEntities(ctx, db, database.SpaceType))
				r.Post("/", createEntity(ctx, db))
			})
			r.Route("/buildings", func(r chi.Router) {
				r.Get("/", getEntities(ctx, db, database.BuildingType))
				r.Post("/", createEntity(ctx, db))
			})
			r.Route("/sensors", func(r chi.Router) {
				r.Get("/", getEntities(ctx, db, database.SensorType))
				r.Post("/", createEntity(ctx, db))
			})
			r.Route("/observations", func(r chi.Router) {
				r.Get("/", GetObservations(ctx, db))
				r.Post("/", CreateObservation(ctx, db))
			})
		})
	})
}

func createEntity(ctx context.Context, db database.Database) http.HandlerFunc {
	log := logging.GetFromContext(ctx)

	return func(w http.ResponseWriter, r *http.Request) {
		var err error
		defer r.Body.Close()

		ctx, span := tracer.Start(r.Context(), "create-entity")
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()
		_, ctx, requestLogger := o11y.AddTraceIDToLoggerAndStoreInContext(span, log, ctx)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			requestLogger.Error().Err(err).Msg("unable to read body")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var e database.Entity
		err = json.Unmarshal(body, &e)
		if err != nil {
			requestLogger.Error().Err(err).Msg("unable to unmarshal body")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		err = db.AddEntity(ctx, e)
		if err != nil {
			requestLogger.Error().Err(err).Msgf("unable to add entity [%s]", e.Type)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		e, err = db.GetEntity(ctx, e.Id, e.Type)
		if err != nil {
			requestLogger.Error().Err(err).Msgf("unable to fetch entity [%s]", e.Type)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		b, err := json.Marshal(e)
		if err != nil {
			requestLogger.Error().Err(err).Msgf("unable marshal entity [%s]", e.Type)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Add("Content-Type", "application/ld+json")
		w.WriteHeader(http.StatusCreated)
		w.Write(b)
	}
}

func getEntities(ctx context.Context, db database.Database, entityType string) http.HandlerFunc {
	log := logging.GetFromContext(ctx)

	return func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx, span := tracer.Start(r.Context(), fmt.Sprintf("get-%s", entityType))
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()
		_, ctx, requestLogger := o11y.AddTraceIDToLoggerAndStoreInContext(span, log, ctx)

		root, rootOk := getRootEntity(ctx, r, db)

		var entities []database.Entity

		if rootOk {
			entities, err = db.GetChildEntities(ctx, root, entityType)
			if err != nil {
				requestLogger.Error().Err(err).Msg("could not load entities from root entity")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		} else {
			entities, err = db.GetEntities(ctx, entityType)
			if err != nil {
				requestLogger.Error().Err(err).Msgf("unable to load %s", entityType)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}

		result := newHydraCollectionResult(r.URL.String(), entities, len(entities))

		b, err := json.Marshal(result)
		if err != nil {
			requestLogger.Error().Err(err).Msgf("unable marshal result")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Add("Content-Type", "application/ld+json")
		w.WriteHeader(http.StatusCreated)
		w.Write(b)
	}
}

func getRootEntity(ctx context.Context, r *http.Request, db database.Database) (database.Entity, bool) {
	rootId := r.URL.Query().Get("root[id]")
	if rootId == "" {
		return database.Entity{}, false
	}

	rootType := r.URL.Query().Get("root[type]")
	if rootType == "" {
		return database.Entity{}, false
	}

	root, err := db.GetEntity(ctx, rootId, database.GetTypeFromTypeName(rootType))
	if err != nil {
		return database.Entity{}, false
	}

	return root, true
}

func GetObservations(ctx context.Context, db database.Database) http.HandlerFunc {
	log := logging.GetFromContext(ctx)

	return func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx, span := tracer.Start(r.Context(), "get-sensors")
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()
		_, ctx, requestLogger := o11y.AddTraceIDToLoggerAndStoreInContext(span, log, ctx)

		sensorId := r.URL.Query().Get("sensor_id")
		if sensorId == "" {
			requestLogger.Error().Err(err).Msg("no ID in query string")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		startingTime, _ := time.Parse(time.RFC3339, "1970-01-01")
		endingTime := time.Now().UTC()

		starting := r.URL.Query().Get("hasObservationTime[starting]")
		if starting != "" {
			startingTime, err = time.Parse(time.RFC3339, starting)
			if err != nil {
				requestLogger.Error().Err(err).Msg("starting time in wrong format")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}

		ending := r.URL.Query().Get("hasObservationTime[ending]")
		if starting != "" {
			endingTime, err = time.Parse(time.RFC3339, ending)
			if err != nil {
				requestLogger.Error().Err(err).Msg("ending time in wrong format")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}

		observations, err := db.GetObservations(ctx, sensorId, startingTime, endingTime)
		if err != nil {
			requestLogger.Error().Err(err).Msg("could not load observations")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		result := newHydraCollectionResult(r.URL.String(), observations, len(observations))

		b, err := json.Marshal(result)
		if err != nil {
			requestLogger.Error().Err(err).Msgf("unable marshal observations result")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Add("Content-Type", "application/ld+json")
		w.WriteHeader(http.StatusCreated)
		w.Write(b)
	}
}

func CreateObservation(ctx context.Context, db database.Database) http.HandlerFunc {
	log := logging.GetFromContext(ctx)

	return func(w http.ResponseWriter, r *http.Request) {
		var err error
		defer r.Body.Close()

		ctx, span := tracer.Start(r.Context(), "create-observation")
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()
		_, ctx, requestLogger := o11y.AddTraceIDToLoggerAndStoreInContext(span, log, ctx)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			requestLogger.Error().Err(err).Msg("unable to read body")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var so database.SensorObservation
		err = json.Unmarshal(body, &so)
		if err != nil {
			requestLogger.Error().Err(err).Msg("unable to unmarshal body")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		err = db.AddObservation(ctx, so)
		if err != nil {
			requestLogger.Error().Err(err).Msg("unable to create observation")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}
