package application

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/diwise/api-rec/internal/pkg/infrastructure/database"
	"github.com/farshidtz/senml/v2"
)

const FunctionUpdatedName = "function.updated"

type FunctionUpdated struct {
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

func (m FunctionUpdated) MapToObservation() (database.SensorObservation, bool) {
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
			Value:           mapFloatValue(&m.WaterQuality.Temperature, 1),
			QuantityKind:    "Temperature",
			SensorId:        m.Id,
		})
	default:
		return database.SensorObservation{}, false
	}

	return so, true
}

const MessageAcceptedName = "message.accepted"

type MessageAccepted struct {
	SensorID  string     `json:"sensorID"`
	Pack      senml.Pack `json:"pack"`
	Timestamp time.Time  `json:"timestamp"`
}

func (m MessageAccepted) MapToObservation() (database.SensorObservation, bool) {
	sensorId := m.Pack[0].StringValue
	observationTime := mapTime(m.Pack[0].BaseTime)
	quantityKind := mapQuantityKind(m)

	var value *float64

	if quantityKind == "Temperature" {
		value = mapFloatValue(m.Pack[1].Value, 1)
	} else {
		value = mapFloatValue(m.Pack[1].Value, 2)
	}

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

func mapQuantityKind(m MessageAccepted) string {
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

func mapFloatValue(v *float64, d uint) *float64 {
	if v == nil {
		return nil
	}

	unit := math.Pow(10, float64(d))
	f := math.Round(*v*unit) / unit
	return &f
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
