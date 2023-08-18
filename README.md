# API-REC

En *spike* för ett API. Lite finns det kod för att ev. bygga vidare på.

# In

## Sensorvärden

sensor -> iot-agent -> iot-events -> api-rec

```json
{
  "format": "rec3.3",
  "deviceId": "https://recref.com/device/64b65a99-a53c-47f5-b959-1c7a641d82d8",
  "observations": [
    {
      "observationTime": "2019-05-27T20:07:44Z",
      "value": 16.1,
      "quantityKind": "https://w3id.org/rec/core/Temperature",
      "sensorId" : "https://recref.com/sensor/e0d5120b-90f1-48d6-a47f-f8ccd7727b04"
    }
  ]
}
```

[https://github.com/RealEstateCore/rec/blob/main/API/Edge/edge_message.schema.json](https://github.com/RealEstateCore/rec/blob/main/API/Edge/edge_message.schema.json)

Skillnden mellan `deviceId` och `sensorId` är att ett `device` kan ha en eller flera `sensor`er i samma "låda".
Dock har man i Iduns [exempel](https://github.com/idun-corp/Idun-Examples/blob/master/ProptechOS-Streaming-Api/examples/netcore/dedicated-processor/SensorObservation.cs) för ProptechOS `sensorId` för båda properties.

`api-rec` tar emot meddelanden och lagrar i ett tidsserie format.

## Struktur

API för att stukturera fastigheter. Vi kan behöva fler/andra modeller från REC.

**POST** `/spaces`

```json
{
  "@context": "https://dev.realestatecore.io/contexts/Space.jsonld",
  "@id": "00f67d60-d4d4-4bd5-af32-cf6c9b9310ec",
  "@type": "dtmi:org:w3id:rec:Space;1"
}
```

**POST** `/buildings`

```json
{
  "@context": "https://dev.realestatecore.io/contexts/Building.jsonld",
  "@id": "79b30db6-c5d3-4cd1-a438-6d8954b330ad",
  "@type": "dtmi:org:w3id:rec:Building;1",
  "isPartOf" : {
        "@id": "00f67d60-d4d4-4bd5-af32-cf6c9b9310ec",
        "@type": "dtmi:org:w3id:rec:Space;1"
  }
}
```

**POST** `/sensors`

```json
{
  "@context": "https://dev.realestatecore.io/contexts/Sensor.jsonld",
  "@id": "76bb4d31-1167-49e0-8766-768eb47c47e2",
  "@type": "dtmi:org:brickschema:schema:Brick:Sensor;1",
  "isPartOf" : {
        "@id": "79b30db6-c5d3-4cd1-a438-6d8954b330ad",
        "@type": "dtmi:org:w3id:rec:Building;1"
  }
}
```

`isPartOf` skapar relation mellan entiteter. Alla modeller har fler properties för metadata som inte finns med i *spiken*.

# Ut

Exempel med `/sensors`. Andra endpoints skulle kunna ha samma/liknande filter. Annat möjligt filter för t.ex. `/observations` skulle vara `?observationTime[starting]=2023-08-01&observationTime[ending]=2023-08-31` för att få ut data för augusti.

Se [Time interval queries](https://github.com/RealEstateCore/rec/blob/main/API/REST/RealEstateCore_REST_specification.md#time-interval-queries) för information.

`root[type]` och `root[id]` finns inte i spec, men faller kanske in under [Advanced queries](https://github.com/RealEstateCore/rec/blob/main/API/REST/RealEstateCore_REST_specification.md#advanced-queries) och är tänkt svara på frågor som "ge mig alla sensorer i byggnad X".

Möjligen måste `type` vara `dtmi:org:w3id:rec:Building;1` och inte bara `building`. Men det finns en CSV med översättningar för [endpoints](https://github.com/RealEstateCore/rec/blob/main/API/REST/Endpoints.csv).

**GET** `/sensors?root[type]=building&root[id]=79b30db6-c5d3-4cd1-a438-6d8954b330ad`

```json
{
  "@context": {
    "@base": "string",
    "hydra": "http://www.w3.org/ns/hydra/core#"
  },
  "@type": "hydra:Collection",
  "hydra:totalItems": 1,
  "hydra:member": [
    {
      "@context": "https://dev.realestatecore.io/contexts/Sensor.jsonld",
      "@id": "76bb4d31-1167-49e0-8766-768eb47c47e2",
      "@type": "dtmi:org:brickschema:schema:Brick:Sensor;1",
    }
  ]
}
```

oklart vad `"@base": "string"` i första `@context` ska vara.

**GET** `/observations?root[type]=sensor&root[id]=76bb4d31-1167-49e0-8766-768eb47c47e2&observationTime[starting]=2023-08-01&observationTime[ending]=2023-08-31`

```json
{
  "@context": {
    "@base": "string",
    "hydra": "http://www.w3.org/ns/hydra/core#"
  },
  "@type": "hydra:Collection",
  "hydra:totalItems": 1,
  "hydra:member": [
    {
      "observationTime": "2019-05-27T20:07:44Z",
      "value": 16.1,
      "quantityKind": "https://w3id.org/rec/core/Temperature",
      "sensorId" : "https://recref.com/sensor/e0d5120b-90f1-48d6-a47f-f8ccd7727b04"
    }
  ]
}
```

# Databas

En graf skapas med två tabeller tills det behövs en riktig grafdatabashanterare.

## DDL

```sql
CREATE TYPE entity_type AS ENUM (
    'dtmi:org:w3id:rec:Space;1',
    'dtmi:org:w3id:rec:Building;1',
    'dtmi:org:brickschema:schema:Brick:Sensor;1',
    'dtmi:org:w3id:rec:ObservationEvent;1'
);

CREATE TYPE entity_context AS ENUM (
    'https://dev.realestatecore.io/contexts/Space.jsonld',
    'https://dev.realestatecore.io/contexts/Building.jsonld',
    'https://dev.realestatecore.io/contexts/Sensor.jsonld',
    'https://dev.realestatecore.io/contexts/ObservationEvent.jsonld'
);

CREATE TABLE entity (
    node_id        BIGSERIAL,
    entity_id      TEXT NOT NULL,
    entity_type    entity_type NOT NULL,
    entity_context entity_context NOT NULL,
    PRIMARY KEY (node_id)
);

CREATE UNIQUE INDEX entity_entity_type_entity_id_unique_indx ON entity (entity_type, entity_id);

CREATE table relation (
    parent        BIGINT NOT NULL,
    child         BIGINT NOT NULL,
    PRIMARY KEY (parent, child)
);

CREATE INDEX relation_child_parent_indx ON relation(child, parent);

```

`ENUM` för att säkra korrekt data. Dock ingen kontroll att ett `context` och en `type` hör ihop genom detta.

## SQL

SQL för att svara på frågor som t.ex. "ge mig alla sensorer i byggnad X"

```sql
WITH RECURSIVE traverse(node_id, entity_type, entity_id) AS (
    SELECT
        node_id,
        entity_type,
        entity_id
    FROM
        entity
    WHERE
        entity.entity_id = $1 AND
        entity.entity_type = $2
    UNION ALL
    SELECT
        entity.node_id,
        entity.entity_type,
        entity.entity_id
    FROM traverse JOIN
    relation ON traverse.node_id = relation.parent JOIN
    entity ON relation.child = entity.node_id
)
SELECT
    traverse.entity_id
FROM traverse
WHERE traverse.entity_type = $3
GROUP BY traverse.entity_id
ORDER BY traverse.entity_id ASC
```

PK för en entitet är `id` + `type`. `$1` och `$2` pekar ut root-entiteten och `$3` den typ som ska hittas bland kopplade entiteter.
