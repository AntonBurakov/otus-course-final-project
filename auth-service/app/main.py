import hashlib
import hmac
import os
import secrets

import httpx
from fastapi import FastAPI, Header, HTTPException, Query
from pydantic import BaseModel, EmailStr


CONSUL_HTTP_ADDR = os.getenv("CONSUL_HTTP_ADDR", "http://consul:8500")
VAULT_ADDR = os.getenv("VAULT_ADDR", "http://vault:8200")
VAULT_TOKEN = os.getenv("VAULT_TOKEN", "dev-token")
JWT_SECRET = os.getenv("JWT_SECRET", "local-dev-secret")

app = FastAPI(title="Auth Service", version="1.0.0")
users: dict[str, dict[str, str]] = {}
sessions: dict[str, str] = {}


class Credentials(BaseModel):
    email: EmailStr
    password: str


async def load_secret_from_vault() -> None:
    global JWT_SECRET
    try:
        async with httpx.AsyncClient(timeout=2) as client:
            response = await client.get(
                f"{VAULT_ADDR}/v1/secret/data/event-booking/auth",
                headers={"X-Vault-Token": VAULT_TOKEN},
            )
        if response.status_code == 200:
            JWT_SECRET = response.json()["data"]["data"].get("jwt_secret", JWT_SECRET)
    except httpx.HTTPError:
        pass


async def register_in_consul() -> None:
    payload = {
        "ID": "auth-service",
        "Name": "auth-service",
        "Address": "auth-service",
        "Port": 8001,
        "Check": {"HTTP": "http://auth-service:8001/health", "Interval": "10s"},
    }
    try:
        async with httpx.AsyncClient(timeout=2) as client:
            await client.put(f"{CONSUL_HTTP_ADDR}/v1/agent/service/register", json=payload)
    except httpx.HTTPError:
        pass


def hash_password(password: str) -> str:
    salt = secrets.token_hex(8)
    digest = hashlib.sha256(f"{salt}:{password}".encode()).hexdigest()
    return f"{salt}:{digest}"


def verify_password(password: str, stored: str) -> bool:
    salt, digest = stored.split(":", 1)
    candidate = hashlib.sha256(f"{salt}:{password}".encode()).hexdigest()
    return hmac.compare_digest(candidate, digest)


def issue_token(email: str) -> str:
    nonce = secrets.token_urlsafe(24)
    signature = hmac.new(JWT_SECRET.encode(), f"{email}:{nonce}".encode(), hashlib.sha256).hexdigest()
    token = f"{email}:{nonce}:{signature}"
    sessions[token] = email
    return token


@app.on_event("startup")
async def startup() -> None:
    await load_secret_from_vault()
    await register_in_consul()


@app.get("/health")
async def health() -> dict[str, str]:
    return {"status": "ok", "service": "auth-service"}


@app.post("/register")
async def register(credentials: Credentials) -> dict[str, str]:
    email = credentials.email.lower()
    if email in users:
        raise HTTPException(status_code=409, detail="user already exists")
    users[email] = {"email": email, "password_hash": hash_password(credentials.password)}
    return {"email": email, "status": "registered"}


@app.post("/login")
async def login(credentials: Credentials) -> dict[str, str]:
    email = credentials.email.lower()
    user = users.get(email)
    if not user or not verify_password(credentials.password, user["password_hash"]):
        raise HTTPException(status_code=401, detail="invalid credentials")
    return {"access_token": issue_token(email), "token_type": "Bearer"}


@app.get("/verify")
async def verify(
    authorization_header: str | None = Header(default=None, alias="Authorization"),
    authorization_query: str | None = Query(default=None, alias="authorization"),
) -> dict[str, str]:
    authorization = authorization_header or authorization_query
    if not authorization or not authorization.startswith("Bearer "):
        raise HTTPException(status_code=401, detail="missing token")
    token = authorization.removeprefix("Bearer ").strip()
    email = sessions.get(token)
    if not email:
        raise HTTPException(status_code=401, detail="invalid token")
    return {"email": email}
