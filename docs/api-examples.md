# API Examples

Run the stack:

```bash
docker compose up --build
```

Register:

```bash
curl -X POST http://localhost:8000/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"student@example.com","password":"secret123"}'
```

Login:

```bash
curl -X POST http://localhost:8000/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"student@example.com","password":"secret123"}'
```

List events:

```bash
curl http://localhost:8000/events
```

Create a booking:

```bash
TOKEN='<paste access_token>'

curl -X POST http://localhost:8000/bookings \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"event_id":"evt-sample-1"}'
```
