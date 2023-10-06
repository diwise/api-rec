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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
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

type key int

var settingsKey key

type apiSettings struct {
	ApiPath string
}

func (a apiSettings) Replace(urlPath string) string {
	if a.ApiPath != "" {
		return strings.Replace(urlPath, "/api", a.ApiPath, 1)
	}
	return urlPath
}

func getIntOrDefault(url *url.URL, key string, i int) int {
	value := url.Query().Get(key)
	if value == "" {
		return i
	}
	v, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return i
	}
	return int(v)
}

func getTimeOrDefault(url *url.URL, key string, t time.Time) (time.Time, error) {
	st := url.Query().Get(key)
	if st == "" {
		return t, nil
	}
	pt, err := time.Parse(time.RFC3339, st)
	if err != nil {
		return time.Unix(0, 0), err
	}
	return pt, nil
}

func newHydraCollectionResult(ctx context.Context, url *url.URL, member any, totalItems int) hydraCollectionResult {
	r := hydraCollectionResult{
		Context:    "http://www.w3.org/ns/hydra/context.jsonld",
		Id:         url.Path,
		Type:       "hydra:Collection",
		TotalItems: totalItems,
		Member:     member,
	}

	q := url.Query()
	page := getIntOrDefault(url, "page", 0)
	size := getIntOrDefault(url, "size", 10)

	if q.Has("page") {
		q.Set("page", fmt.Sprintf("%d", page))
	} else {
		q.Add("page", fmt.Sprintf("%d", page))
	}

	if q.Has("size") {
		q.Set("size", fmt.Sprintf("%d", size))
	} else {
		q.Add("size", fmt.Sprintf("%d", size))
	}

	if totalItems > size {
		settings := ctx.Value(settingsKey).(apiSettings)

		reqUri := settings.Replace(url.Path) + "?" + q.Encode()

		previous := page - 1
		next := page + 1
		last := totalItems / size

		getPageUrl := func(p int) string {
			if p < 0 {
				return ""
			}
			if p > last {
				return ""
			}
			return strings.Replace(reqUri, fmt.Sprintf("page=%d", page), fmt.Sprintf("page=%d", p), -1)
		}

		r.View = &partialCollectionView{
			Id:       url.RequestURI(),
			Type:     "hydra:PartialCollectionView",
			First:    getPageUrl(0),
			Previous: getPageUrl(previous),
			Next:     getPageUrl(next),
			Last:     getPageUrl(last),
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
			r.Use(SettingsCtx)

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

func SettingsCtx(next http.Handler) http.Handler {
	apiPath := env.GetVariableOrDefault(context.Background(), "API_PATH", "")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		settings := apiSettings{
			ApiPath: apiPath,
		}

		ctx := context.WithValue(r.Context(), settingsKey, settings)
		next.ServeHTTP(w, r.WithContext(ctx))
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
			requestLogger.Error("unable to read body", "err", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var e database.Entity
		err = json.Unmarshal(body, &e)
		if err != nil {
			requestLogger.Error("unable to unmarshal body", "err", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		err = app.AddEntity(ctx, e)
		if err != nil {
			requestLogger.Error("unable to add entity", "type", e.Type, "err", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		e, err = app.GetEntity(ctx, e.Id, e.Type)
		if err != nil {
			requestLogger.Error("unable to fetch entity", "type", e.Type, "err", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		b, err := json.Marshal(e)
		if err != nil {
			requestLogger.Error("unable marshal entity", "type", e.Type, "err", err.Error())
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
		var result hydraCollectionResult

		if rootOk {
			entities, err = app.GetChildEntities(ctx, root, entityType)
			if err != nil {
				requestLogger.Error("could not load entities from root entity", "err", err.Error())
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			result = newHydraCollectionResult(ctx, r.URL, entities, len(entities))
		} else {
			totalItems, entities, err := app.GetEntities(ctx, entityType, getIntOrDefault(r.URL, "page", 0), getIntOrDefault(r.URL, "size", 10))
			if err != nil {
				requestLogger.Error("unable to load entities", "type", entityType, "err", err.Error())
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			result = newHydraCollectionResult(ctx, r.URL, entities, int(totalItems))
		}

		b, err := json.Marshal(result)
		if err != nil {
			requestLogger.Error("unable marshal result", "err", err.Error())
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

		ctx, span := tracer.Start(r.Context(), "get-observations")
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()
		_, ctx, requestLogger := o11y.AddTraceIDToLoggerAndStoreInContext(span, log, ctx)

		sensorId := r.URL.Query().Get("sensorId")
		if sensorId == "" {
			requestLogger.Error("no ID in query string", "err", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		startingTime, err := getTimeOrDefault(r.URL, "hasObservationTime[starting]", time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC))
		if err != nil {
			requestLogger.Error("starting time in wrong format, must be RFC3339", "err", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		endingTime, err := getTimeOrDefault(r.URL, "hasObservationTime[ending]", time.Now().UTC())
		if err != nil {
			requestLogger.Error("ending time in wrong format, must be RFC3339", "err", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		totalItems, observations, err := app.GetObservations(ctx, sensorId, startingTime, endingTime, getIntOrDefault(r.URL, "page", 0), getIntOrDefault(r.URL, "size", 10))
		if err != nil {
			requestLogger.Error("could not load observations", "err", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		result := newHydraCollectionResult(ctx, r.URL, observations, int(totalItems))

		b, err := json.Marshal(result)
		if err != nil {
			requestLogger.Error("unable marshal observations result", "err", err.Error())
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
			requestLogger.Error("unable to read body", "err", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var so database.SensorObservation
		err = json.Unmarshal(body, &so)
		if err != nil {
			requestLogger.Error("unable to unmarshal body", "err", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		err = app.AddObservation(ctx, so)
		if err != nil {
			requestLogger.Error("unable to create observation", "err", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}

func handleCloudevents(ctx context.Context, app application.Application) http.HandlerFunc {
	log := logging.GetFromContext(ctx)

	eventCounter, err := otel.Meter("api-rec/cloudevents").Int64Counter(
		"diwise.cloudevents.total",
		metric.WithUnit("1"),
		metric.WithDescription("Total number of received cloudevents"),
	)

	if err != nil {
		log.Error("failed to create otel cloudevent counter", "err", err.Error())
	}

	return func(w http.ResponseWriter, r *http.Request) {
		var err error
		defer r.Body.Close()

		ctx, span := tracer.Start(r.Context(), "handle-cloudevents")
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()
		_, ctx, requestLogger := o11y.AddTraceIDToLoggerAndStoreInContext(span, log, ctx)

		event, err := cloudevents.NewEventFromHTTPRequest(r)
		if err != nil {
			requestLogger.Error("failed to parse cloud event from request", "err", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		eventCounter.Add(ctx, 1)

		var observation database.SensorObservation
		var observationOk bool = false

		switch event.Type() {
		case application.MessageAcceptedName:
			var ma application.MessageAccepted
			err := json.Unmarshal(event.Data(), &ma)
			if err != nil {
				requestLogger.Error("failed to parse message.accepted in cloud event", "err", err.Error())
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			observation, observationOk = ma.MapToObservation()
		case application.FunctionUpdatedName:
			var fu application.FunctionUpdated
			err := json.Unmarshal(event.Data(), &fu)
			if err != nil {
				requestLogger.Error("failed to parse function.updated in cloud event", "err", err.Error())
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			observation, observationOk = fu.MapToObservation()
		}

		if observationOk {
			err = app.AddObservation(ctx, observation)
			if err != nil {
				requestLogger.Error("failed to store observation", "err", err.Error())
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			return
		} else {
			requestLogger.Error("failed to map incomming message to observation", "err", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
}
