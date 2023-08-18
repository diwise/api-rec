package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/diwise/api-rec/internal/pkg/infrastructure/database"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/tracing"
	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("api-rec/api")

/*
	** MUST **
	Implement the following subset of REC classes as a minimum; 
	ActuationInterface
	Actuator
	BuildingComponent
	Device
	RealEstate
	RealEstateComponent
	Sensor
	Storey
*/

func RegisterEndpoints(ctx context.Context, r *chi.Mux, db database.Database) {
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Route("/api", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Route("/spaces", func(r chi.Router) {
				r.Get("/", GetSpaces(ctx, db))
				r.Post("/", CreateSpace(ctx, db))
			})
			r.Route("/buildings", func(r chi.Router) {
				r.Get("/", GetBuildings(ctx, db))
				r.Post("/", CreateBuilding(ctx, db))
			})
			r.Route("/sensors", func(r chi.Router) {
				r.Get("/", GetSensors(ctx, db))
				r.Post("/", CreateSensor(ctx, db))
			})
			r.Route("/observations", func(r chi.Router) {
				r.Get("/", GetObservations(ctx, db))
				r.Get("/{id}", GetObservation(ctx, db))
			})
		})
	})
}

func createEntity(ctx context.Context, db database.Database, spanName string) http.HandlerFunc {
	log := logging.GetFromContext(ctx)

	return func(w http.ResponseWriter, r *http.Request) {
		var err error
		defer r.Body.Close()

		ctx, span := tracer.Start(r.Context(), spanName)
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
			requestLogger.Error().Err(err).Msgf("unable to add entity [%s]", spanName)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		e, err = db.GetEntity(ctx, e.Id, e.Type)
		if err != nil {
			requestLogger.Error().Err(err).Msgf("unable to fetch entity [%s]", spanName)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		b, err := json.Marshal(e)
		if err != nil {
			requestLogger.Error().Err(err).Msgf("unable marshal entity [%s]", spanName)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Add("Content-Type", "application/ld+json")
		w.WriteHeader(http.StatusCreated)
		w.Write(b)
	}
}

func CreateSpace(ctx context.Context, db database.Database) http.HandlerFunc {
	return createEntity(ctx, db, "create-space")
}

func CreateBuilding(ctx context.Context, db database.Database) http.HandlerFunc {
	return createEntity(ctx, db, "create-building")
}

func CreateSensor(ctx context.Context, db database.Database) http.HandlerFunc {
	return createEntity(ctx, db, "create-sensor")
}

func GetSpaces(ctx context.Context, db database.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

	}
}

func GetBuildings(ctx context.Context, db database.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

	}
}

func GetSensors(ctx context.Context, db database.Database) http.HandlerFunc {
	log := logging.GetFromContext(ctx)

	return func(w http.ResponseWriter, r *http.Request) {
		var err error
		defer r.Body.Close()

		ctx, span := tracer.Start(r.Context(), "get-sensors")
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()
		_, ctx, requestLogger := o11y.AddTraceIDToLoggerAndStoreInContext(span, log, ctx)

		/*
			?property=value
			?property[operator]=value	
			?hasObservationTime[starting]=2019-02-14

			 For instance queries like “all sensors on a building” or “all buildings near a geo location”, etc. 
			 The REC Consortium expects in the future to specify advanced query formats.

			root[type]=building
			root[id]=234-234-234-234
		*/

		buildingId := r.URL.Query().Get("building")
		if buildingId == "" {
			requestLogger.Error().Err(err).Msg("no building in query string")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		building, err := db.GetEntity(ctx, buildingId, "dtmi:org:w3id:rec:Building;1")
		if err != nil {
			requestLogger.Error().Err(err).Msg("no building in query string")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		sensors, err := db.GetChildEntities(ctx, building, "dtmi:org:brickschema:schema:Brick:Sensor;1")
		if err != nil {
			requestLogger.Error().Err(err).Msg("could not fetch sensors")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		b, err := json.Marshal(sensors)
		if err != nil {
			requestLogger.Error().Err(err).Msg("unable marshal entities [get-sensors]")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Add("Content-Type", "application/ld+json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
	}
}

func GetObservation(ctx context.Context, db database.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

	}
}

func GetObservations(ctx context.Context, db database.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

	}
}
