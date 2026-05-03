FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
#RUN go mod download
COPY vendor/ ./vendor/
COPY . .

#RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o proxy-convert .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -mod=vendor -o proxy-convert .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/proxy-convert .
COPY clash_template.json ./clash_template.json
COPY templates ./templates
COPY bin ./bin

RUN mkdir -p /root/database /root/bin /root/templates && \
    chmod +x ./bin/mihomo-linux-* 2>/dev/null || true 

EXPOSE 5000

CMD ["./proxy-convert"]
