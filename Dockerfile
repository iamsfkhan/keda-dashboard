# syntax=docker/dockerfile:1.7
FROM node:22-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.23-alpine AS backend
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=frontend /src/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/keda-dashboard ./cmd/dashboard

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=backend /out/keda-dashboard /keda-dashboard
ENV DEMO_MODE=false
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/keda-dashboard"]
