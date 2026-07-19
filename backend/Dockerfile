FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/dbfock-api ./cmd/api

FROM alpine:3.21
RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/dbfock-api /app/dbfock-api
COPY migrations /app/migrations
RUN mkdir /data && chown -R app:app /data /app
USER app
EXPOSE 8080
CMD ["/app/dbfock-api"]
