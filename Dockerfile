FROM golang:1.22-alpine AS builder
WORKDIR /src

COPY main.go .

RUN CGO_ENABLED=0 GOOS=linux go build -o lc3-assembler main.go

FROM debian:bookworm-slim
WORKDIR /app

COPY --from=builder /src/lc3-assembler /usr/local/bin/lc3-assembler

COPY vm /app/vm

RUN chmod +x /app/vm

CMD ["lc3-assembler"]