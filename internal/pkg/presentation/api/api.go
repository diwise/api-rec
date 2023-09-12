package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/diwise/api-rec/internal/pkg/application"
	"github.com/diwise/api-rec/internal/pkg/infrastructure/database"
	"github.com/diwise/service-chassis/pkg/infrastructure/env"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/tracing"
	"github.com/go-chi/chi/v5"
	"github.com/rs/cors"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("api-rec/api")

type hydraCollectionResult struct {
	Context    string                 `json:"@context"`
	Id         string                 `json:"@id"`
	Type       string                 `json:"@type"`
	TotalItems int                    `json:"hydra:totalItems"`
	Member     any                    `json:"hydra:member"`
	View       *partialCollectionView `json:"hydra:view,omitempty"`
}

type partialCollectionView struct {
	Id       string `json:"@id"`
	Type     string `json:"@type"`
	First    string `json:"first"`
	Previous string `json:"previous,omitempty"`
	Next     string `json:"next,omitempty"`
	Last     string `json:"last"`
}

var apiPath *string = nil

func urlPath(url *url.URL) string {
	if apiPath != nil {
		return *apiPath
	}
	p := env.GetVariableOrDefault(zerolog.Logger{}, "API_PATH", url.Path)
	apiPath = &p

	return *apiPath
}

func newHydraCollectionResult(url *url.URL, qry *url.Values, member any, page, size, totalItems int) hydraCollectionResult {
	r := hydraCollectionResult{
		Context:    "http://www.w3.org/ns/hydra/context.jsonld",
		Id:         url.Path,
		Type:       "hydra:Collection",
		TotalItems: totalItems,
		Member:     member,
	}

	if qry != nil {
		reqUri := urlPath(url) + "?" + qry.Encode()
		currentPageN := page
		firstPageN := 0
		previousPageN := currentPageN - 1
		nextPageN := currentPageN + 1
		lastPageN := totalItems / size

		getPageUrl := func(p int) string {
			if p < 0 {
				return ""
			}
			if p > lastPageN {
				return ""
			}
			return strings.Replace(reqUri, fmt.Sprintf("page=%d", page), fmt.Sprintf("page=%d", p), -1)
		}

		r.View = &partialCollectionView{
			Id:       url.RequestURI(),
			Type:     "hydra:PartialCollectionView",
			First:    getPageUrl(firstPageN),
			Previous: getPageUrl(previousPageN),
			Next:     getPageUrl(nextPageN),
			Last:     getPageUrl(lastPageN),
		}
	}

	return r
}

func RegisterEndpoints(ctx context.Context, r *chi.Mux, app application.Application) {
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
				r.Get("/", getEntities(ctx, app, database.SpaceType))
				r.Post("/", createEntity(ctx, app))
			})
			r.Route("/buildings", func(r chi.Router) {
				r.Get("/", getEntities(ctx, app, database.BuildingType))
				r.Post("/", createEntity(ctx, app))
			})
			r.Route("/sensors", func(r chi.Router) {
				r.Get("/", getEntities(ctx, app, database.SensorType))
				r.Post("/", createEntity(ctx, app))
			})
			r.Route("/observations", func(r chi.Router) {
				r.Get("/", getObservations(ctx, app))
				r.Post("/", createObservation(ctx, app))
			})
			r.Route("/cloudevents", func(r chi.Router) {
				r.Post("/", handleCloudevents(ctx, app))
			})
		})
	})
}

