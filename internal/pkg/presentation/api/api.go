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
	"github.com/diwise/api-rec/internal/pkg/infrastructure/database"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/tracing"
	"github.com/farshidtz/senml/v2"
	"github.com/go-chi/chi/v5"
	"github.com/rs/cors"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("api-rec/api")

type hydraCollectionResult struct {
	Context    string                 `json:"@context"`
	Id         string                 `json:"@id"`
	Type       string                 `json:"@type"`
	TotalItems int                    `json:"hydra:totalItems"`
	Member     any                    `json:"hydra:member"`
	View       *partialCollectionView `json:"view,omitempty"`
}

type partialCollectionView struct {
	Id       string `json:"@id"`
	Type     string `json:"@type"`
	First    string `json:"first"`
	Previous string `json:"previous,omitempty"`
	Next     string `json:"next,omitempty"`
	Last     string `json:"last"`
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
		reqUri := url.Path + "?" + qry.Encode()
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
				r.Get("/", getObservations(ctx, db))
				r.Post("/", createObservation(ctx, db))
			})
			r.Route("/cloudevents", func(r chi.Router) {
				r.Post("/", handleCloudevents(ctx, db))
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
			entities, err = db.GetChildEntities(ctx, root, entityType)
			if err != nil {
				requestLogger.Error().Err(err).Msg("could not load entities from root entity")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			result = newHydraCollectionResult(r.URL, nil, entities, -1, -1, len(entities))
		} else {
			totalItems, entities, err := db.GetEntities(ctx, entityType, page, size)
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

func getObservations(ctx context.Context, db database.Database) http.HandlerFunc {
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

		totalItems, observations, err := db.GetObservations(ctx, sensorId, startingTime, endingTime, page, size)
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

func createObservation(ctx context.Context, db database.Database) http.HandlerFunc {
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

func handleCloudevents(ctx context.Context, db database.Database) http.HandlerFunc {
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
		case MessageAccepted:
			var m messageAccepted
			err := json.Unmarshal(event.Data(), &m)
			if err != nil {
				requestLogger.Error().Err(err).Msg("failed to parse message.accepted in cloud event")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			observation, observationOk = m.mapToObservation()
		case FunctionUpdated:
			var m functionUpdated
			err := json.Unmarshal(event.Data(), &m)
			if err != nil {
				requestLogger.Error().Err(err).Msg("failed to parse function.updated in cloud event")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			observation, observationOk = m.mapToObservation()
		}

		if observationOk {
			err = db.AddObservation(ctx, observation)
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

const FunctionUpdated = "function.updated"

type functionUpdated struct {
	Id      string `json:"id"`
	Type    string `json:"type"`
	SubType string `json:"subType"`
	Counter *struct {
		Counter int  `json:"counter"`
		State   bool `json:"state"`
	} `json:"counter,omitempty"`
	Level *struct {
		Current float64  `json:"current"`
		Percent *float64 `json:"percent,omitempty"`
		Offset  *float64 `json:"offset,omitempty"`
	} `json:"level,omitempty"`
	Presence *struct {
		State bool `json:"state"`
	} `json:"presence,omitempty"`
	Timer *struct {
		StartTime time.Time      `json:"startTime"`
		EndTime   *time.Time     `json:"endTime,omitempty"`
		Duration  *time.Duration `json:"duration,omitempty"`
		State     bool           `json:"state"`
	} `json:"timer,omitempty"`
	WaterQuality *struct {
		Temperature float64   `json:"temperature"`
		Timestamp   time.Time `json:"timestamp"`
	} `json:"waterquality,omitempty"`
	Building *struct {
		Energy float64 `json:"energy"`
		Power  float64 `json:"power"`
	} `json:"building,omitempty"`
}

func (m functionUpdated) mapToObservation() (database.SensorObservation, bool) {
	so := database.SensorObservation{
		Format:       "rec3.1.1",
		DeviceID:     fmt.Sprintf("%s:%s:%s", m.Type, m.SubType, m.Id),
		Observations: make([]database.Observation, 0),
	}

	ts := time.Now().UTC()

	switch m.Type {
	case "building":
		so.Observations = append(so.Observations, database.Observation{
			ObservationTime: ts,
			Value:           &m.Building.Energy,
			QuantityKind:    "Energy",
			SensorId:        m.Id,
		}, database.Observation{
			ObservationTime: ts,
			Value:           &m.Building.Power,
			QuantityKind:    "Power",
			SensorId:        m.Id,
		})
	case "counter":
		v := float64(m.Counter.Counter)
		so.Observations = append(so.Observations, database.Observation{
			ObservationTime: ts,
			Value:           &v,
			ValueBoolean:    &m.Counter.State,
			QuantityKind:    "diwise:Level",
			SensorId:        m.Id,
		})
	case "level":
		so.Observations = append(so.Observations, database.Observation{
			ObservationTime: ts,
			Value:           &m.Level.Current,
			QuantityKind:    "diwise:Level",
			SensorId:        m.Id,
		})
	case "presence":
		if m.SubType == "lifebuoy" {
			so.Observations = append(so.Observations, database.Observation{
				ObservationTime: ts,
				ValueBoolean:    &m.Presence.State,
				QuantityKind:    "diwise:Lifebuoy",
				SensorId:        m.Id,
			})
		} else {
			so.Observations = append(so.Observations, database.Observation{
				ObservationTime: ts,
				ValueBoolean:    &m.Presence.State,
				QuantityKind:    "diwise:Presence",
				SensorId:        m.Id,
			})
		}
	case "timer":
		v := m.Timer.Duration.Seconds()
		so.Observations = append(so.Observations, database.Observation{
			ObservationTime: *m.Timer.EndTime,
			ValueBoolean:    &m.Timer.State,
			Value:           &v,
			QuantityKind:    "diwise:Timer",
			SensorId:        m.Id,
		})
	case "waterquality":
		so.Observations = append(so.Observations, database.Observation{
			ObservationTime: m.WaterQuality.Timestamp,
			Value:           &m.WaterQuality.Temperature,
			QuantityKind:    "Temperature",
			SensorId:        m.Id,
		})
	default:
		return database.SensorObservation{}, false
	}

	return so, true
}

const MessageAccepted = "message.accepted"

type messageAccepted struct {
	SensorID  string     `json:"sensorID"`
	Pack      senml.Pack `json:"pack"`
	Timestamp time.Time  `json:"timestamp"`
}

func (m messageAccepted) mapToObservation() (database.SensorObservation, bool) {
	sensorId := m.Pack[0].StringValue
	observationTime := mapTime(m.Pack[0].BaseTime)
	quantityKind := mapQuantityKind(m)

	value := m.Pack[1].Value
	valueString := mapValueString(m.Pack[1].StringValue)
	valueBoolean := m.Pack[1].BoolValue

	if sensorId == "" || quantityKind == "" {
		return database.SensorObservation{}, false
	}

	so := database.SensorObservation{
		Format:   "rec3.1.1",
		DeviceID: m.SensorID,
		Observations: []database.Observation{
			{
				SensorId:        sensorId,
				ObservationTime: observationTime,
				QuantityKind:    quantityKind,
				Value:           value,
				ValueString:     valueString,
				ValueBoolean:    valueBoolean,
			},
		},
	}

	return so, true
}

func mapQuantityKind(m messageAccepted) string {
	lwm2mType := m.Pack[0].BaseName

	switch strings.ToLower(lwm2mType) {
	case AirQuality:
		if m.Pack[1].Name == "17" {
			return "Concentration"
		}

		return "diwise:AirQuality"
	case Conductivity:
		return "Conductivity"
	case DigitalInput:
		return "diwise:DigitalInput"
	case Distance:
		return "Distance"
	case Energy:
		return "Energy"
	case Power:
		return "Power"
	case Presence:
		return "diwise:Presence"
	case Pressure:
		return "Pressure"
	case Temperature:
		return "Temperature"
	case Watermeter:
		return "Volume"
	case Humidity:
		return "RelativeHumidity"
	case Illuminance:
		return "Illuminance"
	}

	return lwm2mType
}

func mapTime(bt float64) time.Time {
	return time.Unix(int64(bt), 0).UTC()
}

func mapValueString(vs string) *string {
	if vs == "" {
		return nil
	}
	return &vs
}

const (
	lwm2mPrefix string = "urn:oma:lwm2m:ext:"

	AirQuality   string = lwm2mPrefix + "3428"
	Conductivity string = lwm2mPrefix + "3327"
	DigitalInput string = lwm2mPrefix + "3200"
	Distance     string = lwm2mPrefix + "3330"
	Energy       string = lwm2mPrefix + "3331"
	Humidity     string = lwm2mPrefix + "3304"
	Illuminance  string = lwm2mPrefix + "3301"
	Power        string = lwm2mPrefix + "3328"
	Presence     string = lwm2mPrefix + "3302"
	Pressure     string = lwm2mPrefix + "3323"
	Temperature  string = lwm2mPrefix + "3303"
	Watermeter   string = lwm2mPrefix + "3424"
)
