FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY  ./ ./

RUN go clean --modcache && go build -ldflags "-w" -mod=readonly -o /bin cmd/server/app.go

FROM alpine
RUN apk update && apk add wget

WORKDIR /
COPY --from=builder  /bin /bin

EXPOSE 8080

CMD ["bin/app"]