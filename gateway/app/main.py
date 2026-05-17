import os
from typing import Any

import httpx
from fastapi import FastAPI, Header, HTTPException, Request


AUTH_SERVICE_URL = os.getenv("AUTH_SERVICE_URL", "http://auth-service:8001")
EVENTS_SERVICE_URL = os.getenv("EVENTS_SERVICE_URL", "http://events-service:8002")
BOOKING_SERVICE_URL = os.getenv("BOOKING_SERVICE_URL", "http://booking-service:8003")
CONSUL_HTTP_ADDR = os.getenv("CONSUL_HTTP_ADDR", "http://consul:8500")

app = FastAPI(title="Event Booking API Gateway", version="1.0.0")


async def register_in_consul() -> None:
    payload = {
        "ID": "gateway",
        "Name": "gateway",
        "Address": "gateway",
        "Port": 8000,
        "Check": {"HTTP": "http://gateway:8000/health", "Interval": "10s"},
    }
    try:
        async with httpx.AsyncClient(timeout=2) as client:
            await client.put(f"{CONSUL_HTTP_ADDR}/v1/agent/service/register", json=payload)
    except httpx.HTTPError:
        pass


@app.on_event("startup")
async def startup() -> None:
    await register_in_consul()


@app.get("/health")
async def health() -> dict[str, str]:
    return {"status": "ok", "service": "gateway"}


async def proxy_json(method: str, url: str, request: Request, authorization: str | None = None) -> Any:
    body = await request.json() if method in {"POST", "PUT", "PATCH"} else None
    headers = {"Authorization": authorization} if authorization else {}
    async with httpx.AsyncClient(timeout=10) as client:
        response = await client.request(method, url, json=body, headers=headers)
    if response.status_code >= 400:
        raise HTTPException(status_code=response.status_code, detail=response.json())
    return response.json()


@app.post("/auth/register")
async def register(request: Request) -> Any:
    return await proxy_json("POST", f"{AUTH_SERVICE_URL}/register", request)


@app.post("/auth/login")
async def login(request: Request) -> Any:
    return await proxy_json("POST", f"{AUTH_SERVICE_URL}/login", request)


@app.get("/events")
async def list_events() -> Any:
    async with httpx.AsyncClient(timeout=10) as client:
        response = await client.get(f"{EVENTS_SERVICE_URL}/events")
    return response.json()


@app.post("/events")
async def create_event(request: Request, authorization: str | None = Header(default=None)) -> Any:
    return await proxy_json("POST", f"{EVENTS_SERVICE_URL}/events", request, authorization)


@app.post("/bookings")
async def create_booking(request: Request, authorization: str | None = Header(default=None)) -> Any:
    return await proxy_json("POST", f"{BOOKING_SERVICE_URL}/bookings", request, authorization)


@app.get("/bookings/{booking_id}")
async def get_booking(booking_id: str, authorization: str | None = Header(default=None)) -> Any:
    headers = {"Authorization": authorization} if authorization else {}
    async with httpx.AsyncClient(timeout=10) as client:
        response = await client.get(f"{BOOKING_SERVICE_URL}/bookings/{booking_id}", headers=headers)
    if response.status_code >= 400:
        raise HTTPException(status_code=response.status_code, detail=response.json())
    return response.json()