func createEntity(ctx context.Context, app application.Application) http.HandlerFunc {
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

		err = app.AddEntity(ctx, e)
		if err != nil {
			requestLogger.Error().Err(err).Msgf("unable to add entity [%s]", e.Type)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		e, err = app.GetEntity(ctx, e.Id, e.Type)
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

func getEntities(ctx context.Context, app application.Application, entityType string) http.HandlerFunc {
	log := logging.GetFromContext(ctx)

	return func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx, span := tracer.Start(r.Context(), fmt.Sprintf("get-%s", entityType))
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()
		_, ctx, requestLogger := o11y.AddTraceIDToLoggerAndStoreInContext(span, log, ctx)

		root, rootOk := getRootEntity(ctx, r, app)

		var entities []database.Entity

		page := 0
		size := 10
		var result hydraCollectionResult

		q := r.URL.Query()

		if p := q.Get("page"); p != "" {
			if i, err := strconv.ParseInt(p, 10, 32); err == nil {
				page = int(i)
			}
		} else {
			q.Add("page", "0")
		}

		if p := r.URL.Query().Get("size"); p != "" {
			if i, err := strconv.ParseInt(p, 10, 32); err == nil {
				size = int(i)
			}
		} else {
			q.Add("size", "10")
		}

		if rootOk {
			entities, err = app.GetChildEntities(ctx, root, entityType)
			if err != nil {
				requestLogger.Error().Err(err).Msg("could not load entities from root entity")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			result = newHydraCollectionResult(r.URL, nil, entities, -1, -1, len(entities))
		} else {
			totalItems, entities, err := app.GetEntities(ctx, entityType, page, size)
			if err != nil {
				requestLogger.Error().Err(err).Msgf("unable to load %s", entityType)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			result = newHydraCollectionResult(r.URL, &q, entities, page, size, int(totalItems))
		}

		b, err := json.Marshal(result)
		if err != nil {
			requestLogger.Error().Err(err).Msgf("unable marshal result")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Add("Content-Type", "application/ld+json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
	}
}

func getRootEntity(ctx context.Context, r *http.Request, app application.Application) (database.Entity, bool) {
	rootId := r.URL.Query().Get("root[id]")
	if rootId == "" {
		return database.Entity{}, false
	}

	rootType := r.URL.Query().Get("root[type]")
	if rootType == "" {
		return database.Entity{}, false
	}

	root, err := app.GetEntity(ctx, rootId, database.GetTypeFromTypeName(rootType))
	if err != nil {
		return database.Entity{}, false
	}

	return root, true
}

func getObservations(ctx context.Context, app application.Application) http.HandlerFunc {
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

		page := 0
		size := 10
		startingTime, _ := time.Parse(time.RFC3339, "1970-01-01")
		endingTime := time.Now().UTC()

		q := r.URL.Query()

		if p := q.Get("page"); p != "" {
			if i, err := strconv.ParseInt(p, 10, 32); err == nil {
				page = int(i)
			}
		} else {
			q.Add("page", "0")
		}

		if p := r.URL.Query().Get("size"); p != "" {
			if i, err := strconv.ParseInt(p, 10, 32); err == nil {
				size = int(i)
			}
		} else {
			q.Add("size", "10")
		}

		starting := q.Get("hasObservationTime[starting]")
		if starting != "" {
			startingTime, err = time.Parse(time.RFC3339, starting)
			if err != nil {
				requestLogger.Error().Err(err).Msg("starting time in wrong format")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}

		ending := q.Get("hasObservationTime[ending]")
		if ending != "" {
			endingTime, err = time.Parse(time.RFC3339, ending)
			if err != nil {
				requestLogger.Error().Err(err).Msg("ending time in wrong format")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}

		totalItems, observations, err := app.GetObservations(ctx, sensorId, startingTime, endingTime, page, size)
		if err != nil {
			requestLogger.Error().Err(err).Msg("could not load observations")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		result := newHydraCollectionResult(r.URL, &q, observations, page, size, int(totalItems))

		b, err := json.Marshal(result)
		if err != nil {
			requestLogger.Error().Err(err).Msgf("unable marshal observations result")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Add("Content-Type", "application/ld+json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
	}
}

func createObservation(ctx context.Context, app application.Application) http.HandlerFunc {
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

		err = app.AddObservation(ctx, so)
		if err != nil {
			requestLogger.Error().Err(err).Msg("unable to create observation")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}

func handleCloudevents(ctx context.Context, app application.Application) http.HandlerFunc {
	log := logging.GetFromContext(ctx)

	return func(w http.ResponseWriter, r *http.Request) {
		var err error
		defer r.Body.Close()

		ctx, span := tracer.Start(r.Context(), "handle-cloudevents")
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()
		_, ctx, requestLogger := o11y.AddTraceIDToLoggerAndStoreInContext(span, log, ctx)

		event, err := cloudevents.NewEventFromHTTPRequest(r)
		if err != nil {
			requestLogger.Error().Err(err).Msg("failed to parse CloudEvent from request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var observation database.SensorObservation
		var observationOk bool = false

		switch event.Type() {
		case application.MessageAcceptedName:
			var ma application.MessageAccepted
			err := json.Unmarshal(event.Data(), &ma)
			if err != nil {
				requestLogger.Error().Err(err).Msg("failed to parse message.accepted in cloud event")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			observation, observationOk = ma.MapToObservation()
		case application.FunctionUpdatedName:
			var fu application.FunctionUpdated
			err := json.Unmarshal(event.Data(), &fu)
			if err != nil {
				requestLogger.Error().Err(err).Msg("failed to parse function.updated in cloud event")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			observation, observationOk = fu.MapToObservation()
		}

		if observationOk {
			err = app.AddObservation(ctx, observation)
			if err != nil {
				requestLogger.Error().Err(err).Msg("failed to store observation")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			return
		} else {
			requestLogger.Error().Err(err).Msg("failed to map incomming message to observation")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
}
