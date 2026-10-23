FROM golang:1.27-alpine AS build
# Fix upstream typo in Go 1.27.3 runtime/traceback.go comment ('/-' instead of '//')
RUN sed -i 's|/- outermost|// outermost|' /usr/local/go/src/runtime/traceback.go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/...

FROM alpine:3
RUN adduser -D -u 10001 app
COPY --from=build /out/ /usr/local/bin/
USER app
EXPOSE 8080
CMD ["app"]
