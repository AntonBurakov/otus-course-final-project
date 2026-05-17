import os
import uuid
from datetime import datetime

import httpx
from fastapi import FastAPI, Header, HTTPException
from pydantic import BaseModel, Field


AUTH_SERVICE_URL = os.getenv("AUTH_SERVICE_URL", "http://auth-service:8001")
CONSUL_HTTP_ADDR = os.getenv("CONSUL_HTTP_ADDR", "http://consul:8500")

app = FastAPI(title="Events Service", version="1.0.0")
events: dict[str, dict] = {}


class EventCreate(BaseModel):
    title: str = Field(min_length=3, max_length=120)
    description: str = Field(default="", max_length=1000)
    starts_at: datetime
    capacity: int = Field(gt=0, le=100000)


async def register_in_consul() -> None:
    payload = {
        "ID": "events-service",
        "Name": "events-service",
        "Address": "events-service",
        "Port": 8002,
        "Check": {"HTTP": "http://events-service:8002/health", "Interval": "10s"},
    }
    try:
        async with httpx.AsyncClient(timeout=2) as client:
            await client.put(f"{CONSUL_HTTP_ADDR}/v1/agent/service/register", json=payload)
    except httpx.HTTPError:
        pass


async def require_user(authorization: str | None) -> str:
    if not authorization:
        raise HTTPException(status_code=401, detail="missing token")
    async with httpx.AsyncClient(timeout=5) as client:
        response = await client.get(f"{AUTH_SERVICE_URL}/verify", headers={"Authorization": authorization})
    if response.status_code != 200:
        raise HTTPException(status_code=401, detail="invalid token")
    return response.json()["email"]


@app.on_event("startup")
async def startup() -> None:
    await register_in_consul()
    sample_id = "evt-sample-1"
    events[sample_id] = {
        "id": sample_id,
        "title": "OTUS Architecture Night",
        "description": "Demo event for the final microservices project.",
        "starts_at": "2026-06-01T19:00:00",
        "capacity": 100,
        "booked": 0,
    }


@app.get("/health")
async def health() -> dict[str, str]:
    return {"status": "ok", "service": "events-service"}


@app.get("/events")
async def list_events() -> list[dict]:
    return list(events.values())


@app.get("/events/{event_id}")
async def get_event(event_id: str) -> dict:
    event = events.get(event_id)
    if not event:
        raise HTTPException(status_code=404, detail="event not found")
    return event


@app.post("/events", status_code=201)
async def create_event(payload: EventCreate, authorization: str | None = Header(default=None)) -> dict:
    await require_user(authorization)
    event_id = f"evt-{uuid.uuid4().hex[:12]}"
    event = {
        "id": event_id,
        "title": payload.title,
        "description": payload.description,
        "starts_at": payload.starts_at.isoformat(),
        "capacity": payload.capacity,
        "booked": 0,
    }
    events[event_id] = event
    return event
